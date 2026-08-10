package stoch

import (
	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

const ValueK = "Stoch_VALUE_K"
const ValueD = "Stoch_VALUE_D"

type Stoch struct {
	closePrices *circularbuffer.CircularBuffer
	highPrices  *circularbuffer.CircularBuffer
	lowPrices   *circularbuffer.CircularBuffer
	fastKPeriod int
	fastDPeriod int
}

func New(fastKPeriod, fastDPeriod int) *Stoch {
	return &Stoch{
		closePrices: circularbuffer.New(1, fastKPeriod*fastDPeriod),
		highPrices:  circularbuffer.New(1, fastKPeriod*fastDPeriod),
		lowPrices:   circularbuffer.New(1, fastKPeriod*fastDPeriod),
		fastKPeriod: fastKPeriod,
		fastDPeriod: fastDPeriod,
	}
}

func (v *Stoch) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.closePrices.Insert(o.Close)
	v.highPrices.Insert(o.High)
	v.lowPrices.Insert(o.Low)
}

func (v *Stoch) Value() (map[string]float64, error) {
	var m = map[string]float64{}

	closePrices, err := v.closePrices.GetAll()
	if err != nil {
		return nil, err
	}
	highPrices, err := v.highPrices.GetAll()
	if err != nil {
		return nil, err
	}
	lowPrices, err := v.lowPrices.GetAll()
	if err != nil {
		return nil, err
	}

	k, d := talib.StochF(highPrices, lowPrices, closePrices, v.fastKPeriod, v.fastDPeriod, talib.SMA)
	if len(k) > 0 {
		m[ValueK] = k[len(k)-1]
	}
	if len(d) > 0 {
		m[ValueD] = k[len(d)-1]
	}

	return m, err
}
