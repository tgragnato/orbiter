package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultYahooBaseURL = "https://query1.finance.yahoo.com"

// YahooProvider fetches EOD data from Yahoo Finance chart API.
type YahooProvider struct {
	mu      sync.RWMutex
	client  *http.Client
	baseURL string
	apiKey  string
}

// NewYahooProvider creates a provider using a production Yahoo endpoint.
func NewYahooProvider(client *http.Client) *YahooProvider {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &YahooProvider{client: client, baseURL: defaultYahooBaseURL}
}

// SetAPIKey dynamically updates the API key used for authenticated Yahoo endpoints.
func (p *YahooProvider) SetAPIKey(apiKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apiKey = strings.TrimSpace(apiKey)
}

// APIKey returns the current API key.
func (p *YahooProvider) APIKey() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.apiKey
}

// WithAPIKey sets an optional API key for authenticated Yahoo endpoints.
func (p *YahooProvider) WithAPIKey(apiKey string) *YahooProvider {
	p.SetAPIKey(apiKey)
	return p
}

// GetEOD returns daily candles including adjusted close and corporate action fields.
func (p *YahooProvider) GetEOD(ticker string, from, to time.Time) ([]Candle, error) {
	ticker = strings.TrimSpace(ticker)
	if ticker == "" {
		return nil, errors.New("ticker is required")
	}
	if to.Before(from) {
		return nil, errors.New("invalid date range: to before from")
	}

	requestURL, err := p.buildURL(ticker, from, to)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.5.2 Safari/605.1.15")
	if key := p.APIKey(); key != "" {
		req.Header.Set("X-API-KEY", key)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("yahoo request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var payload yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode yahoo response: %w", err)
	}

	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo api error: %s", payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) == 0 {
		return nil, errors.New("yahoo response has no result")
	}

	result := payload.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 || len(result.Indicators.AdjClose) == 0 {
		return nil, errors.New("yahoo response missing quote/adjclose data")
	}

	quote := result.Indicators.Quote[0]
	adjClose := result.Indicators.AdjClose[0]
	// Dividends and splits are keyed by midnight-UTC unix timestamp so that
	// lookup works regardless of the intraday time Yahoo attaches to each event.
	dividends := parseDividends(result.Events.Dividends)
	splits := parseSplits(result.Events.Splits)

	candles := make([]Candle, 0, len(result.Timestamp))
	matchedDivDates := make(map[int64]bool, len(dividends))

	for i, ts := range result.Timestamp {
		if i >= len(quote.Open) || i >= len(quote.High) || i >= len(quote.Low) || i >= len(quote.Close) || i >= len(quote.Volume) || i >= len(adjClose.AdjClose) {
			continue
		}
		if quote.Open[i] == nil || quote.High[i] == nil || quote.Low[i] == nil || quote.Close[i] == nil || quote.Volume[i] == nil || adjClose.AdjClose[i] == nil {
			continue
		}

		// Normalise candle timestamp to midnight UTC for dividend/split lookup.
		dateTS := time.Unix(ts, 0).UTC().Truncate(24 * time.Hour).Unix()

		candle := Candle{
			Ticker:        ticker,
			Time:          time.Unix(ts, 0).UTC(),
			Open:          *quote.Open[i],
			High:          *quote.High[i],
			Low:           *quote.Low[i],
			Close:         *quote.Close[i],
			AdjustedClose: *adjClose.AdjClose[i],
			Volume:        *quote.Volume[i],
			SplitFactor:   1,
			Currency:      strings.ToUpper(strings.TrimSpace(result.Meta.Currency)),
		}
		if dividend, ok := dividends[dateTS]; ok {
			candle.CashDividend = dividend
			matchedDivDates[dateTS] = true
		}
		if splitFactor, ok := splits[dateTS]; ok {
			candle.SplitFactor = splitFactor
		}
		candles = append(candles, candle)
	}

	// Dividend ex-dates sometimes fall on non-trading days (weekends, public
	// holidays). Yahoo still emits the event but no price candle exists for
	// that date, so the lookup above never fires. Emit a synthetic candle so
	// ComputeDividendIncomes does not silently drop these dividends.
	for divDateTS, amount := range dividends {
		if matchedDivDates[divDateTS] {
			continue
		}
		candles = append(candles, Candle{
			Ticker:       ticker,
			Time:         time.Unix(divDateTS, 0).UTC(),
			CashDividend: amount,
			SplitFactor:  1,
			Currency:     strings.ToUpper(strings.TrimSpace(result.Meta.Currency)),
		})
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Time.Before(candles[j].Time)
	})

	return candles, nil
}

func (p *YahooProvider) buildURL(ticker string, from, to time.Time) (string, error) {
	base, err := url.Parse(p.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v8/finance/chart/" + url.PathEscape(ticker)

	query := base.Query()
	query.Set("interval", "1d")
	query.Set("events", "div,split")
	query.Set("includeAdjustedClose", "true")
	query.Set("period1", fmt.Sprintf("%d", from.UTC().Unix()))
	query.Set("period2", fmt.Sprintf("%d", to.UTC().Unix()))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func parseDividends(raw map[string]yahooDividendEvent) map[int64]float64 {
	out := make(map[int64]float64, len(raw))
	for _, event := range raw {
		dateTS := time.Unix(event.Date, 0).UTC().Truncate(24 * time.Hour).Unix()
		out[dateTS] = event.Amount
	}
	return out
}

func parseSplits(raw map[string]yahooSplitEvent) map[int64]float64 {
	out := make(map[int64]float64, len(raw))
	for _, event := range raw {
		factor := 1.0
		if event.Denominator > 0 {
			factor = event.Numerator / event.Denominator
		}
		dateTS := time.Unix(event.Date, 0).UTC().Truncate(24 * time.Hour).Unix()
		out[dateTS] = factor
	}
	return out
}

type yahooChartResponse struct {
	Chart struct {
		Result []yahooResult `json:"result"`
		Error  *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type yahooResult struct {
	Meta       yahooMeta    `json:"meta"`
	Timestamp  []int64      `json:"timestamp"`
	Indicators yahooMetrics `json:"indicators"`
	Events     yahooEvents  `json:"events"`
}

type yahooMeta struct {
	Currency string `json:"currency"`
}

type yahooMetrics struct {
	Quote []struct {
		Open   []*float64 `json:"open"`
		High   []*float64 `json:"high"`
		Low    []*float64 `json:"low"`
		Close  []*float64 `json:"close"`
		Volume []*int64   `json:"volume"`
	} `json:"quote"`
	AdjClose []struct {
		AdjClose []*float64 `json:"adjclose"`
	} `json:"adjclose"`
}

type yahooEvents struct {
	Dividends map[string]yahooDividendEvent `json:"dividends"`
	Splits    map[string]yahooSplitEvent    `json:"splits"`
}

type yahooDividendEvent struct {
	Date   int64   `json:"date"`
	Amount float64 `json:"amount"`
}

type yahooSplitEvent struct {
	Date        int64   `json:"date"`
	Numerator   float64 `json:"numerator"`
	Denominator float64 `json:"denominator"`
}
