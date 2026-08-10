package feed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

// --- fakes ---

type fakePriceStore struct {
	symbols    []string
	symbolsErr error
	updated    map[string]float64
	updateErr  error
}

func (f *fakePriceStore) ActiveSymbols(_ context.Context) ([]string, error) {
	return f.symbols, f.symbolsErr
}

func (f *fakePriceStore) UpdateMarketPrice(_ context.Context, symbol string, price float64) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.updated == nil {
		f.updated = make(map[string]float64)
	}
	f.updated[symbol] = price
	return nil
}

func (f *fakePriceStore) UpdateHoldingCurrency(_ context.Context, _, _ string) error {
	return nil
}

type fakeProvider struct {
	candles map[string][]data.Candle
	err     error
}

func (f *fakeProvider) GetEOD(ticker string, _, _ time.Time) ([]data.Candle, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.candles[ticker], nil
}

// --- tests ---

func TestUpdaterRefreshUpdatesActiveSymbols(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbols: []string{"VWCE.MI", "XMAD.MI"}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"VWCE.MI": {{AdjustedClose: 98.50, Close: 98.00}},
		"XMAD.MI": {{AdjustedClose: 0, Close: 120.00}}, // AdjustedClose=0 → falls back to Close
	}}

	u := New(store, provider, time.Hour)
	u.refresh(context.Background())

	if store.updated["VWCE.MI"] != 98.50 {
		t.Fatalf("VWCE.MI price = %f, want 98.50", store.updated["VWCE.MI"])
	}
	if store.updated["XMAD.MI"] != 120.00 {
		t.Fatalf("XMAD.MI price = %f, want 120.00 (fallback to Close)", store.updated["XMAD.MI"])
	}
}

func TestUpdaterRefreshSkipsSymbolsWithNoCandles(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbols: []string{"UNKNOWN.MI"}}
	provider := &fakeProvider{candles: map[string][]data.Candle{}} // no candles for any symbol

	u := New(store, provider, time.Hour)
	u.refresh(context.Background())

	if len(store.updated) != 0 {
		t.Fatalf("expected no updates, got %v", store.updated)
	}
}

func TestUpdaterRefreshActiveSymbolsError(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbolsErr: errors.New("db error")}
	provider := &fakeProvider{}

	u := New(store, provider, time.Hour)
	// Should not panic and should log the error gracefully.
	u.refresh(context.Background())

	if len(store.updated) != 0 {
		t.Fatalf("expected no updates on error, got %v", store.updated)
	}
}

func TestUpdaterRefreshProviderError(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbols: []string{"ERR.MI"}}
	provider := &fakeProvider{err: errors.New("network error")}

	u := New(store, provider, time.Hour)
	u.refresh(context.Background())

	if len(store.updated) != 0 {
		t.Fatalf("expected no updates on provider error, got %v", store.updated)
	}
}

func TestUpdaterRefreshZeroPriceSkipped(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbols: []string{"ZERO.MI"}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"ZERO.MI": {{AdjustedClose: 0, Close: 0}},
	}}

	u := New(store, provider, time.Hour)
	u.refresh(context.Background())

	if len(store.updated) != 0 {
		t.Fatalf("zero price should be skipped, got %v", store.updated)
	}
}

func TestUpdaterRefreshEmptySymbolList(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbols: nil}
	provider := &fakeProvider{}

	u := New(store, provider, time.Hour)
	u.refresh(context.Background())

	if len(store.updated) != 0 {
		t.Fatalf("empty symbol list should produce no updates, got %v", store.updated)
	}
}

func TestUpdaterRunStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	store := &fakePriceStore{symbols: []string{}}
	provider := &fakeProvider{}
	u := New(store, provider, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		u.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}
