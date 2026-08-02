package heikinashi

import (
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Score returns ±0.7 when two consecutive Heikin-Ashi candles confirm a
// direction, 0 when undecided. Direction is taken from the most recently
// updated currentDirection field.
func (ha *HeikinAshi) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < 3 {
		return 0
	}
	haPrev := ohlc.ToHeikinAshi(closedCandles[len(closedCandles)-3], closedCandles[len(closedCandles)-2])
	haNow := ohlc.ToHeikinAshi(closedCandles[len(closedCandles)-2], closedCandles[len(closedCandles)-1])

	longConfirmed := isLongCandle(haNow) && isLongCandle(haPrev) && haNow.Close.GreaterThan(haPrev.Close)
	shortConfirmed := isShortCandle(haNow) && isShortCandle(haPrev) && haNow.Close.LessThan(haPrev.Close)

	switch {
	case longConfirmed:
		if ha.currentDirection != nil && *ha.currentDirection == broker.BuyDirectionLong {
			return 0.7
		}
		return 0.4
	case shortConfirmed:
		if ha.currentDirection != nil && *ha.currentDirection == broker.BuyDirectionShort {
			return -0.7
		}
		return -0.4
	default:
		return 0
	}
}
