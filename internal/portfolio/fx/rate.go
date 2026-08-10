// Package fx provides historical and live foreign-exchange rate lookup and
// currency conversion for multi-currency portfolio valuation.
package fx

import (
	"time"
)

// Rate stores one EOD exchange rate between two ISO 4217 currency codes.
//
// Convention: 1 unit of BaseCurrency = Rate units of QuoteCurrency.
// For example {Base:"EUR", Quote:"USD", Rate:1.08} means 1 EUR = 1.08 USD.
// The Yahoo Finance ticker for this pair is "EURUSD=X".
type Rate struct {
	BaseCurrency  string
	QuoteCurrency string
	Date          time.Time
	Rate          float64
}
