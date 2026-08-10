package volatility

import (
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

type Volatility struct {
	cb *circularbuffer.CircularBuffer
}

func New(minSize, maxSize int) *Volatility {
	return &Volatility{
		cb: circularbuffer.New(minSize, maxSize),
	}
}

func (v *Volatility) AddOHLC(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.cb.Insert(o.VolatilityInPercentage())
}

func (v *Volatility) MedianVolatilityInPercentage() (float64, error) {
	return v.cb.Median()
}

func (v *Volatility) VolatilityInPercentageQuantile(quantile float64) (float64, error) {
	return v.cb.Quantile(quantile)
}
