package rsi

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestRSI_Value(t *testing.T) {
	t.Parallel()

	var prices = []float64{14.5, 18.45, 12.75, 15.35, 13.05, 16.10, 12.20, 11.65, 13.25, 15.30, 14.85, 16.15, 19.05, 21.45, 17.55}
	var rsi14 = New(14)

	for i := len(prices) - 1; i >= 0; i-- {
		rsi14.Insert(generateCandle(prices[i]))
	}

	rsiValue, err := rsi14.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rsiValue[Value] != 45.83901773533423 {
		t.Fatalf("expected %v, got %v", 45.83901773533423, rsiValue[Value])
	}
}

func TestRSI_Value_Shift(t *testing.T) {
	t.Parallel()

	var prices1 = []float64{15947.1, 15952.1, 15953.6, 15952.1, 15953.6, 15955.6, 15952.6, 15954.1, 15952.1, 15962.1, 15960.1, 15960, 15959.8, 15959, 15959.9}
	var prices2 = []float64{15948, 15947.1, 15952.1, 15953.6, 15952.1, 15953.6, 15955.6, 15952.6, 15954.1, 15952.1, 15962.1, 15960.1, 15960, 15959.8, 15959, 15959.9}

	var rsi1 = New(14)
	for _, price := range prices1 {
		rsi1.Insert(generateCandle(price))
	}
	rsiValue, err := rsi1.Value()
	t.Logf("rsi1 -> %f", rsiValue[Value])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rsiValue[Value] != 69.99999999999888 {
		t.Fatalf("expected %v, got %v", 69.99999999999888, rsiValue[Value])
	}

	var rsi2 = New(14)
	for _, price := range prices2 {
		rsi2.Insert(generateCandle(price))
	}
	rsiValue, err = rsi2.Value()
	t.Logf("rsi2 -> %f", rsiValue[Value])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rsiValue[Value] != 68.15212319178684 {
		t.Fatalf("expected %v, got %v", 68.15212319178684, rsiValue[Value])
	}
}

func randFloats(minVal, maxVal float64, n int) []float64 {
	res := make([]float64, n)
	for i := range res {
		res[i] = minVal + rand.Float64()*(maxVal-minVal)
	}
	return res
}

func TestRSI_Random(t *testing.T) {
	t.Parallel()

	var prices1 = randFloats(10, 100, 14*3)

	var rsi1 = New(14)
	for _, price := range prices1 {
		rsi1.Insert(generateCandle(price))
	}
	rsiValue, err := rsi1.Value()
	t.Logf("rsi1 -> %f", rsiValue[Value])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prices2 := randFloats(10, 100, 14*10)
	prices2 = append(prices2, prices1...)
	rsi1 = New(14)
	for _, price := range prices2 {
		rsi1.Insert(generateCandle(price))
	}
	rsi2Value, err := rsi1.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("rsi2 -> %f", rsi2Value[Value])

	diff := rsi2Value[Value] - rsiValue[Value]
	t.Logf("diff: %f", diff)
	if diff >= 1 {
		t.Fatalf("expected true")
	}
}

func TestRSI_NotEnoughCandles(t *testing.T) {
	t.Parallel()

	var rsiIndicator = New(14)
	rsiIndicator.Insert(generateCandle(1))
	rsiIndicator.Insert(generateCandle(2))
	_, err := rsiIndicator.Value()
	if (err == nil) != (indicator.ErrNotEnoughData == nil) || (err != nil && indicator.ErrNotEnoughData != nil && err.Error() != indicator.ErrNotEnoughData.Error()) {
		t.Fatalf("expected error %v, got %v", err, indicator.ErrNotEnoughData)
	}
}

func generateCandle(price float64) *ohlc.OHLC {
	var o = ohlc.New("test", time.Now(), time.Minute, false)
	o.NewPrice(price, o.Start)
	o.ForceClose()
	return o
}
