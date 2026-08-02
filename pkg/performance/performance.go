package performance

import (
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

type Performance struct {
	cb *circularbuffer.CircularBuffer
}

func New(minSize, maxSize int) *Performance {
	return &Performance{
		cb: circularbuffer.New(minSize, maxSize),
	}
}

func (v *Performance) AddOHLC(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	volaFloat, _ := o.PerformanceInPercentage().Float64()
	v.cb.Insert(volaFloat)
}

func (v *Performance) MedianPerformanceInPercentage() (float64, error) {
	return v.cb.Median()
}

func (v *Performance) PerformanceInPercentageQuantile(quantile float64) (float64, error) {
	return v.cb.Quantile(quantile)
}
