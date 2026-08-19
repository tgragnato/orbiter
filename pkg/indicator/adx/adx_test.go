package adx_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/indicator/adx"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestADX_Bearish_Trend_Above_35(t *testing.T) {
	t.Parallel()

	var adx1 = adx.New(14)
	for i := 100; i > 0; i-- {
		adx1.Insert(generateCandle(float64(i)))
	}

	adxValue, err := adx1.Value()
	t.Logf("adx1 -> %f", adxValue[adx.Value])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !(adxValue[adx.Value] > 35.0) {
		t.Fatalf("expected true")
	}
}

func TestADX_Bullish_Trend_Above_35(t *testing.T) {
	t.Parallel()

	var adx1 = adx.New(14)
	for i := range 100 {
		adx1.Insert(generateCandle(float64(i)))
	}

	adxValue, err := adx1.Value()
	t.Logf("adx1 -> %f", adxValue[adx.Value])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if adxValue[adx.Value] <= 35.0 {
		t.Fatalf("expected true")
	}
}

func TestADX_NotEnoughCandles(t *testing.T) {
	t.Parallel()

	var adxIndicator = adx.New(14)
	adxIndicator.Insert(generateCandle(1))
	adxIndicator.Insert(generateCandle(2))

	_, err := adxIndicator.Value()
	if !errors.Is(err, indicator.ErrNotEnoughData) {
		t.Fatalf("expected error %v, got %v", indicator.ErrNotEnoughData, err)
	}
}

func generateCandle(price float64) *ohlc.OHLC {
	var o = ohlc.New("test", time.Now(), time.Minute, false)
	o.NewPrice(price, o.Start)
	o.ForceClose()

	return o
}
