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
