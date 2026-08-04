package stochrsi

import (
	"github.com/tgragnato/orbiter/pkg/indicator/stochrsi"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Score returns a directional conviction in [-1.0, +1.0] from the average of
// the StochRSI K and D lines. Above upperThreshold → -1.0; below lowerThreshold
// → +1.0. Interpolated linearly in between.
func (d *RSI) Score(_ []*ohlc.OHLC) float64 {
	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		return 0
	}
	k := rsiValueMap[stochrsi.ValueK]
	dv := rsiValueMap[stochrsi.ValueD]
	avg := (k + dv) / 2.0

	const mid = 50.0
	switch {
	case avg >= upperThreshold:
		return -1.0
	case avg <= lowerThreshold:
		return 1.0
	case avg < mid:
		return (mid - avg) / (mid - lowerThreshold)
	default:
		return -(avg - mid) / (upperThreshold - mid)
	}
}
