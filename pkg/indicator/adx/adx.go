package adx

import (
	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

const Value = "ADX_VALUE"

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

type ADX struct {
	closePrices floatRing
	highPrices  floatRing
	lowPrices   floatRing
	size        int
}

// New creates a new instance.
// size is usually 14
func New(size int) *ADX {
	return &ADX{
		highPrices:  newFloatRing(size * 2),
		lowPrices:   newFloatRing(size * 2),
		closePrices: newFloatRing(size * 2),
		size:        size,
	}
}

func (v *ADX) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	closePrice, _ := o.Close.Float64()
	v.closePrices.push(closePrice)

	high, _ := o.High.Float64()
	v.highPrices.push(high)

	low, _ := o.Low.Float64()
	v.lowPrices.push(low)
}

func (v *ADX) Value() (map[string]float64, error) {
	closePrices := v.closePrices.values()
	highPrices := v.highPrices.values()
	lowPrices := v.lowPrices.values()
	if len(closePrices) < v.size*2 || len(highPrices) < v.size*2 || len(lowPrices) < v.size*2 {
		return nil, indicator.ErrNotEnoughData
	}

	var m = map[string]float64{}
	adx := talib.Adx(highPrices, lowPrices, closePrices, v.size)
	if len(adx) > 0 {
		m[Value] = adx[len(adx)-1]
	}
	return m, nil
}

func (v *ADX) ValueResultKeys() []string {
	return []string{Value}
}
