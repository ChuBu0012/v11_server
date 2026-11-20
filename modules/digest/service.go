package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"v11-server/modules/alerts"
)

// FeedSource defines a curated RSS/Atom feed to pull from.
type FeedSource struct {
	Category string
	Name     string
	URL      string
	MaxItems int
}

// DefaultCuratedFeeds contains recommended AI, political, and financial feeds.
var DefaultCuratedFeeds = []FeedSource{
	{
		Category: "AI & Tech",
		Name:     "TechCrunch AI",
		URL:      "https://techcrunch.com/category/artificial-intelligence/feed/",
		MaxItems: 4,
	},
	{
		Category: "AI & Tech",
		Name:     "The Verge AI",
		URL:      "https://www.theverge.com/rss/ai-artificial-intelligence/index.xml",
		MaxItems: 4,
	},
	{
		Category: "Politics & News",
		Name:     "BBC Thai",
		URL:      "https://feeds.bbci.co.uk/thai/rss.xml",
		MaxItems: 4,
	},
	{
		Category: "Politics & News",
		Name:     "The Standard Politics",
		URL:      "https://thestandard.co/category/news/politics/feed/",
		MaxItems: 4,
	},
	{
		Category: "Stocks",
		Name:     "Kaohoon",
		URL:      "https://www.kaohoon.com/feed",
		MaxItems: 4,
	},
	{
		Category: "Stocks",
		Name:     "Prachachat Finance",
		URL:      "https://www.prachachat.net/finance/feed",
		MaxItems: 4,
	},
}

// Service coordinates fetching feeds, invoking Gemini, updating SQLite, and dispatching to Telegram.
type Service struct {
	store      *Store
	gemini     *GeminiClient
	notifier   alerts.Notifier
	httpClient *http.Client
	feeds      []FeedSource
	goldSource GoldSource
	messenger  Messenger
}

// NewService creates a digest Service instance.
func NewService(store *Store, gemini *GeminiClient, notifier alerts.Notifier, opts ...ServiceOption) *Service {
	s := &Service{
		store:      store,
		gemini:     gemini,
		notifier:   notifier,
		httpClient: &http.Client{Timeout: 12 * time.Second},
		feeds:      DefaultCuratedFeeds,
		goldSource: NewGoldFetcher(nil),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServiceOption allows configuring the digest Service.
type ServiceOption func(*Service)

// WithFeeds sets custom feed sources (useful for unit testing).
func WithFeeds(feeds []FeedSource) ServiceOption {
	return func(s *Service) { s.feeds = feeds }
}

// WithHTTPClient sets a custom HTTP client for feed fetching.
func WithServiceHTTPClient(client *http.Client) ServiceOption {
	return func(s *Service) { s.httpClient = client }
}

// WithGoldSource sets custom gold source (useful for unit testing).
func WithGoldSource(g GoldSource) ServiceOption {
	return func(s *Service) { s.goldSource = g }
}

// WithMessenger sets custom messenger for menu buttons dispatching.
func WithMessenger(m Messenger) ServiceOption {
	return func(s *Service) { s.messenger = m }
}

// SetMessenger sets the messenger on an existing service instance.
func (s *Service) SetMessenger(m Messenger) {
	s.messenger = m
}

// CollectFeedItems fetches all configured news items concurrently.
func (s *Service) CollectFeedItems(ctx context.Context) []NewsItem {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allItems []NewsItem

	for _, feed := range s.feeds {
		wg.Add(1)
		go func(f FeedSource) {
			defer wg.Done()
			feedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			items, err := FetchFeed(feedCtx, f.Category, f.Name, f.URL, f.MaxItems, s.httpClient)
			if err != nil {
				log.Printf("[digest:feed] Warning: fetch feed %s (%s) failed: %v", f.Name, f.URL, err)
				return
			}
			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
		}(feed)
	}

	wg.Wait()
	return allItems
}

// GenerateDigest generates a fresh daily digest. If force is false and digest is disabled, it skips execution.
func (s *Service) GenerateDigest(ctx context.Context, force bool) (*RunRecord, error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load digest settings: %w", err)
	}

	if !settings.Enabled && !force {
		log.Printf("[digest] Scheduled run skipped: digest is paused in settings")
		_ = s.store.RecordSkipped(ctx)
		return nil, nil
	}

	log.Printf("[digest] 📰 Starting AI Daily Digest generation (force=%v)...", force)
	runID, err := s.store.RecordStart(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to record digest start: %w", err)
	}

	// 1. Gather RSS News Items & Recategorize Gold
	rawItems := s.CollectFeedItems(ctx)
	items := recategorizeGold(rawItems)
	log.Printf("[digest] Collected %d news items from RSS feeds (after gold categorization)", len(items))

	// Fetch Gold Quotes & Previous Run
	var goldQuote GoldQuote
	if s.goldSource != nil {
		goldQuote = s.goldSource.Fetch(ctx)
	}
	prevGold, _ := s.store.PreviousGold(ctx, 0)

	// 2. Call Gemini for Summarization
	geminiCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	summary, err := s.gemini.SummarizeNews(geminiCtx, items)
	if err != nil {
		log.Printf("[digest] Error: Gemini summarization failed: %v", err)
		_ = s.store.RecordFailure(ctx, runID, err.Error())
		return nil, fmt.Errorf("gemini summarization failed: %w", err)
	}

	// 2.1 Inject Gold prices into Summary
	summary = injectGold(summary, goldQuote, prevGold)
	goldJSONBytes, _ := json.Marshal(goldQuote)
	goldJSON := string(goldJSONBytes)

	// 3. Save Success Record to SQLite
	if err := s.store.RecordSuccess(ctx, runID, summary, len(items), goldJSON); err != nil {
		log.Printf("[digest] Warning: failed to record success in store: %v", err)
	}

	record, err := s.store.GetRun(ctx, runID)
	if err != nil || record == nil {
		record, _ = s.store.GetLatestRun(ctx)
	}

	// 4. Dispatch to Telegram (single message with buttons or fallback to notifier)
	if s.messenger != nil {
		overviewMsg := RenderView(record, "overview", prevGold)
		buttons := MenuButtons(runID, "overview")
		teleCtx, teleCancel := context.WithTimeout(ctx, 25*time.Second)
		defer teleCancel()
		if err := s.messenger.SendMenu(teleCtx, overviewMsg, buttons); err != nil {
			log.Printf("[digest:messenger] Warning: dispatch menu failed: %v", err)
		} else {
			log.Printf("[digest:messenger] ✅ Sent single daily digest menu to Telegram")
		}
	} else if s.notifier != nil {
		overviewMsg := RenderView(record, "overview", prevGold)
		teleCtx, teleCancel := context.WithTimeout(ctx, 25*time.Second)
		defer teleCancel()
		if err := s.notifier.Send(teleCtx, overviewMsg); err != nil {
			log.Printf("[digest:telegram] Warning: dispatch to Telegram failed: %v", err)
		} else {
			log.Printf("[digest:telegram] ✅ Sent daily digest to Telegram successfully")
		}
	}

	return record, nil
}

// GetStore returns the underlying Store.
func (s *Service) GetStore() *Store {
	return s.store
}
