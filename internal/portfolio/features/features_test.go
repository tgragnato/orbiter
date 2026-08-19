package features_test

import (
	"math"
	"testing"

	"github.com/tgragnato/orbiter/internal/portfolio/features"
)

func TestMean(t *testing.T) {
	t.Parallel()

	if got := features.Mean([]float64{1, 2, 3, 4, 5}); math.Abs(got-3.0) > 1e-9 {
		t.Errorf("Mean = %f, want 3.0", got)
	}

	if got := features.Mean(nil); got != 0 {
		t.Errorf("Mean(nil) = %f, want 0", got)
	}
}

func TestStdDev(t *testing.T) {
	t.Parallel()

	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	mean := features.Mean(vals)

	std := features.StdDev(vals, mean)
	if math.Abs(std-2.0) > 0.001 {
		t.Errorf("StdDev = %f, want ~2.0", std)
	}

	if got := features.StdDev([]float64{1}, 1); got != 0 {
		t.Errorf("StdDev single element = %f, want 0", got)
	}
}
