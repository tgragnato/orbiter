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
