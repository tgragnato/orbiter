package data

import "time"

// Candle contains one EOD market data point enriched with corporate actions.
type Candle struct {
	Ticker        string
	Time          time.Time
	Open          float64
	High          float64
	Low           float64
	Close         float64
	AdjustedClose float64
	Volume        int64
	SplitFactor   float64
	CashDividend  float64
}

// DataProvider retrieves EOD candles for a ticker and date range.
type DataProvider interface {
	GetEOD(ticker string, from, to time.Time) ([]Candle, error)
}
