package data

import (
	"context"
	"time"
)

// CandleStorer is implemented by any store that can persist and retrieve EOD
// candles. Used by CachingProvider to avoid repeated Yahoo Finance requests for
// historical data that is already available locally.
type CandleStorer interface {
	// GetCachedCandles returns persisted candles for symbol in [from, to].
	GetCachedCandles(ctx context.Context, symbol string, from, to time.Time) ([]Candle, error)
	// UpsertCandles stores candles, updating existing rows on (symbol, candle_date) conflict.
	UpsertCandles(ctx context.Context, candles []Candle) error
	// LatestCandleDate returns the most recent cached candle date for symbol,
	// or zero time if no candles are cached for that symbol.
	LatestCandleDate(ctx context.Context, symbol string) (time.Time, error)
}

// CachingProvider wraps a DataProvider with a DB-backed EOD candle cache.
//
// On the first GetEOD call for a symbol the full history is fetched from the
// upstream provider and written to the store. Subsequent calls only fetch the
// days after the last cached date (typically 1–2 trading days per day), so
// Yahoo Finance is never hit for data that is already local.
//
// All store writes are best-effort: a write failure does not surface as a
// GetEOD error — the call returns the fresh upstream data instead.
type CachingProvider struct {
	upstream DataProvider
	store    CandleStorer
}

// NewCachingProvider returns a CachingProvider backed by store.
func NewCachingProvider(upstream DataProvider, store CandleStorer) *CachingProvider {
	return &CachingProvider{upstream: upstream, store: store}
}

// GetEOD implements DataProvider with a DB-backed cache.
//
//  1. Query the latest cached date for ticker.
//  2. If nothing is cached: fetch the full range from upstream, cache it, return.
//  3. If the cache is current (latest ≥ yesterday): return entirely from cache.
//  4. Otherwise: fetch only the delta [latest+1, to] from upstream, cache it,
//     then serve the full [from, to] range from the DB.
func (c *CachingProvider) GetEOD(ticker string, from, to time.Time) ([]Candle, error) {
	ctx := context.Background()

	latest, err := c.store.LatestCandleDate(ctx, ticker)
	if err != nil {
		// Cache unavailable — fall back to upstream for this call only.
		return c.upstream.GetEOD(ticker, from, to)
	}

	yesterday := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -1)

	if latest.IsZero() {
		// No cached data for this symbol — full fetch from upstream.
		candles, err := c.upstream.GetEOD(ticker, from, to)
		if err != nil {
			return nil, err
		}
		if len(candles) > 0 {
			_ = c.store.UpsertCandles(ctx, candles) // best-effort; don't fail the call
		}
		return candles, nil
	}

	// Cache exists. Fetch the incremental update when stale.
	if latest.Before(yesterday) {
		fetchFrom := latest.AddDate(0, 0, 1)
		fresh, fetchErr := c.upstream.GetEOD(ticker, fetchFrom, to)
		if fetchErr == nil && len(fresh) > 0 {
			_ = c.store.UpsertCandles(ctx, fresh) // best-effort
		}
	}

	// Serve entirely from the DB cache (includes newly inserted rows).
	return c.store.GetCachedCandles(ctx, ticker, from, to)
}
