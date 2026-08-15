package helper

import (
	"math"

	"github.com/tgragnato/orbiter/pkg/broker"
)

func CalcStopLossPriceByPercentage(price, percentage float64, orderDirection broker.BuyDirection) float64 {
	percentFrom := price / 100 * percentage

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price - percentFrom
	case broker.BuyDirectionShort:
		return price + percentFrom
	default:
		return 0
	}
}

func CalcTargetPriceByPercentage(price, percentage float64, orderDirection broker.BuyDirection) float64 {
	percentFrom := price / 100 * percentage

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price + percentFrom
	case broker.BuyDirectionShort:
		return price - percentFrom
	default:
		return 0
	}
}

func GetPercentile(n []float64, percentile int) float64 {
	if len(n) == 0 {
		return 0.0
	}
	var pos = int(math.Round(float64(len(n)) / 100.0 * float64(percentile)))
	if pos < 1 || percentile == 0 {
		pos = 1
	}
	return n[pos-1]
}
