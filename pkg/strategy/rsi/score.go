package rsi

import "github.com/tgragnato/orbiter/pkg/ohlc"

// Score returns a directional conviction in [-1.0, +1.0] based on the current
// RSI value. RSI ≤ lowerThreshold → +1.0 (oversold/buy); RSI ≥ upperThreshold
// → -1.0 (overbought/sell). SMA confirmation is not required here so the ML
// ensemble can learn to combine indicators on its own.
func (d *RSI) Score(_ []*ohlc.OHLC) float64 {
	rsiVal, err := d.getRSIValues()
	if err != nil {
		return 0
	}
	const mid = 50.0
	switch {
	case rsiVal <= lowerThreshold:
		return 1.0
	case rsiVal >= upperThreshold:
		return -1.0
	case rsiVal < mid:
		return (mid - rsiVal) / (mid - lowerThreshold)
	default:
		return -(rsiVal - mid) / (upperThreshold - mid)
	}
}
