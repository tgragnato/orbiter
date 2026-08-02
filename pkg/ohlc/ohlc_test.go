package ohlc

import (
	"sort"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestOHLC_PerformanceInPercentage(t *testing.T) {
	t.Parallel()

	var o = OHLC{
		Open:   decimal.NewFromFloat(100),
		Close:  decimal.NewFromFloat(90),
		closed: true,
	}

	perfFloat, _ := o.PerformanceInPercentage().Float64()
	if -10 != perfFloat {
		t.Fatalf("expected %v, got %v", -10, perfFloat)
	}

	o = OHLC{
		Open:   decimal.NewFromFloat(100),
		Close:  decimal.NewFromFloat(120),
		closed: true,
	}
	perfFloat, _ = o.PerformanceInPercentage().Float64()
	if 20 != perfFloat {
		t.Fatalf("expected %v, got %v", 20, perfFloat)
	}

	o = OHLC{
		Open:   decimal.NewFromFloat(50),
		Close:  decimal.NewFromFloat(100),
		closed: true,
	}
	perfFloat, _ = o.PerformanceInPercentage().Float64()
	if 100 != perfFloat {
		t.Fatalf("expected %v, got %v", 100, perfFloat)
	}
}

func TestOHLC_PerformanceFromOpenToHighAbsolute(t *testing.T) {
	t.Parallel()

	var o = OHLC{
		Open:   decimal.NewFromFloat(100),
		High:   decimal.NewFromFloat(150),
		closed: true,
	}

	perfFloat, _ := o.PerformanceFromOpenToHighAbsolute().Float64()
	if 50 != perfFloat {
		t.Fatalf("expected %v, got %v", 50, perfFloat)
	}
}

func TestOHLC_PerformanceFromOpenToLowAbsolute(t *testing.T) {
	t.Parallel()

	var o = OHLC{
		Open:   decimal.NewFromFloat(100),
		High:   decimal.NewFromFloat(50),
		closed: true,
	}

	perfFloat, _ := o.PerformanceFromOpenToHighAbsolute().Float64()
	if -50 != perfFloat {
		t.Fatalf("expected %v, got %v", -50, perfFloat)
	}
}

func TestOHLC_ReversionPerformanceFromHighAbsolute(t *testing.T) {
	t.Parallel()

	var o = OHLC{
		High:   decimal.NewFromFloat(100),
		Close:  decimal.NewFromFloat(50),
		closed: true,
	}

	perfFloat, _ := o.ReversionPerformanceFromHighAbsolute().Float64()
	if -50 != perfFloat {
		t.Fatalf("expected %v, got %v", -50, perfFloat)
	}
}

func TestOHLC_ForceClose(t *testing.T) {
	t.Parallel()

	var o = OHLC{}
	if o.closed {
		t.Fatalf("expected false")
	}

	o.ForceClose()
	if !o.closed {
		t.Fatalf("expected true")
	}
}

func TestOHLC_HasGaps(t *testing.T) {
	t.Parallel()

	var o = OHLC{}
	if o.HasGaps() {
		t.Fatalf("expected false")
	}

	o.Gaps = true
	if !o.HasGaps() {
		t.Fatalf("expected true")
	}
}

func TestOHLC_Closed(t *testing.T) {
	t.Parallel()

	var o = OHLC{}
	if o.HasGaps() {
		t.Fatalf("expected false")
	}

	o.closed = true
	if !o.Closed() {
		t.Fatalf("expected true")
	}
}

func TestOHLC_String(t *testing.T) {
	t.Parallel()

	var o = &OHLC{}
	if o.Validate() == nil {
		t.Fatalf("expected true")
	}

	o = New("abc", time.Now(), time.Second, false)
	o.NewPrice(decimal.NewFromFloat(1), time.Now())
	if o.String() == "" {
		t.Fatalf("expected true")
	}
}

func TestOHLC_Age(t *testing.T) {
	t.Parallel()

	var now = time.Now()
	var o = OHLC{Start: now}
	if 1 != o.Age(now.Add(time.Second)).Seconds() {
		t.Fatalf("expected %v, got %v", 1, o.Age(now.Add(time.Second)).Seconds())
	}
}

func TestOHLC_Validate(t *testing.T) {
	t.Parallel()

	var o = &OHLC{}
	if !(o.Validate() != nil) {
		t.Fatalf("expected true")
	}

	o = New("abc", time.Now(), time.Second, false)
	price := decimal.NewFromFloat(1)
	o.NewPrice(price, time.Now())
	if o.Validate() != nil {
		t.Fatalf("unexpected error: %v", o.Validate())
	}

	// open = 0
	obroken := o
	obroken.Open = decimal.Decimal{}
	if !(obroken.Validate() != nil) {
		t.Fatalf("expected true")
	}

	// low > high
	obroken = o
	obroken.Low = obroken.High.Add(price)
	if !(obroken.Validate() != nil) {
		t.Fatalf("expected true")
	}

	// close < low
	obroken = o
	obroken.Close = obroken.Low.Sub(price)
	if !(obroken.Validate() != nil) {
		t.Fatalf("expected true")
	}

	// close > high
	obroken = o
	obroken.Close = obroken.High.Add(price)
	if !(obroken.Validate() != nil) {
		t.Fatalf("expected true")
	}

	// end < start
	obroken = o
	obroken.End = obroken.Start.Add(-time.Minute)
	if !(obroken.Validate() != nil) {
		t.Fatalf("expected true")
	}

	// instrument == ""
	obroken = o
	obroken.Instrument = ""
	if !(obroken.Validate() != nil) {
		t.Fatalf("expected true")
	}
}

func TestOHLC_VolatilityInPercentage(t *testing.T) {
	t.Parallel()

	var o = New("abc", time.Now(), time.Second, false)
	o.NewPrice(decimal.NewFromFloat(1), time.Now())
	o.NewPrice(decimal.NewFromFloat(2), time.Now())

	vola := o.VolatilityInPercentage()
	volaFloat, _ := vola.Float64()
	if 100 != volaFloat {
		t.Fatalf("expected %v, got %v", 100, volaFloat)
	}
}

func TestOHLC_NewPrice(t *testing.T) {
	t.Parallel()

	var o = New("abc", time.Now(), time.Second, false)
	now := time.Now()

	// open
	price := decimal.NewFromFloat(1)
	o.NewPrice(price, now)
	assertDecimal(t, price, o.Open)
	if !(o.priceDataSeen) {
		t.Fatalf("expected true")
	}

	// high
	price = decimal.NewFromFloat(2)
	o.NewPrice(price, now)
	assertDecimal(t, price, o.High)

	// low
	price = decimal.NewFromFloat(0.5)
	closePrice := price
	o.NewPrice(price, now)
	assertDecimal(t, price, o.Low)
	assertDecimal(t, price, o.Close)

	// close
	now = o.End
	price = decimal.NewFromFloat(1.2)
	considered := o.NewPrice(price, now)
	if considered {
		t.Fatalf("expected false")
	}

	assertDecimal(t, closePrice, o.Close)
	if !o.closed {
		t.Fatalf("expected true")
	}
	if !o.Closed() {
		t.Fatalf("expected true")
	}
	if !now.Equal(o.End) {
		t.Fatalf("expected %s, got %s", now, o.End)
	}

	// after end
	now = o.End.Add(time.Second)
	price = decimal.NewFromFloat(1.3)
	considered = o.NewPrice(price, now)
	if considered {
		t.Fatalf("expected false")
	}
}

func TestOHLC_NewPrice_with_Gaps(t *testing.T) {
	t.Parallel()

	var o = New("abc", time.Now(), time.Hour, false)
	now := time.Now()

	price := decimal.NewFromFloat(1)
	o.NewPrice(price, now)
	if o.HasGaps() {
		t.Fatalf("expected false")
	}

	o.NewPrice(price, now.Add(maxGapBetweenTicksInSeconds).Add(time.Minute))
	if !o.HasGaps() {
		t.Fatalf("expected true")
	}
}

func assertDecimal(t *testing.T, want, got decimal.Decimal) {
	wantFloat, _ := want.Float64()
	gotFloat, _ := got.Float64()
	if wantFloat != gotFloat {
		t.Fatalf("expected %v, got %v", wantFloat, gotFloat)
	}
}

func Test__smoothCandleStart(t *testing.T) {
	t.Parallel()

	period := time.Minute * 15
	want := time.Date(2020, 12, 17, 21, 15, 0, 0, time.UTC)
	is := time.Date(2020, 12, 17, 21, 24, 7, 8, time.UTC)
	if !want.Equal(smoothCandleStart(is, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(is, period))
	}

	period = time.Minute * 15
	want = time.Date(2020, 12, 17, 21, 30, 0, 0, time.UTC)
	is = time.Date(2020, 12, 17, 21, 33, 7, 8, time.UTC)
	if !want.Equal(smoothCandleStart(is, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(is, period))
	}

	period = time.Minute * 45
	want = time.Date(2020, 12, 17, 21, 0, 0, 0, time.UTC)
	is = time.Date(2020, 12, 17, 21, 15, 7, 8, time.UTC)
	if !want.Equal(smoothCandleStart(is, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(is, period))
	}

	period = time.Minute * 60
	want = time.Date(2020, 12, 17, 21, 0, 0, 0, time.UTC)
	is = time.Date(2020, 12, 17, 21, 15, 7, 8, time.UTC)
	if !want.Equal(smoothCandleStart(is, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(is, period))
	}
}

func Test__hightime(t *testing.T) {
	t.Parallel()

	var one = decimal.NewFromFloat(1)
	var o = New("abc", time.Now(), time.Hour, false)
	now := time.Now()

	// Price: 1
	price := one
	o.NewPrice(price, now)

	// Price: 2 -> our high
	price = price.Add(one)
	now = now.Add(time.Minute)
	highTime := now
	o.NewPrice(price, now)

	// Price: 1
	price = price.Sub(one)
	now = now.Add(time.Minute)
	o.NewPrice(price, now)
	if !highTime.Equal(o.HighTime) {
		t.Fatalf("expected %s, got %s", highTime, o.HighTime)
	}
}

func Test__lowtime(t *testing.T) {
	t.Parallel()

	var one = decimal.NewFromFloat(1)
	var o = New("abc", time.Now(), time.Hour, false)
	now := time.Now()

	// Price: 1
	price := one
	o.NewPrice(price, now)

	// Price: 0 -> our low
	price = decimal.Zero
	now = now.Add(time.Minute)
	lowTime := now
	o.NewPrice(price, now)

	// Price: 1
	price = one
	now = now.Add(time.Minute)
	o.NewPrice(price, now)
	if !lowTime.Equal(o.LowTime) {
		t.Fatalf("expected %s, got %s", lowTime, o.LowTime)
	}
}

func Test__ToTicks(t *testing.T) {
	t.Parallel()

	var one = decimal.NewFromFloat(1)
	now := time.Now()
	var o = New("abc", now, time.Hour, false)

	// open
	price := one
	openTime := now
	o.NewPrice(price, now)

	// low
	price = decimal.Zero
	now = now.Add(time.Minute)
	lowTime := now
	o.NewPrice(price, now)

	// high
	price = decimal.NewFromFloat(5)
	now = now.Add(time.Minute)
	highTime := now
	o.NewPrice(price, now)

	price = decimal.NewFromFloat(3)
	now = now.Add(time.Minute)
	closeTime := now
	o.NewPrice(price, now)
	o.ForceClose()
	if !lowTime.Equal(o.LowTime) {
		t.Fatalf("expected %s, got %s", lowTime, o.LowTime)
	}
	if !highTime.Equal(o.HighTime) {
		t.Fatalf("expected %s, got %s", highTime, o.HighTime)
	}
	if !openTime.Equal(o.Start) {
		t.Fatalf("expected %s, got %s", openTime, o.Start)
	}
	if !closeTime.Equal(o.End) {
		t.Fatalf("expected %s, got %s", closeTime, o.End)
	}

	ticks := o.ToTicks()
	if 4 != len(ticks) {
		t.Fatalf("expected %d, got %d", 4, len(ticks))
	}
	if !ticks[0].Datetime.Equal(openTime) {
		t.Fatalf("expected %s, got %s", ticks[0].Datetime, openTime)
	}
	if !ticks[1].Datetime.Equal(lowTime) {
		t.Fatalf("expected %s, got %s", ticks[1].Datetime, lowTime)
	}
	if !ticks[2].Datetime.Equal(highTime) {
		t.Fatalf("expected %s, got %s", ticks[2].Datetime, highTime)
	}
	if !ticks[3].Datetime.Equal(closeTime) {
		t.Fatalf("expected %s, got %s", ticks[3].Datetime, closeTime)
	}
}

func TestOHLC__Sort(t *testing.T) {
	t.Parallel()

	var now = time.Now()
	var o1 = generateOHLC(now, 1)
	var o2 = generateOHLC(now.Add(time.Minute), 2)
	var ohlcList = []OHLC{*o2, *o1}
	sort.Sort(OHLCList(ohlcList))
	if !ohlcList[0].End.Equal(o1.End) {
		t.Fatalf("expected %s, got %s", ohlcList[0].End, o1.End)
	}
	if !ohlcList[1].End.Equal(o2.End) {
		t.Fatalf("expected %s, got %s", ohlcList[1].End, o2.End)
	}
}

func generateOHLC(when time.Time, price float64) *OHLC {
	var o = New("abc", when, time.Hour, false)
	priceDec := decimal.NewFromFloat(price)
	o.NewPrice(priceDec, when)
	o.ForceClose()
	return o
}
