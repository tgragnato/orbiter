package circularbuffer

import (
	"testing"
)

func getFilledBuffer() *CircularBuffer {
	v := New(5, 10)
	for i := 1; i < 12; i++ {
		v.Insert(float64(i))
	}
	return v
}

func TestInsert(t *testing.T) {
	t.Parallel()

	v := getFilledBuffer()

	wantArray := []float64{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for i := range wantArray {
		want := wantArray[i]
		if want != v.records[i] {
			t.Errorf("TestInsert: wantArray differs: index=%d want=%.1f got=%.1f", i, want, v.records[i])
		}
	}
}

func TestMedian(t *testing.T) {
	t.Parallel()

	v := getFilledBuffer()

	perf, err := v.Median()
	if err != nil {
		t.Fatalf("TestMedian: unexpected error: %v", err)
	}
	if perf != 6 {
		t.Fatalf("TestMedian: want=%.1f got=%.1f", 6.0, perf)
	}
}

func TestAverage(t *testing.T) {
	t.Parallel()

	v := getFilledBuffer()

	perf, err := v.Average()
	if err != nil {
		t.Fatalf("TestAverage: unexpected error: %v", err)
	}
	if perf != 6.5 {
		t.Fatalf("TestAverage: want=%.1f got=%.1f", 6.5, perf)
	}
}

func TestQuantile(t *testing.T) {
	t.Parallel()

	v := getFilledBuffer()

	perf, err := v.Quantile(0)
	if err != nil {
		t.Fatalf("TestQuantile: unexpected error for q=0: %v", err)
	}
	if perf != v.sortedRecords[0] {
		t.Fatalf("TestQuantile: q=0 want=%.1f got=%.1f", v.sortedRecords[0], perf)
	}

	perf, err = v.Quantile(1)
	if err != nil {
		t.Fatalf("TestQuantile: unexpected error for q=1: %v", err)
	}
	if perf != v.sortedRecords[len(v.sortedRecords)-1] {
		t.Fatalf("TestQuantile: q=1 want=%.1f got=%.1f", v.sortedRecords[len(v.sortedRecords)-1], perf)
	}
}

func TestMin(t *testing.T) {
	t.Parallel()

	v := getFilledBuffer()

	perf, err := v.Min()
	if err != nil {
		t.Fatalf("TestMin: unexpected error: %v", err)
	}
	if perf != 2 {
		t.Fatalf("TestMin: want=%.1f got=%.1f", 2.0, perf)
	}
}

func TestMax(t *testing.T) {
	t.Parallel()

	v := getFilledBuffer()

	perf, err := v.Max()
	if err != nil {
		t.Fatalf("TestMax: unexpected error: %v", err)
	}
	if perf != 11 {
		t.Fatalf("TestMax: want=%.1f got=%.1f", 11.0, perf)
	}
}
