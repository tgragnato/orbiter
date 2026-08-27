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

package sma_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestSMA_Value(t *testing.T) {
	t.Parallel()

	var sma20 = sma.New(21)

	total := 0
	prices := 0

	now := time.Now()

	for idx := 1; idx < 22; idx++ {
		bar := ohlc.New("test", now, time.Minute, false)
		total += idx
		prices++

		bar.NewPrice(float64(idx), bar.Start)
		bar.ForceClose()
		sma20.Insert(bar)

		if idx < 20 {
			_, err := sma20.Value()

			if err == nil || !strings.Contains(err.Error(), "not enough") {
				t.Fatalf("expected error containing not enough, got %v", err)
			}
		}
	}

	sma20Value, err := sma20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if float64(total/prices) !=
		sma20Value[sma.Value] {
		t.Fatalf("expected %v, got %v", float64(total/prices), sma20Value[sma.Value])
	}
}
