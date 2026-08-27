// Copyright (c) 2019 Simon Klinkert
// Copyright (c) 2026 Tommaso Gragnato
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

//nolint:testpackage // accesses unexported field cb for white-box testing
package volatility

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestAddOHLC(t *testing.T) {
	t.Parallel()

	vol := New(5, 10)

	now := time.Now()

	for i := 1; i < 12; i++ {
		bar := ohlc.New("test", now, time.Minute, false)
		bar.NewPrice(1.0, bar.Start)
		bar.NewPrice(float64(i)+1.0, bar.Start)
		t.Logf("ADD: %d -> %g", i, bar.VolatilityInPercentage())
		bar.ForceClose()
		vol.AddOHLC(bar)
	}

	wantArray := []float64{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}

	isArray, err := vol.cb.GetAll()
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

func TestMedianVolatilityInPercentage(t *testing.T) {
	t.Parallel()

	vol := New(10, 10)

	now := time.Now()

	for i := range 10 {
		bar := ohlc.New("EURUSD", now, time.Minute, false)
		bar.NewPrice(1.0, bar.Start)
		bar.NewPrice(float64(i)+1.0, bar.Start)
		bar.ForceClose()
		vol.AddOHLC(bar)
	}

	perf, err := vol.MedianVolatilityInPercentage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if perf != 400 {
		t.Fatalf("expected %v, got %v", 400, perf)
	}
}

func TestVolatilityInPercentageQuantile(t *testing.T) {
	t.Parallel()

	vol := New(1000, 1000)

	now := time.Now()

	for i := range 1000 {
		bar := ohlc.New("EURUSD", now, time.Minute, false)
		bar.NewPrice(1.0, bar.Start)
		bar.NewPrice(float64(i)+1.0, bar.End)

		if !bar.Closed() {
			t.Fatalf("expected true")
		}

		vol.AddOHLC(bar)
	}

	isArray, err := vol.cb.GetAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perf, err := vol.VolatilityInPercentageQuantile(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if isArray[0] != perf {
		t.Fatalf("expected %v, got %v", isArray[0], perf)
	}

	perf, err = vol.VolatilityInPercentageQuantile(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if isArray[len(isArray)-1] != perf {
		t.Fatalf("expected %v, got %v", isArray[len(isArray)-1], perf)
	}
}
