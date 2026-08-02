package sma10

import (
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Score returns a directional conviction in [-1.0, +1.0].
// Close above SMA-200 and below SMA-10 → positive (buy setup).
// Close below SMA-200 and above SMA-10 → negative (sell setup).
func (d *SMA) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) == 0 {
		return 0
	}

	smaValue, err := d.sma.Value()
	if err != nil {
		return 0
	}
	sma200 := smaValue[sma.Value]

	sma10val, err := d.sma10.Value()
	if err != nil {
		return 0
	}
	sma10 := sma10val[sma.Value]

	if sma200 == 0 || sma10 == 0 {
		return 0
	}

	closePrice := helper.DecimalToFloat(closedCandles[len(closedCandles)-1].Close)

	// Long setup: above SMA-200 trend filter, price dipped below fast SMA-10.
	if closePrice > sma200 && closePrice < sma10 {
		if sma10-sma200 == 0 {
			return 0
		}
		conviction := (sma10 - closePrice) / (sma10 - sma200)
		if conviction > 1 {
			conviction = 1
		}
		return conviction
	}

	// Short setup: below SMA-200 trend filter, price risen above fast SMA-10.
	if closePrice < sma200 && closePrice > sma10 {
		if sma200-sma10 == 0 {
			return 0
		}
		conviction := (closePrice - sma10) / (sma200 - sma10)
		if conviction > 1 {
			conviction = 1
		}
		return -conviction
	}

	return 0
}
