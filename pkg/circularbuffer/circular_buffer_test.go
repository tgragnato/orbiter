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

type getAllCase struct {
	name    string
	minSize int
	maxSize int
	inserts []float64
	want    []float64
	wantErr bool
}

// getAllCases covers every GetAll branch: error path, no-wrap (index==0),
// mid-fill growth, and wrap with reordering — for both minSize<maxSize and
// the minSize==maxSize layout used by StochRSI.
func getAllCases() []getAllCase {
	return []getAllCase{
		{
			name:    "not enough data returns error",
			minSize: 5, maxSize: 10,
			inserts: []float64{1, 2, 3},
			wantErr: true,
		},
		{
			name:    "exactly minSize, index not yet 0 again",
			minSize: 5, maxSize: 10,
			inserts: []float64{1, 2, 3, 4, 5},
			want:    []float64{1, 2, 3, 4, 5},
		},
		{
			name:    "growing phase between minSize and maxSize",
			minSize: 5, maxSize: 10,
			inserts: []float64{1, 2, 3, 4, 5, 6, 7},
			want:    []float64{1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:    "exactly maxSize, index wraps to 0",
			minSize: 5, maxSize: 10,
			inserts: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			want:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			name:    "one overwrite past maxSize, ring reordered",
			minSize: 5, maxSize: 10,
			inserts: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			want:    []float64{2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
		{
			name:    "second full wrap, index back to 0",
			minSize: 5, maxSize: 10,
			inserts: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
			want:    []float64{11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		},
		{
			// StochRSI uses minSize == maxSize; the ring wraps after every
			// maxSize inserts and must still return chronological order.
			name:    "minSize equals maxSize, first fill",
			minSize: 5, maxSize: 5,
			inserts: []float64{10, 20, 30, 40, 50},
			want:    []float64{10, 20, 30, 40, 50},
		},
		{
			name:    "minSize equals maxSize, one overwrite",
			minSize: 5, maxSize: 5,
			inserts: []float64{10, 20, 30, 40, 50, 60},
			want:    []float64{20, 30, 40, 50, 60},
		},
		{
			name:    "minSize equals maxSize, second full wrap",
			minSize: 5, maxSize: 5,
			inserts: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			want:    []float64{60, 70, 80, 90, 100},
		},
	}
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	for _, tc := range getAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			buf := New(tc.minSize, tc.maxSize)
			for _, v := range tc.inserts {
				buf.Insert(v)
			}

			got, err := buf.GetAll()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("length mismatch: want %d, got %d", len(tc.want), len(got))
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("index %d: want %.1f, got %.1f", i, w, got[i])
				}
			}
		})
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
