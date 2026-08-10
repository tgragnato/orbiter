package round

import (
	"errors"
	"math"

	"github.com/tgragnato/orbiter/pkg/ohlc"
)

const (
	LowerRoundNumberWeak   = "LowerRoundNumberWeak"
	LowerRoundNumberStrong = "LowerRoundNumberStrong"
	UpperRoundNumberWeak   = "UpperRoundNumberWeak"
	UpperRoundNumberStrong = "UpperRoundNumberStrong"
)

type Number struct {
	latestCandle *ohlc.OHLC
}

func New() *Number {
	return &Number{}
}

func (rn *Number) Insert(o *ohlc.OHLC) {
	rn.latestCandle = o
}

func floor(number, multiplier float64) float64 {
	return math.Floor(number*multiplier) / multiplier
}

func ceil(number, multiplier float64) float64 {
	return math.Ceil(number*multiplier) / multiplier
}

func (rn *Number) Value() (map[string]float64, error) {
	if rn.latestCandle == nil {
		return nil, errors.New("price data is missing")
	}

	var m = map[string]float64{}
	var unit float64
	var multiplier float64

	switch {
	case rn.latestCandle.Close < 1.00:
		m[LowerRoundNumberWeak] = math.Floor(rn.latestCandle.Close*100) / 100
		m[LowerRoundNumberStrong] = math.Floor(rn.latestCandle.Close*10) / 10
		m[UpperRoundNumberWeak] = math.Ceil(rn.latestCandle.Close*100) / 100
		m[UpperRoundNumberStrong] = math.Ceil(rn.latestCandle.Close*10) / 10
		return m, nil
	case rn.latestCandle.Close < 10.00:
		unit = 1
		multiplier = 1
	case rn.latestCandle.Close < 100.00:
		unit = 10
		multiplier = 0.1
	case rn.latestCandle.Close < 1000.00:
		unit = 100
		multiplier = 0.01
	case rn.latestCandle.Close < 10000.00:
		unit = 1000
		multiplier = 0.01
	default:
		return nil, errors.New("not supported: price is too high")
	}

	m[LowerRoundNumberWeak] = math.Floor(rn.latestCandle.Close*multiplier) / multiplier
	m[LowerRoundNumberStrong] = unit
	m[UpperRoundNumberWeak] = math.Ceil(rn.latestCandle.Close*multiplier) / multiplier
	m[UpperRoundNumberStrong] = unit * 10

	return m, nil
}
