package stochrsi

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestStochRSI_Value(t *testing.T) {
	t.Parallel()

	var rsi20 = New(5, 2, 14)

	total := 0
	prices := 0
	now := time.Now()
	for i := 1; i < 100; i++ {
		o := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++
		o.NewPrice(decimal.NewFromFloat(float64(i)), o.Start)
		o.ForceClose()
		rsi20.Insert(o)
	}

	rsi20Value, err := rsi20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 100 != rsi20Value[ValueK] {
		t.Fatalf("expected %v, got %v", 100, rsi20Value[ValueK])
	}
	if 100 != rsi20Value[ValueD] {
		t.Fatalf("expected %v, got %v", 100, rsi20Value[ValueD])
	}
}

func TestStochRSI_Value_Down(t *testing.T) {
	t.Parallel()

	var rsi20 = New(5, 2, 14)

	total := 0
	prices := 0
	now := time.Now()
	for i := 100; i > 0; i-- {
		o := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++
		o.NewPrice(decimal.NewFromFloat(float64(i)), o.Start)
		o.ForceClose()
		rsi20.Insert(o)
	}

	rsi20Value, err := rsi20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 0 != rsi20Value[ValueK] {
		t.Fatalf("expected %v, got %v", 0, rsi20Value[ValueK])
	}
}
