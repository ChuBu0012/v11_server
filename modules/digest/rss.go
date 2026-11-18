package digest

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// RSSItem represents an RSS 2.0 item.
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// RSSChannel represents an RSS 2.0 channel.
type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

// RSSFeed represents an RSS 2.0 document.
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

// AtomEntry represents an Atom entry.
type atomEntry struct {
	Title string `xml:"title"`
	Link  struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Summary string `xml:"summary"`
	Content string `xml:"content"`
	Updated string `xml:"updated"`
}

// AtomFeed represents an Atom document.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTML(input string) string {
	cleaned := htmlTagRegex.ReplaceAllString(input, " ")
	cleaned = html.UnescapeString(cleaned)
	return strings.Join(strings.Fields(cleaned), " ")
}

// FetchFeedOptions configures the feed fetcher.
type FetchFeedOptions struct {
	Client *http.Client
}

// FetchFeed downloads and parses an RSS or Atom feed, returning up to maxItems.
func FetchFeed(ctx context.Context, category, sourceName, feedURL string, maxItems int, client *http.Client) ([]NewsItem, error) {
	if client == nil {
		tr := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 15 * time.Second,
				}
				return dialer.DialContext(ctx, "tcp4", addr)
			},
			ForceAttemptHTTP2: true,
			IdleConnTimeout:   30 * time.Second,
		}
		client = &http.Client{Timeout: 15 * time.Second, Transport: tr}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; vivo 1804) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed %s failed: %w", feedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed %s returned HTTP %d", feedURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*512)) // 512KB max
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	var items []NewsItem

	// Try RSS 2.0 first
	var rFeed rssFeed
	if err := xml.Unmarshal(body, &rFeed); err == nil && len(rFeed.Channel.Items) > 0 {
		count := 0
		for _, item := range rFeed.Channel.Items {
			if count >= maxItems {
				break
			}
			t := strings.TrimSpace(item.Title)
			if t == "" {
				continue
			}
			pubDate := parseFlexibleDate(item.PubDate)
			desc := stripHTML(item.Description)
			if len(desc) > 300 {
				desc = desc[:300] + "..."
			}

			items = append(items, NewsItem{
				Category:    category,
				Source:      sourceName,
				Title:       t,
				Link:        strings.TrimSpace(item.Link),
				Description: desc,
				PublishedAt: pubDate,
			})
			count++
		}
		return items, nil
	}

	// Try Atom next
	var aFeed atomFeed
	if err := xml.Unmarshal(body, &aFeed); err == nil && len(aFeed.Entries) > 0 {
		count := 0
		for _, entry := range aFeed.Entries {
			if count >= maxItems {
				break
			}
			t := strings.TrimSpace(entry.Title)
			if t == "" {
				continue
			}
			pubDate := parseFlexibleDate(entry.Updated)
			desc := entry.Summary
			if desc == "" {
				desc = entry.Content
			}
			desc = stripHTML(desc)
			if len(desc) > 300 {
				desc = desc[:300] + "..."
			}

			link := strings.TrimSpace(entry.Link.Href)
			items = append(items, NewsItem{
				Category:    category,
				Source:      sourceName,
				Title:       t,
				Link:        link,
				Description: desc,
				PublishedAt: pubDate,
			})
			count++
		}
		return items, nil
	}

	return nil, fmt.Errorf("could not parse feed from %s as RSS or Atom", feedURL)
}

func parseFlexibleDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Now()
	}
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05Z",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, strings.TrimSpace(dateStr)); err == nil {
			return t
		}
	}
	return time.Now()
}
