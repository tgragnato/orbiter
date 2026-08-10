package rsi

import (
	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

const Value = "RSI_VALUE"

// floatRing is a fixed-capacity FIFO queue that preserves insertion order.
// When full, the oldest element is dropped to make room for the new one.
type floatRing struct {
	buf []float64
	cap int
}

func newFloatRing(capacity int) floatRing {
	return floatRing{buf: make([]float64, 0, capacity), cap: capacity}
}

func (r *floatRing) push(v float64) {
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, v)
	} else {
		copy(r.buf, r.buf[1:])
		r.buf[r.cap-1] = v
	}
}

func (r *floatRing) values() []float64 {
	return r.buf
}

type RSI struct {
	cb   floatRing
	size int
}

// New creates a new instance.
// size is usually 14
func New(size int) *RSI {
	// The talib code seems to be doing a simple moving average for the initial n values,
	// and then do 1/n exponential smoothing thereafter. This is the standard Wilder's RSI.
	// I believe the calculations shold start at the beginning of the data and not using a sliding window which would
	// be problematic due to the simple moving average for the 1st n values.
	// So basically, in order for talib RSI to be calculated as 'accurate' as possible, the number of price points
	// I pass into the function should greatly exceed the number of price points needed to initialize the indicator
	// until you reach an n value where further increasing number of price points has a negligible effect on
	// the RSI value.
	// https://www.reddit.com/r/algotrading/comments/kmgmtt/cant_validate_rsi_indicator_values_from_talib_vs/
	return &RSI{
		cb:   newFloatRing(size * 10),
		size: size,
	}
}

func (v *RSI) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.cb.push(o.Close)
}

func (v *RSI) Value() (map[string]float64, error) {
	var m = map[string]float64{}

	closePrices := v.cb.values()

	if len(closePrices) < v.size+1 {
		return nil, indicator.ErrNotEnoughData
	}

	rsi := talib.Rsi(closePrices, v.size)
	if len(rsi) > 0 {
		m[Value] = rsi[len(rsi)-1]
	}
	return m, nil
}

func (v *RSI) ValueResultKeys() []string {
	return []string{Value}
}
