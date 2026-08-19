//nolint:testpackage // accesses unexported fields closed and priceDataSeen
package ohlc

import (
	"sort"
	"testing"
	"time"
)

//nolint:funlen // multiple sub-cases
func TestOHLC_PerformanceInPercentage(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              100,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             90,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            true,
		lastReceivedPrice: time.Time{},
	}

	perfFloat := ohlc.PerformanceInPercentage()
	if perfFloat != -10 {
		t.Fatalf("expected %v, got %v", -10, perfFloat)
	}

	ohlc = OHLC{
		Instrument:        "",
		Open:              100,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             120,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            true,
		lastReceivedPrice: time.Time{},
	}

	perfFloat = ohlc.PerformanceInPercentage()
	if perfFloat != 20 {
		t.Fatalf("expected %v, got %v", 20, perfFloat)
	}

	ohlc = OHLC{
		Instrument:        "",
		Open:              50,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             100,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            true,
		lastReceivedPrice: time.Time{},
	}

	perfFloat = ohlc.PerformanceInPercentage()
	if perfFloat != 100 {
		t.Fatalf("expected %v, got %v", 100, perfFloat)
	}
}

func TestOHLC_PerformanceFromOpenToHighAbsolute(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              100,
		High:              150,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            true,
		lastReceivedPrice: time.Time{},
	}

	perfFloat := ohlc.PerformanceFromOpenToHighAbsolute()
	if perfFloat != 50 {
		t.Fatalf("expected %v, got %v", 50, perfFloat)
	}
}

func TestOHLC_PerformanceFromOpenToLowAbsolute(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              100,
		High:              50,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            true,
		lastReceivedPrice: time.Time{},
	}

	perfFloat := ohlc.PerformanceFromOpenToHighAbsolute()
	if perfFloat != -50 {
		t.Fatalf("expected %v, got %v", -50, perfFloat)
	}
}

func TestOHLC_ReversionPerformanceFromHighAbsolute(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              0,
		High:              100,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             50,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            true,
		lastReceivedPrice: time.Time{},
	}

	perfFloat := ohlc.ReversionPerformanceFromHighAbsolute()
	if perfFloat != -50 {
		t.Fatalf("expected %v, got %v", -50, perfFloat)
	}
}

func TestOHLC_ForceClose(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}
	if ohlc.closed {
		t.Fatalf("expected false")
	}

	ohlc.ForceClose()

	if !ohlc.closed {
		t.Fatalf("expected true")
	}
}

func TestOHLC_HasGaps(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}
	if ohlc.HasGaps() {
		t.Fatalf("expected false")
	}

	ohlc.Gaps = true
	if !ohlc.HasGaps() {
		t.Fatalf("expected true")
	}
}

func TestOHLC_Closed(t *testing.T) {
	t.Parallel()

	ohlc := OHLC{
		Instrument:        "",
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}
	if ohlc.HasGaps() {
		t.Fatalf("expected false")
	}

	ohlc.closed = true
	if !ohlc.Closed() {
		t.Fatalf("expected true")
	}
}

func TestOHLC_String(t *testing.T) {
	t.Parallel()

	ohlcPtr := &OHLC{
		Instrument:        "",
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}
	if ohlcPtr.Validate() == nil {
		t.Fatalf("expected true")
	}

	ohlcPtr = New("abc", time.Now(), time.Second, false)
	ohlcPtr.NewPrice(1, time.Now())

	if ohlcPtr.String() == "" {
		t.Fatalf("expected true")
	}
}

func TestOHLC_Age(t *testing.T) {
	t.Parallel()

	now := time.Now()

	ohlc := OHLC{
		Instrument:        "",
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             now,
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}

	if ohlc.Age(now.Add(time.Second)).Seconds() != 1 {
		t.Fatalf("expected %v, got %v", 1, ohlc.Age(now.Add(time.Second)).Seconds())
	}
}

//nolint:funlen // multiple sub-cases
func TestOHLC_Validate(t *testing.T) {
	t.Parallel()

	ohlcPtr := &OHLC{
		Instrument:        "",
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             time.Time{},
		End:               time.Time{},
		Duration:          0,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}
	if ohlcPtr.Validate() == nil {
		t.Fatalf("expected true")
	}

	ohlcPtr = New("abc", time.Now(), time.Second, false)
	ohlcPtr.NewPrice(1, time.Now())

	if ohlcPtr.Validate() != nil {
		t.Fatalf("unexpected error: %v", ohlcPtr.Validate())
	}

	// open = 0
	obroken := ohlcPtr
	obroken.Open = 0

	if obroken.Validate() == nil {
		t.Fatalf("expected true")
	}

	// low > high
	obroken = ohlcPtr
	obroken.Low = obroken.High + 1

	if obroken.Validate() == nil {
		t.Fatalf("expected true")
	}

	// close < low
	obroken = ohlcPtr
	obroken.Close = obroken.Low - 1

	if obroken.Validate() == nil {
		t.Fatalf("expected true")
	}

	// close > high
	obroken = ohlcPtr
	obroken.Close = obroken.High + 1

	if obroken.Validate() == nil {
		t.Fatalf("expected true")
	}

	// end < start
	obroken = ohlcPtr
	obroken.End = obroken.Start.Add(-time.Minute)

	if obroken.Validate() == nil {
		t.Fatalf("expected true")
	}

	// instrument == ""
	obroken = ohlcPtr
	obroken.Instrument = ""

	if obroken.Validate() == nil {
		t.Fatalf("expected true")
	}
}

func TestOHLC_VolatilityInPercentage(t *testing.T) {
	t.Parallel()

	ohlc := New("abc", time.Now(), time.Second, false)
	ohlc.NewPrice(1, time.Now())
	ohlc.NewPrice(2, time.Now())

	vola := ohlc.VolatilityInPercentage()
	if vola != 100 {
		t.Fatalf("expected %v, got %v", 100, vola)
	}
}

func TestOHLC_NewPrice(t *testing.T) {
	t.Parallel()

	ohlc := New("abc", time.Now(), time.Second, false)
	now := time.Now()

	// open
	price := 1.0
	ohlc.NewPrice(price, now)
	assertDecimal(t, price, ohlc.Open)

	if !(ohlc.priceDataSeen) {
		t.Fatalf("expected true")
	}

	// high
	price = 2
	ohlc.NewPrice(price, now)
	assertDecimal(t, price, ohlc.High)

	// low
	price = 0.5
	closePrice := price
	ohlc.NewPrice(price, now)
	assertDecimal(t, price, ohlc.Low)
	assertDecimal(t, price, ohlc.Close)

	// close
	now = ohlc.End
	price = 1.2

	considered := ohlc.NewPrice(price, now)
	if considered {
		t.Fatalf("expected false")
	}

	assertDecimal(t, closePrice, ohlc.Close)

	if !ohlc.closed {
		t.Fatalf("expected true")
	}

	if !ohlc.Closed() {
		t.Fatalf("expected true")
	}

	if !now.Equal(ohlc.End) {
		t.Fatalf("expected %s, got %s", now, ohlc.End)
	}

	// after end
	now = ohlc.End.Add(time.Second)
	price = 1.3

	considered = ohlc.NewPrice(price, now)
	if considered {
		t.Fatalf("expected false")
	}
}

func TestOHLC_NewPrice_with_Gaps(t *testing.T) {
	t.Parallel()

	ohlc := New("abc", time.Now(), time.Hour, false)
	now := time.Now()

	ohlc.NewPrice(1, now)

	if ohlc.HasGaps() {
		t.Fatalf("expected false")
	}

	ohlc.NewPrice(1, now.Add(maxGapBetweenTicksInSeconds).Add(time.Minute))

	if !ohlc.HasGaps() {
		t.Fatalf("expected true")
	}
}

func assertDecimal(t *testing.T, want, got float64) {
	t.Helper()

	if want != got {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func Test__smoothCandleStart(t *testing.T) {
	t.Parallel()

	period := time.Minute * 15
	want := time.Date(2020, 12, 17, 21, 15, 0, 0, time.UTC)
	inputTime := time.Date(2020, 12, 17, 21, 24, 7, 8, time.UTC)

	if !want.Equal(smoothCandleStart(inputTime, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(inputTime, period))
	}

	period = time.Minute * 15
	want = time.Date(2020, 12, 17, 21, 30, 0, 0, time.UTC)
	inputTime = time.Date(2020, 12, 17, 21, 33, 7, 8, time.UTC)

	if !want.Equal(smoothCandleStart(inputTime, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(inputTime, period))
	}

	period = time.Minute * 45
	want = time.Date(2020, 12, 17, 21, 0, 0, 0, time.UTC)
	inputTime = time.Date(2020, 12, 17, 21, 15, 7, 8, time.UTC)

	if !want.Equal(smoothCandleStart(inputTime, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(inputTime, period))
	}

	period = time.Minute * 60
	want = time.Date(2020, 12, 17, 21, 0, 0, 0, time.UTC)
	inputTime = time.Date(2020, 12, 17, 21, 15, 7, 8, time.UTC)

	if !want.Equal(smoothCandleStart(inputTime, period)) {
		t.Fatalf("expected %s, got %s", want, smoothCandleStart(inputTime, period))
	}
}

func Test__hightime(t *testing.T) {
	t.Parallel()

	ohlc := New("abc", time.Now(), time.Hour, false)
	now := time.Now()

	// Price: 1
	ohlc.NewPrice(1, now)

	// Price: 2 -> our high
	now = now.Add(time.Minute)
	highTime := now
	ohlc.NewPrice(2, now)

	// Price: 1
	ohlc.NewPrice(1, now)

	if !highTime.Equal(ohlc.HighTime) {
		t.Fatalf("expected %s, got %s", highTime, ohlc.HighTime)
	}
}

func Test__lowtime(t *testing.T) {
	t.Parallel()

	ohlc := New("abc", time.Now(), time.Hour, false)
	now := time.Now()

	// Price: 1
	ohlc.NewPrice(1, now)

	// Price: 0 -> our low
	now = now.Add(time.Minute)
	lowTime := now
	ohlc.NewPrice(0, now)

	// Price: 1
	now = now.Add(time.Minute)
	ohlc.NewPrice(1, now)

	if !lowTime.Equal(ohlc.LowTime) {
		t.Fatalf("expected %s, got %s", lowTime, ohlc.LowTime)
	}
}

func Test__ToTicks(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ohlc := New("abc", now, time.Hour, false)

	// open
	openTime := now
	ohlc.NewPrice(1, now)

	// low
	now = now.Add(time.Minute)
	lowTime := now
	ohlc.NewPrice(0, now)

	// high
	now = now.Add(time.Minute)
	highTime := now
	ohlc.NewPrice(5, now)

	now = now.Add(time.Minute)
	closeTime := now
	ohlc.NewPrice(3, now)
	ohlc.ForceClose()

	if !lowTime.Equal(ohlc.LowTime) {
		t.Fatalf("expected %s, got %s", lowTime, ohlc.LowTime)
	}

	if !highTime.Equal(ohlc.HighTime) {
		t.Fatalf("expected %s, got %s", highTime, ohlc.HighTime)
	}

	if !openTime.Equal(ohlc.Start) {
		t.Fatalf("expected %s, got %s", openTime, ohlc.Start)
	}

	if !closeTime.Equal(ohlc.End) {
		t.Fatalf("expected %s, got %s", closeTime, ohlc.End)
	}

	ticks := ohlc.ToTicks()
	if len(ticks) != 4 {
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

	now := time.Now()
	ohlc1 := generateOHLC(now, 1)
	ohlc2 := generateOHLC(now.Add(time.Minute), 2)

	var ohlcList = []OHLC{*ohlc2, *ohlc1}

	sort.Slice(ohlcList, func(i, j int) bool { return ohlcList[i].End.Before(ohlcList[j].End) })

	if !ohlcList[0].End.Equal(ohlc1.End) {
		t.Fatalf("expected %s, got %s", ohlcList[0].End, ohlc1.End)
	}

	if !ohlcList[1].End.Equal(ohlc2.End) {
		t.Fatalf("expected %s, got %s", ohlcList[1].End, ohlc2.End)
	}
}

func generateOHLC(when time.Time, price float64) *OHLC {
	ohlc := New("abc", when, time.Hour, false)
	priceDec := price
	ohlc.NewPrice(priceDec, when)
	ohlc.ForceClose()

	return ohlc
}
