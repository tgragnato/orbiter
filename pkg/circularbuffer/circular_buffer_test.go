//nolint:testpackage // accesses unexported fields records and sortedRecords
package circularbuffer

import (
	"testing"
)

func getFilledBuffer() *CircularBuffer {
	buf := New(5, 10)

	for i := 1; i < 12; i++ {
		buf.Insert(float64(i))
	}

	return buf
}

func TestInsert(t *testing.T) {
	t.Parallel()

	buf := getFilledBuffer()

	wantArray := []float64{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	for i := range wantArray {
		want := wantArray[i]

		if want != buf.records[i] {
			t.Errorf("TestInsert: wantArray differs: index=%d want=%.1f got=%.1f", i, want, buf.records[i])
		}
	}
}

func TestMedian(t *testing.T) {
	t.Parallel()

	buf := getFilledBuffer()

	perf, err := buf.Median()
	if err != nil {
		t.Fatalf("TestMedian: unexpected error: %v", err)
	}

	if perf != 6 {
		t.Fatalf("TestMedian: want=%.1f got=%.1f", 6.0, perf)
	}
}

func TestAverage(t *testing.T) {
	t.Parallel()

	buf := getFilledBuffer()

	perf, err := buf.Average()
	if err != nil {
		t.Fatalf("TestAverage: unexpected error: %v", err)
	}

	if perf != 6.5 {
		t.Fatalf("TestAverage: want=%.1f got=%.1f", 6.5, perf)
	}
}

func TestQuantile(t *testing.T) {
	t.Parallel()

	buf := getFilledBuffer()

	perf, err := buf.Quantile(0)
	if err != nil {
		t.Fatalf("TestQuantile: unexpected error for q=0: %v", err)
	}

	if perf != buf.sortedRecords[0] {
		t.Fatalf("TestQuantile: q=0 want=%.1f got=%.1f", buf.sortedRecords[0], perf)
	}

	perf, err = buf.Quantile(1)
	if err != nil {
		t.Fatalf("TestQuantile: unexpected error for q=1: %v", err)
	}

	if perf != buf.sortedRecords[len(buf.sortedRecords)-1] {
		t.Fatalf("TestQuantile: q=1 want=%.1f got=%.1f", buf.sortedRecords[len(buf.sortedRecords)-1], perf)
	}
}

func TestMin(t *testing.T) {
	t.Parallel()

	buf := getFilledBuffer()

	perf, err := buf.Min()
	if err != nil {
		t.Fatalf("TestMin: unexpected error: %v", err)
	}

	if perf != 2 {
		t.Fatalf("TestMin: want=%.1f got=%.1f", 2.0, perf)
	}
}

func TestMax(t *testing.T) {
	t.Parallel()

	buf := getFilledBuffer()

	perf, err := buf.Max()
	if err != nil {
		t.Fatalf("TestMax: unexpected error: %v", err)
	}

	if perf != 11 {
		t.Fatalf("TestMax: want=%.1f got=%.1f", 11.0, perf)
	}
}
