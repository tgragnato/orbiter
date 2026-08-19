package helper

import (
	"math"

	"github.com/tgragnato/orbiter/pkg/broker"
)

const percentageDivisor = 100.0

// CalcStopLossPriceByPercentage calculates the stop loss price given a price, percentage, and order direction.
func CalcStopLossPriceByPercentage(price, percentage float64, orderDirection broker.BuyDirection) float64 {
	percentFrom := price / percentageDivisor * percentage

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price - percentFrom
	case broker.BuyDirectionShort:
		return price + percentFrom
	default:
		return 0
	}
}

// CalcTargetPriceByPercentage calculates the target price given a price, percentage, and order direction.
func CalcTargetPriceByPercentage(price, percentage float64, orderDirection broker.BuyDirection) float64 {
	percentFrom := price / percentageDivisor * percentage

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price + percentFrom
	case broker.BuyDirectionShort:
		return price - percentFrom
	default:
		return 0
	}
}

// GetPercentile returns the value at the given percentile from a sorted slice of float64 values.
func GetPercentile(values []float64, percentile int) float64 {
	if len(values) == 0 {
		return 0.0
	}

	var pos = int(math.Round(float64(len(values)) / percentageDivisor * float64(percentile)))
	if pos < 1 || percentile == 0 {
		pos = 1
	}

	return values[pos-1]
}
