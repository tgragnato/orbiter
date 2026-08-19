//nolint:testpackage // accesses unexported field provider.baseURL
package data

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newYahooProviderWithBaseURL(client *http.Client, baseURL string) *YahooProvider {
	provider := NewYahooProvider(client)
	provider.baseURL = strings.TrimRight(baseURL, "/")

	return provider
}

func TestYahooProviderGetEODParsesAdjustedCloseDividendsAndSplits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v8/finance/chart/VWCE.DE") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if got := r.URL.Query().Get("events"); got != "div,split" {
			t.Fatalf("events query = %q, want div,split", got)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(yahooFixtureOK))
	}))
	defer server.Close()

	provider := newYahooProviderWithBaseURL(server.Client(), server.URL)
	from := time.Unix(1704067200, 0).UTC()
	until := time.Unix(1704239999, 0).UTC()

	candles, err := provider.GetEOD("VWCE.DE", from, until)
	if err != nil {
		t.Fatalf("GetEOD() error = %v", err)
	}

	if len(candles) != 2 {
		t.Fatalf("len(candles) = %d, want 2", len(candles))
	}

	if candles[0].AdjustedClose != 100.5 {
		t.Fatalf("first adjusted close = %f, want 100.5", candles[0].AdjustedClose)
	}

	if candles[0].SplitFactor != 2 {
		t.Fatalf("first split factor = %f, want 2", candles[0].SplitFactor)
	}

	if candles[1].CashDividend != 1.23 {
		t.Fatalf("second cash dividend = %f, want 1.23", candles[1].CashDividend)
	}
}

func TestYahooProviderGetEODSkipsNullCandleData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(yahooFixtureWithNull))
	}))
	defer server.Close()

	provider := newYahooProviderWithBaseURL(server.Client(), server.URL)
	from := time.Unix(1704067200, 0).UTC()
	until := time.Unix(1704239999, 0).UTC()

	candles, err := provider.GetEOD("SWDA.MI", from, until)
	if err != nil {
		t.Fatalf("GetEOD() error = %v", err)
	}

	if len(candles) != 1 {
		t.Fatalf("len(candles) = %d, want 1", len(candles))
	}
}

func TestYahooProviderGetEODHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		http.Error(writer, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	provider := newYahooProviderWithBaseURL(server.Client(), server.URL)

	_, err := provider.GetEOD("VWCE.DE", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("GetEOD() error = %v, want status 502", err)
	}
}

func TestYahooProviderGetEODValidationAndDecodeErrors(t *testing.T) {
	t.Parallel()

	provider := NewYahooProvider(&http.Client{
		Timeout: 2 * time.Second,
	})

	_, emptyTickerErr := provider.GetEOD("", time.Now().Add(-time.Hour), time.Now())
	if emptyTickerErr == nil {
		t.Fatalf("GetEOD() error = nil for empty ticker")
	}

	_, badRangeErr := provider.GetEOD("VWCE.DE", time.Now(), time.Now().Add(-time.Hour))
	if badRangeErr == nil {
		t.Fatalf("GetEOD() error = nil for invalid date range")
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte("{not-json"))
	}))
	defer server.Close()

	provider = newYahooProviderWithBaseURL(server.Client(), server.URL)

	_, decodeErr := provider.GetEOD("VWCE.DE", time.Now().Add(-24*time.Hour), time.Now())
	if decodeErr == nil {
		t.Fatalf("GetEOD() error = nil for malformed json")
	}
}

func TestYahooProviderGetEODAPIErrorPayloadAndMissingResult(t *testing.T) {
	t.Parallel()

	apiErrorServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"chart":{"result":null,"error":{"description":"symbol not found"}}}`))
	}))
	defer apiErrorServer.Close()

	provider := newYahooProviderWithBaseURL(apiErrorServer.Client(), apiErrorServer.URL)

	_, apiErr := provider.GetEOD("MISSING", time.Now().Add(-24*time.Hour), time.Now())
	if apiErr == nil || !strings.Contains(apiErr.Error(), "symbol not found") {
		t.Fatalf("GetEOD() error = %v, want api error", apiErr)
	}

	missingResultServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"chart":{"result":[],"error":null}}`))
	}))
	defer missingResultServer.Close()

	provider = newYahooProviderWithBaseURL(missingResultServer.Client(), missingResultServer.URL)

	_, missingErr := provider.GetEOD("VWCE.DE", time.Now().Add(-24*time.Hour), time.Now())
	if missingErr == nil || !strings.Contains(missingErr.Error(), "no result") {
		t.Fatalf("GetEOD() error = %v, want missing result error", missingErr)
	}
}

func TestYahooProviderWithAPIKey(t *testing.T) {
	t.Parallel()

	apiKeyHeader := ""

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		apiKeyHeader = r.Header.Get("X-Api-Key")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(yahooFixtureOK))
	}))
	defer server.Close()

	provider := newYahooProviderWithBaseURL(server.Client(), server.URL).WithAPIKey("secret-api-key")
	from := time.Unix(1704067200, 0).UTC()
	until := time.Unix(1704239999, 0).UTC()

	_, err := provider.GetEOD("VWCE.DE", from, until)
	if err != nil {
		t.Fatalf("GetEOD() error = %v", err)
	}

	if apiKeyHeader != "secret-api-key" {
		t.Fatalf("apiKeyHeader = %q, want secret-api-key", apiKeyHeader)
	}

	// Also test FX adapter with shared provider
	fxAdapter := NewYahooFXProviderWithProvider(provider)

	_, err = fxAdapter.GetRates("EUR", "USD", from, until)
	if err != nil {
		t.Fatalf("GetRates() error = %v", err)
	}

	if apiKeyHeader != "secret-api-key" {
		t.Fatalf("apiKeyHeader in FX = %q, want secret-api-key", apiKeyHeader)
	}
}

const yahooFixtureOK = `
{
  "chart": {
    "result": [
      {
        "timestamp": [1704067200, 1704153600],
        "indicators": {
          "quote": [
            {
              "open": [100.0, 101.0],
              "high": [101.0, 102.0],
              "low": [99.0, 100.0],
              "close": [100.2, 101.2],
              "volume": [1000, 1100]
            }
          ],
          "adjclose": [
            {
              "adjclose": [100.5, 101.5]
            }
          ]
        },
        "events": {
          "dividends": {
            "1704153600": {
              "date": 1704153600,
              "amount": 1.23
            }
          },
          "splits": {
            "1704067200": {
              "date": 1704067200,
              "numerator": 2,
              "denominator": 1
            }
          }
        }
      }
    ],
    "error": null
  }
}`

const yahooFixtureWithNull = `
{
  "chart": {
    "result": [
      {
        "timestamp": [1704067200, 1704153600],
        "indicators": {
          "quote": [
            {
              "open": [100.0, null],
              "high": [101.0, 102.0],
              "low": [99.0, 100.0],
              "close": [100.2, 101.2],
              "volume": [1000, 1100]
            }
          ],
          "adjclose": [
            {
              "adjclose": [100.5, 101.5]
            }
          ]
        },
        "events": {
          "dividends": {},
          "splits": {}
        }
      }
    ],
    "error": null
  }
}`
