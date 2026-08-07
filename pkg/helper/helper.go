package helper

import (
	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/broker"
)

func FloatToDecimal(n float64) decimal.Decimal {
	return decimal.NewFromFloat(n)
}

func DecimalToFloat(n decimal.Decimal) float64 {
	f, _ := n.Float64()
	return f
}

func CalcStopLossPriceByPercentage(price, percentage decimal.Decimal, orderDirection broker.BuyDirection) decimal.Decimal {
	percentFrom := price.Div(decimal.NewFromFloat(100)).Mul(percentage)

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price.Sub(percentFrom).Round(6)
	case broker.BuyDirectionShort:
		return price.Add(percentFrom).Round(6)
	default:
		return decimal.Zero
	}
}

func CalcTargetPriceByPercentage(price, percentage decimal.Decimal, orderDirection broker.BuyDirection) decimal.Decimal {
	percentFrom := price.Div(decimal.NewFromFloat(100)).Mul(percentage)

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price.Add(percentFrom).Round(6)
	case broker.BuyDirectionShort:
		return price.Sub(percentFrom).Round(6)
	default:
		return decimal.Zero
	}
}

func GetPercentile(n []float64, percentile int) float64 {
	var pos = int(float64(len(n)) / float64(100) * float64(percentile))
	if pos < 1 {
		pos = 1
	}
	return n[pos-1]
}
