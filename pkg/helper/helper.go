package helper

import (
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
	var pos = int(float64(len(n)) / float64(100) * float64(percentile))
	if pos < 1 {
		pos = 1
	}
	return n[pos-1]
}
