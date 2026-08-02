package harami

import "github.com/tgragnato/orbiter/pkg/ohlc"

// Score returns +0.8 when a bullish harami long pattern is detected, 0 otherwise.
// Harami is a long-only setup so the score is never negative.
func (h *Harami) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < 2 {
		return 0
	}
	second := closedCandles[len(closedCandles)-2]
	first := closedCandles[len(closedCandles)-1]
	if h.isHaramiLong(second, first) {
		return 0.8
	}
	return 0
}
