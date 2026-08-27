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

package helper_test

import (
	"testing"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/helper"
)

func TestCalcStopLossPriceByPercentage(t *testing.T) {
	t.Parallel()

	price := 100.0
	stopLossPercentage := 20.0

	// Long
	stopPrice := helper.CalcStopLossPriceByPercentage(price, stopLossPercentage, broker.BuyDirectionLong)
	if stopPrice != 80 {
		t.Fatalf("expected %v, got %v", 80, stopPrice)
	}

	// Short
	stopPrice = helper.CalcStopLossPriceByPercentage(price, stopLossPercentage, broker.BuyDirectionShort)
	if stopPrice != 120 {
		t.Fatalf("expected %v, got %v", 120, stopPrice)
	}
}

func TestTargetPriceByPercentage(t *testing.T) {
	t.Parallel()

	price := 100.0
	targetPercentage := 20.0

	// Long
	targetPrice := helper.CalcTargetPriceByPercentage(price, targetPercentage, broker.BuyDirectionLong)
	if targetPrice != 120 {
		t.Fatalf("expected %v, got %v", 120, targetPrice)
	}

	// Short
	targetPrice = helper.CalcTargetPriceByPercentage(price, targetPercentage, broker.BuyDirectionShort)
	if targetPrice != 80 {
		t.Fatalf("expected %v, got %v", 80, targetPrice)
	}
}

func TestGetPercentile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		n          []float64
		percentile int
		want       float64
	}{
		{"Empty slice", []float64{}, 50, 0.0},
		{"Single element", []float64{10.0}, 50, 10.0},
		{"Min percentile (0)", []float64{10.0, 20.0, 30.0}, 0, 10.0},
		{"Max percentile (100)", []float64{10.0, 20.0, 30.0}, 100, 30.0},
		{"Median (50) odd count", []float64{10.0, 20.0, 30.0}, 50, 20.0},
		{"Median (50) even count", []float64{10.0, 20.0, 30.0, 40.0}, 50, 20.0},
		{"25th percentile", []float64{10.0, 20.0, 30.0, 40.0}, 25, 10.0},
		{"75th percentile", []float64{10.0, 20.0, 30.0, 40.0}, 75, 30.0},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := helper.GetPercentile(testCase.n, testCase.percentile)
			if got != testCase.want {
				t.Errorf("GetPercentile() = %v, want %v", got, testCase.want)
			}
		})
	}
}
