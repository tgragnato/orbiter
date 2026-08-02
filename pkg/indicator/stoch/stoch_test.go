package stoch

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestStoch_Value(t *testing.T) {
	t.Parallel()

	var stoch20 = New(14, 3)

	total := 0
	prices := 0
	now := time.Now()
	for i := 1; i < 100; i++ {
		o := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++
		o.NewPrice(decimal.NewFromFloat(float64(i)), o.Start)
		o.ForceClose()
		stoch20.Insert(o)
	}

	stoch20Value, err := stoch20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 100 != stoch20Value[ValueK] {
		t.Fatalf("expected %v, got %v", 100, stoch20Value[ValueK])
	}
	if 100 != stoch20Value[ValueD] {
		t.Fatalf("expected %v, got %v", 100, stoch20Value[ValueD])
	}
}

func TestStoch_Value_Down(t *testing.T) {
	t.Parallel()

	var stoch20 = New(14, 3)

	total := 0
	prices := 0
	now := time.Now()
	for i := 100; i > 0; i-- {
		o := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++
		o.NewPrice(decimal.NewFromFloat(float64(i)), o.Start)
		o.ForceClose()
		stoch20.Insert(o)
	}

	stoch20Value, err := stoch20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 0 != stoch20Value[ValueK] {
		t.Fatalf("expected %v, got %v", 0, stoch20Value[ValueK])
	}
}
