package fx

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// cacheKey uniquely identifies a (base, quote, UTC-day) tuple.
type cacheKey struct {
	base  string
	quote string
	day   time.Time // always UTC midnight
}

// Service combines a Provider and a Store to offer currency conversion with an
// in-memory cache. Historical rates (dates strictly before today) are cached
// permanently because they cannot change. Today's rate is re-fetched on every
// SyncRates call so it stays current.
type Service struct {
	provider Provider
	store    Store
	cache    map[cacheKey]float64
	mu       sync.RWMutex
}

// NewService creates an FX service backed by the given provider and store.
func NewService(provider Provider, store Store) *Service {
	return &Service{
		provider: provider,
		store:    store,
		cache:    make(map[cacheKey]float64),
	}
}

// Convert returns amount expressed in fromCurrency converted to toCurrency on
// the given date, using the rate convention 1 base = Rate quote.
//
// Lookup order:
//  1. In-memory cache.
//  2. DB store (exact date, then nearest prior date for weekend/holiday fill).
//  3. Live fetch from provider, persisted to store and cache.
//
// If fromCurrency == toCurrency, amount is returned unchanged.
func (s *Service) Convert(ctx context.Context, amount float64, fromCurrency, toCurrency string, date time.Time) (float64, error) {
	from := strings.ToUpper(strings.TrimSpace(fromCurrency))
	to := strings.ToUpper(strings.TrimSpace(toCurrency))
	if from == to {
		return amount, nil
	}

	rate, err := s.rateFor(ctx, from, to, date)
	if err != nil {
		return 0, err
	}
	return amount * rate, nil
}

// SyncRates fetches the rate series for (base→quote) over [from, to] from the
// provider, persists them to the store, and warms the in-memory cache. Today's
// rates are evicted from the cache before fetching so they are always fresh.
func (s *Service) SyncRates(ctx context.Context, base, quote string, from, to time.Time) error {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if base == quote {
		return nil
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	s.mu.Lock()
	delete(s.cache, cacheKey{base: base, quote: quote, day: today})
	s.mu.Unlock()

	rates, err := s.provider.GetRates(base, quote, from, to)
	if err != nil {
		return fmt.Errorf("fx: sync rates %s/%s: %w", base, quote, err)
	}
	if len(rates) == 0 {
		return nil
	}

	if err := s.store.UpsertRates(ctx, rates); err != nil {
		return err
	}

	s.mu.Lock()
	for _, r := range rates {
		key := cacheKey{base: r.BaseCurrency, quote: r.QuoteCurrency, day: r.Date.UTC().Truncate(24 * time.Hour)}
		s.cache[key] = r.Rate
	}
	s.mu.Unlock()

	return nil
}

// rateFor resolves a rate through cache → store → provider, applying an
// inverse fallback when the direct pair is unavailable.
func (s *Service) rateFor(ctx context.Context, from, to string, date time.Time) (float64, error) {
	day := date.UTC().Truncate(24 * time.Hour)

	// 1. In-memory cache (direct).
	s.mu.RLock()
	if rate, ok := s.cache[cacheKey{base: from, quote: to, day: day}]; ok {
		s.mu.RUnlock()
		return rate, nil
	}
	s.mu.RUnlock()

	// 2. DB — exact date.
	if rate, ok, err := s.store.GetRate(ctx, from, to, day); err != nil {
		return 0, err
	} else if ok {
		s.warmCache(from, to, day, rate)
		return rate, nil
	}

	// 3. DB — nearest prior date (forward-fill over weekends/holidays).
	if rate, ok, err := s.store.GetRateOnOrBefore(ctx, from, to, day); err != nil {
		return 0, err
	} else if ok {
		// Cache under the requested date so subsequent lookups hit memory.
		s.warmCache(from, to, day, rate)
		return rate, nil
	}

	// 4. Live fetch from provider.
	rates, err := s.provider.GetRates(from, to, day.AddDate(0, 0, -7), day)
	if err != nil {
		return 0, fmt.Errorf("fx: live fetch %s/%s: %w", from, to, err)
	}
	if len(rates) == 0 {
		return 0, fmt.Errorf("fx: no rate available for %s/%s on %s", from, to, day.Format("2006-01-02"))
	}

	if storeErr := s.store.UpsertRates(ctx, rates); storeErr != nil {
		slog.Warn("fx: live rate persist failed, using in-memory value", "from", from, "to", to, "error", storeErr)
	}

	// Use the last (most recent) rate in the fetched series.
	last := rates[len(rates)-1]
	s.warmCache(from, to, day, last.Rate)
	return last.Rate, nil
}

func (s *Service) warmCache(from, to string, day time.Time, rate float64) {
	s.mu.Lock()
	s.cache[cacheKey{base: from, quote: to, day: day}] = rate
	s.mu.Unlock()
}
