package portfolio

import (
	"context"
	"time"
)

// WatchlistItem is a satellite asset tracked for potential entry.
// It has no position yet — the TAA engine emits TypeBuy signals when
// the ML conviction score exceeds the entry friction gate.
type WatchlistItem struct {
	ID          int64
	Symbol      string
	MarketPrice float64 // last EOD price updated by the price feed; 0 until first fetch
	Currency    string  // ISO 4217 quotation currency (e.g. "EUR", "USD"); empty until first fetch
	CreatedAt   time.Time
}

// WatchlistStore persists the satellite asset watchlist.
type WatchlistStore interface {
	ListWatchlist(ctx context.Context) ([]WatchlistItem, error)
	AddToWatchlist(ctx context.Context, symbol string) error
	RemoveFromWatchlist(ctx context.Context, symbol string) error
	// ListWatchlistSymbols returns the ticker strings for the watchlist.
	// Used by the featurizer to include unowned assets in ML scoring so
	// the TAA entry path can emit TypeBuy signals for them.
	ListWatchlistSymbols(ctx context.Context) ([]string, error)
	// UpdateWatchlistPrice stores the latest EOD market price and quotation currency
	// for a watchlist item. Called by the price feed on every refresh cycle.
	UpdateWatchlistPrice(ctx context.Context, symbol string, price float64, currency string) error
}
