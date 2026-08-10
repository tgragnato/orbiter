package portfolio

// AllocationType identifies whether an asset belongs to core or satellite sleeve.
type AllocationType string

const (
	// AllocationCore marks a holding as part of the core sleeve.
	AllocationCore AllocationType = "CORE"
	// AllocationSatellite marks a holding as part of the satellite sleeve.
	AllocationSatellite AllocationType = "SATELLITE"
)

// Holding is one portfolio row shown in the unified holdings table.
type Holding struct {
	ID             int64
	Symbol         string
	Quantity       float64
	MarketPrice    float64
	PMC            float64 // weighted average purchase cost (Prezzo Medio di Carico)
	AllocationType AllocationType
	TAAEnabled     bool   // when false the TAA engine skips this holding
	Currency       string // ISO 4217 quotation currency of the asset (e.g. "USD", "EUR")
}

// NAV computes current position net asset value in the asset's local currency.
func (h Holding) NAV() float64 {
	return h.Quantity * h.MarketPrice
}

// NAVInBase converts the local-currency NAV to the portfolio base currency.
// fxRate must be the rate "how many base-currency units per 1 unit of h.Currency"
// (i.e. Rate{Base: baseCurrency, Quote: h.Currency}.Rate inverted, or use
// fx.Service.RateFor(ctx, h.Currency, baseCurrency, date)).
func (h Holding) NAVInBase(fxRate float64) float64 {
	return h.NAV() * fxRate
}

// ToggleAllocation swaps CORE and SATELLITE allocation tags.
func (h Holding) ToggleAllocation() AllocationType {
	if h.AllocationType == AllocationCore {
		return AllocationSatellite
	}
	return AllocationCore
}

// Summary describes top-bar portfolio breakdown metrics.
type Summary struct {
	TotalNAV          float64
	CoreNAV           float64
	SatelliteNAV      float64
	CorePercent       float64
	SatellitePercent  float64
	TotalHoldingsRows int
}

// BuildSummary calculates NAV totals and allocation weights.
func BuildSummary(holdings []Holding) Summary {
	var summary Summary
	summary.TotalHoldingsRows = len(holdings)

	for _, holding := range holdings {
		nav := holding.NAV()
		summary.TotalNAV += nav
		if holding.AllocationType == AllocationCore {
			summary.CoreNAV += nav
		} else {
			summary.SatelliteNAV += nav
		}
	}

	if summary.TotalNAV > 0 {
		summary.CorePercent = summary.CoreNAV / summary.TotalNAV * 100
		summary.SatellitePercent = summary.SatelliteNAV / summary.TotalNAV * 100
	}

	return summary
}
