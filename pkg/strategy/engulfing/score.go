package engulfing

import "github.com/tgragnato/orbiter/pkg/ohlc"

// Score returns ±0.8 when an engulfing pattern is detected, 0 otherwise.
// Bearish engulfing (counter-trend long): +0.8.
// Bullish engulfing (counter-trend short): -0.8.
func (d *Engulfing) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < 2 {
		return 0
	}
	if d.isBearishEngulfingCandle(closedCandles) {
		return 0.8
	}
	if d.isBullishEngulfingCandle(closedCandles) {
		return -0.8
	}
	return 0
}
