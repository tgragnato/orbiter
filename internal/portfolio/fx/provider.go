package fx

import "time"

// Provider fetches historical exchange rate series from an external source.
type Provider interface {
	// GetRates returns daily rates for the pair (BaseCurrency→QuoteCurrency)
	// over [from, to]. Callers may receive fewer points than requested when
	// the provider has no data for certain dates (weekends, public holidays).
	GetRates(base, quote string, from, to time.Time) ([]Rate, error)
}
