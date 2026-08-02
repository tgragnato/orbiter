package amcharts

import (
	"bytes"

	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/internal/broker"
	"github.com/sklinkert/at/pkg/chart"
	"github.com/sklinkert/at/pkg/ohlc"
)

func getCandlesLong(amount int) (candles []*ohlc.OHLC) {
	now := time.Now()
	for i := 0; i < amount; i++ {
		o := ohlc.New("test", now, time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(float64(1)), o.Start)
		o.NewPrice(decimal.NewFromFloat(float64(2)), o.Start)
		o.ForceClose()
		candles = append(candles, o)
		now = now.Add(time.Minute)
	}
	return candles
}

func TestChart_RenderChart(t *testing.T) {
	t.Parallel()

	var c chart.Chart
	var candles = getCandlesLong(5)

	c = NewChart("EURUSD")
	for _, candle := range candles {
		c.OnCandle(*candle)
	}

	position := broker.Position{
		PerformanceRecordID: 0,
		Size:                1,
		CandleBuyTime:       candles[0].Start,
		CandleSellTime:      candles[1].Start,
	}
	c.OnPosition(position)

	buf := new(bytes.Buffer)
	err := c.RenderChart(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html, err := c.RenderChartToHTML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if html == "" {
		t.Fatalf("expected non-empty HTML")
	}
}
