package stoch_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator/stoch"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestStoch_Value(t *testing.T) {
	t.Parallel()

	var stoch20 = stoch.New(14, 3)

	total := 0
	prices := 0

	now := time.Now()

	for i := 1; i < 100; i++ {
		bar := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++

		bar.NewPrice(float64(i), bar.Start)
		bar.ForceClose()
		stoch20.Insert(bar)
	}

	stoch20Value, err := stoch20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stoch20Value[stoch.ValueK] != 100 {
		t.Fatalf("expected %v, got %v", 100, stoch20Value[stoch.ValueK])
	}

	if stoch20Value[stoch.ValueD] != 100 {
		t.Fatalf("expected %v, got %v", 100, stoch20Value[stoch.ValueD])
	}
}

func TestStoch_Value_Down(t *testing.T) {
	t.Parallel()

	var stoch20 = stoch.New(14, 3)

	total := 0
	prices := 0

	now := time.Now()

	for i := 100; i > 0; i-- {
		bar := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++

		bar.NewPrice(float64(i), bar.Start)
		bar.ForceClose()
		stoch20.Insert(bar)
	}

	stoch20Value, err := stoch20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stoch20Value[stoch.ValueK] != 0 {
		t.Fatalf("expected %v, got %v", 0, stoch20Value[stoch.ValueK])
	}
}
