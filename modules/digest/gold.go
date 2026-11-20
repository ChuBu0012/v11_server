package digest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Gold price sources (probed live 2026-11-21, see the digest one-message spec).
const (
	goldAssociationURL = "https://classic.goldtraders.or.th/"
	goldSpotURL        = "https://xaus.com/api/v1/spot"
	goldFXURL          = "https://open.er-api.com/v6/latest/USD"

	gramsPerBaht    = 15.244
	gramsPerTroyOz  = 31.1035
	thaiGoldPurity  = 0.965
	goldFetchBudget = 12 * time.Second
)

// GoldQuote is one snapshot of gold prices. Zero means "could not fetch".
type GoldQuote struct {
	SpotUSD     float64   `json:"spot_usd_oz"`
	BarSell     float64   `json:"bar_sell"`
	BarBuy      float64   `json:"bar_buy"`
	OrnSell     float64   `json:"ornament_sell"`
	OrnBuy      float64   `json:"ornament_buy"`
	AnnouncedAt string    `json:"announced_at,omitempty"`
	Estimated   bool      `json:"estimated"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// GoldFetcher pulls world spot, Thai association prices and (for the
// fallback estimate) USD/THB.
type GoldFetcher struct {
	client                   *http.Client
	assocURL, spotURL, fxURL string
}

func NewGoldFetcher(client *http.Client) *GoldFetcher {
	if client == nil {
		client = &http.Client{Timeout: goldFetchBudget}
	}
	return &GoldFetcher{client: client, assocURL: goldAssociationURL, spotURL: goldSpotURL, fxURL: goldFXURL}
}

// Fetch never fails as a whole: each source is independent, and when the
// association page cannot be read the Thai bar price is estimated from spot.
func (f *GoldFetcher) Fetch(ctx context.Context) GoldQuote {
	ctx, cancel := context.WithTimeout(ctx, goldFetchBudget)
	defer cancel()
	q := GoldQuote{FetchedAt: time.Now()}
	if body, err := f.get(ctx, f.spotURL); err == nil {
		q.SpotUSD, _ = parseSpot(body)
	}
	if body, err := f.get(ctx, f.assocURL); err == nil {
		if bar, barBuy, orn, ornBuy, at, ok := parseAssociation(string(body)); ok {
			q.BarSell, q.BarBuy, q.OrnSell, q.OrnBuy, q.AnnouncedAt = bar, barBuy, orn, ornBuy, at
			return q
		}
	}
	if q.SpotUSD > 0 {
		if body, err := f.get(ctx, f.fxURL); err == nil {
			if thb, ok := parseTHB(body); ok {
				q.BarSell = estimateBar(q.SpotUSD, thb)
				q.Estimated = true
			}
		}
	}
	return q
}

func (f *GoldFetcher) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (PocketSentinel digest)")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, io.ErrUnexpectedEOF
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512*1024))
}

var assocLabel = regexp.MustCompile(`id="DetailPlace_uc_goldprices1_lbl(AsTime|BLSell|BLBuy|OMSell|OMBuy)"[^>]*>(.*?)</span>`)
var tagStrip = regexp.MustCompile(`<[^>]+>`)

// parseAssociation reads the classic goldtraders.or.th price labels.
func parseAssociation(page string) (bar, barBuy, orn, ornBuy float64, at string, ok bool) {
	values := map[string]string{}
	for _, m := range assocLabel.FindAllStringSubmatch(page, -1) {
		values[m[1]] = strings.TrimSpace(tagStrip.ReplaceAllString(m[2], ""))
	}
	num := func(key string) float64 {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(values[key], ",", ""), 64)
		return v
	}
	bar, barBuy, orn, ornBuy, at = num("BLSell"), num("BLBuy"), num("OMSell"), num("OMBuy"), values["AsTime"]
	ok = bar > 0 && barBuy > 0 && orn > 0
	return
}

func parseSpot(body []byte) (float64, bool) {
	var v struct {
		Spot float64 `json:"spot_usd_oz"`
	}
	if json.Unmarshal(body, &v) != nil || v.Spot <= 0 {
		return 0, false
	}
	return v.Spot, true
}

func parseTHB(body []byte) (float64, bool) {
	var v struct {
		Rates map[string]float64 `json:"rates"`
	}
	if json.Unmarshal(body, &v) != nil || v.Rates["THB"] <= 0 {
		return 0, false
	}
	return v.Rates["THB"], true
}

// estimateBar converts world spot to an approximate Thai 96.5% bar price per baht-weight.
func estimateBar(spotUSDPerOz, thbPerUSD float64) float64 {
	return spotUSDPerOz * thbPerUSD * (gramsPerBaht / gramsPerTroyOz) * thaiGoldPurity
}
