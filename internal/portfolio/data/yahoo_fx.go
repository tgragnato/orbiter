package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio/fx"
)

// GetFXRates implements fx.Provider. It fetches daily closing exchange rates for
// the (base, quote) currency pair using Yahoo Finance FX tickers of the form
// "{base}{quote}=X" (e.g. "EURUSD=X"). If that ticker yields no data the method
// automatically retries the inverse pair and reciprocates each rate so the
// caller always receives values in the requested direction.
//
// An identical base and quote returns a single unit-rate without any HTTP call.
func (p *YahooProvider) GetFXRates(base, quote string, from, to time.Time) ([]fx.Rate, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if base == "" || quote == "" {
		return nil, errors.New("fx: base and quote currencies are required")
	}
	if to.Before(from) {
		return nil, errors.New("fx: invalid date range: to before from")
	}
	if base == quote {
		return []fx.Rate{{
			BaseCurrency:  base,
			QuoteCurrency: quote,
			Date:          from.UTC().Truncate(24 * time.Hour),
			Rate:          1,
		}}, nil
	}

	// Try the direct ticker first; fall back to inverse with reciprocation.
	directTicker := base + quote + "=X"
	rates, err := p.fetchFXRates(base, quote, directTicker, from, to, false)
	if err == nil && len(rates) > 0 {
		return rates, nil
	}

	inverseTicker := quote + base + "=X"
	return p.fetchFXRates(base, quote, inverseTicker, from, to, true)
}

func (p *YahooProvider) fetchFXRates(base, quote, ticker string, from, to time.Time, reciprocate bool) ([]fx.Rate, error) {
	reqURL, err := p.buildFXURL(ticker, from, to)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.5.2 Safari/605.1.15")
	if p.apiKey != "" {
		req.Header.Set("X-API-KEY", p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fx yahoo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fx yahoo request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var payload fxChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("fx: decode yahoo response: %w", err)
	}
	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("fx: yahoo api error: %s", payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) == 0 {
		return nil, errors.New("fx: yahoo response has no result")
	}

	result := payload.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return nil, errors.New("fx: yahoo response missing quote data")
	}

	closes := result.Indicators.Quote[0].Close
	rates := make([]fx.Rate, 0, len(result.Timestamp))

	for i, ts := range result.Timestamp {
		if i >= len(closes) || closes[i] == nil {
			continue
		}
		closePrice := *closes[i]
		if closePrice <= 0 {
			continue
		}

		rate := closePrice
		if reciprocate {
			rate = 1 / closePrice
		}

		rates = append(rates, fx.Rate{
			BaseCurrency:  base,
			QuoteCurrency: quote,
			Date:          time.Unix(ts, 0).UTC().Truncate(24 * time.Hour),
			Rate:          rate,
		})
	}

	return rates, nil
}

// buildFXURL constructs a Yahoo Finance chart URL for an FX pair ticker.
// Unlike buildURL it does not request dividend/split events or adjusted close,
// which are irrelevant for currency pair data.
func (p *YahooProvider) buildFXURL(ticker string, from, to time.Time) (string, error) {
	base, err := url.Parse(p.baseURL)
	if err != nil {
		return "", fmt.Errorf("fx: invalid base URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v8/finance/chart/" + url.PathEscape(ticker)

	query := base.Query()
	query.Set("interval", "1d")
	query.Set("period1", fmt.Sprintf("%d", from.UTC().Unix()))
	query.Set("period2", fmt.Sprintf("%d", to.UTC().Unix()))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

// YahooFXAdapter wraps YahooProvider and implements fx.Provider, exposing FX rate
// fetching via the same HTTP client and Yahoo base URL. This eliminates the
// duplicate yahoo provider that previously lived in the fx package.
type YahooFXAdapter struct {
	provider *YahooProvider
}

// NewYahooFXProvider returns an fx.Provider backed by the shared YahooProvider.
// The caller supplies the HTTP client (usually with a 30 s timeout); nil uses the
// provider's own default.
func NewYahooFXProvider(client *http.Client) *YahooFXAdapter {
	return &YahooFXAdapter{provider: NewYahooProvider(client)}
}

// NewYahooFXProviderWithProvider returns an fx.Provider backed by an existing YahooProvider instance.
func NewYahooFXProviderWithProvider(provider *YahooProvider) *YahooFXAdapter {
	if provider == nil {
		provider = NewYahooProvider(nil)
	}
	return &YahooFXAdapter{provider: provider}
}

// GetRates implements fx.Provider by delegating to YahooProvider.GetFXRates.
func (a *YahooFXAdapter) GetRates(base, quote string, from, to time.Time) ([]fx.Rate, error) {
	return a.provider.GetFXRates(base, quote, from, to)
}

// fxChartResponse is a minimal decode of the Yahoo chart API envelope for FX pairs.
// It is separate from yahooChartResponse to keep the FX response lean — FX tickers
// never carry dividend or split event data.
type fxChartResponse struct {
	Chart struct {
		Result []fxResult `json:"result"`
		Error  *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type fxResult struct {
	Timestamp  []int64      `json:"timestamp"`
	Indicators fxIndicators `json:"indicators"`
}

type fxIndicators struct {
	Quote []struct {
		Close []*float64 `json:"close"`
	} `json:"quote"`
}
