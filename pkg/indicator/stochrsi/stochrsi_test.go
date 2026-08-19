package stochrsi_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator/stochrsi"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestStochRSI_Value(t *testing.T) {
	t.Parallel()

	var rsi20 = stochrsi.New(5, 2, 14)

	total := 0
	prices := 0

	now := time.Now()

	for i := 1; i < 100; i++ {
		bar := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++

		bar.NewPrice(float64(i), bar.Start)
		bar.ForceClose()
		rsi20.Insert(bar)
	}

	rsi20Value, err := rsi20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rsi20Value[stochrsi.ValueK] != 100 {
		t.Fatalf("expected %v, got %v", 100, rsi20Value[stochrsi.ValueK])
	}

	if rsi20Value[stochrsi.ValueD] != 100 {
		t.Fatalf("expected %v, got %v", 100, rsi20Value[stochrsi.ValueD])
	}
}

func TestStochRSI_Value_Down(t *testing.T) {
	t.Parallel()

	var rsi20 = stochrsi.New(5, 2, 14)

	total := 0
	prices := 0

	now := time.Now()

	for i := 100; i > 0; i-- {
		bar := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++

		bar.NewPrice(float64(i), bar.Start)
		bar.ForceClose()
		rsi20.Insert(bar)
	}

	rsi20Value, err := rsi20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rsi20Value[stochrsi.ValueK] != 0 {
		t.Fatalf("expected %v, got %v", 0, rsi20Value[stochrsi.ValueK])
	}
}
