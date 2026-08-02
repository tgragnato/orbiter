package performance

import (
	"fmt"

	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/pkg/ohlc"
)

func TestAddOHLC(t *testing.T) {
	t.Parallel()

	v := New(5, 10)
	now := time.Now()
	for i := 1; i < 12; i++ {
		o := ohlc.New("test", now, time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(1.0), o.Start)
		o.NewPrice(decimal.NewFromFloat(float64(i)+1), o.Start)
		fmt.Printf("ADD: %d -> %s\n", i, o.PerformanceInPercentage())
		o.ForceClose()
		v.AddOHLC(o)
	}

	wantArray := []float64{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	isArray, err := v.cb.GetAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := range wantArray {
		want := wantArray[i] * 100
		if want != isArray[i] {
			t.Errorf("TestAddOHLC: wantArray differs: index=%d want=%.1f got=%.1f", i, want, isArray[i])
		}
	}
}

func TestMedianPerformanceInPercentage(t *testing.T) {
	t.Parallel()

	v := New(10, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		o := ohlc.New("EURUSD", now, time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(1.0), o.Start)
		o.NewPrice(decimal.NewFromFloat(float64(i)+1.0), o.Start)
		o.ForceClose()
		v.AddOHLC(o)
	}

	perf, err := v.MedianPerformanceInPercentage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 400 != perf {
		t.Fatalf("expected %v, got %v", 400, perf)
	}
}

func TestPerformanceInPercentageQuantile(t *testing.T) {
	t.Parallel()

	v := New(1000, 1000)
	now := time.Now()
	for i := 0; i < 1000; i++ {
		o := ohlc.New("EURUSD", now, time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(1.0), o.Start)
		o.NewPrice(decimal.NewFromFloat(float64(i)+1.0), o.End)
		if !o.Closed() {
			t.Fatalf("expected true")
		}
		v.AddOHLC(o)
	}

	isArray, err := v.cb.GetAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perf, err := v.PerformanceInPercentageQuantile(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isArray[0] != perf {
		t.Fatalf("expected %v, got %v", isArray[0], perf)
	}

	perf, err = v.PerformanceInPercentageQuantile(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isArray[len(isArray)-1] != perf {
		t.Fatalf("expected %v, got %v", isArray[len(isArray)-1], perf)
	}
}
