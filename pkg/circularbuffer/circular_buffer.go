// Package circularbuffer provides a float64 circular buffer with basic
// statistics.
//
// The buffer has a dynamic size between minSize and maxSize:
// - Before minSize elements are inserted, read/stat methods return an error.
// - Between minSize and maxSize, values are appended.
// - At maxSize, new values overwrite the oldest ones.
//
// Supported statistics are Average, Median, Quantile, Min, and Max.
package circularbuffer

import (
	"errors"
	"fmt"
	"sort"
)

// ErrNotEnoughData is returned when fewer than minSize elements have been inserted.
var ErrNotEnoughData = errors.New("not enough data")

// ErrInvalidQuantile is returned when the quantile argument is outside [0, 1].
var ErrInvalidQuantile = errors.New("quantile needs to be between 0 and 1")

const medianQuantile = 0.5

// CircularBuffer is a fixed-capacity ring buffer of float64 values with
// built-in statistical methods (Average, Median, Quantile, Min, Max).
type CircularBuffer struct {
	records       []float64
	sortedRecords []float64
	minSize       int
	maxSize       int
	index         int
	enoughData    bool
	isDirty       bool    // Track whether data has changed since the last sort
	sum           float64 // Keeps the total sum to calculate the average in O(1)
}

// New creates a CircularBuffer.
//
// minSize defines how many elements are required before read/stat methods are
// available.
// maxSize defines the maximum number of elements stored before the oldest
// values are overwritten.
func New(minSize, maxSize int) *CircularBuffer {
	return &CircularBuffer{
		minSize:       minSize,
		maxSize:       maxSize,
		records:       make([]float64, minSize),
		sortedRecords: make([]float64, 0, maxSize),
		index:         0,
		enoughData:    false,
		isDirty:       false,
		sum:           0,
	}
}

// Average returns the arithmetic mean of stored values in the buffer in O(1).
func (cb *CircularBuffer) Average() (float64, error) {
	if !cb.enoughData {
		return 0, fmt.Errorf("not enough data, have %d, need %d: %w", cb.index, cb.minSize, ErrNotEnoughData)
	}

	return cb.sum / float64(len(cb.records)), nil
}

// GetAll returns all currently stored elements.
//
// It returns an error until at least minSize elements have been inserted.
func (cb *CircularBuffer) GetAll() ([]float64, error) {
	if !cb.enoughData {
		return []float64{}, fmt.Errorf("not enough data, have %d, need %d: %w", cb.index, cb.minSize, ErrNotEnoughData)
	}

	return cb.records, nil
}

// Insert adds a value to the buffer in O(1) time complexity.
func (cb *CircularBuffer) Insert(value float64) {
	if cb.enoughData && len(cb.records) < cb.maxSize {
		// between minSize and maxSize: append the value
		cb.sum += value
		cb.records = append(cb.records, value)
	} else {
		// overwrite the existing element
		cb.sum += value - cb.records[cb.index]
		cb.records[cb.index] = value
	}

	if cb.index+1 == cb.minSize {
		cb.enoughData = true
	}

	if cb.index+1 == cb.maxSize {
		cb.index = 0
	} else {
		cb.index++
	}

	// Mark the sorted buffer as dirty, without sorting now
	cb.isDirty = true
}

// Max returns the largest stored value in the buffer.
func (cb *CircularBuffer) Max() (float64, error) {
	return cb.Quantile(1)
}

// Median returns the median of stored values in the buffer.
func (cb *CircularBuffer) Median() (float64, error) {
	return cb.Quantile(medianQuantile)
}

// Min returns the smallest stored value in the buffer.
func (cb *CircularBuffer) Min() (float64, error) {
	return cb.Quantile(0)
}

// Quantile returns the value at the provided quantile in [0, 1].
//
// It returns an error if quantile is outside [0, 1] or if there is not enough
// data yet.
func (cb *CircularBuffer) Quantile(quantile float64) (float64, error) {
	if !cb.enoughData {
		return 0, fmt.Errorf("not enough data, have %d, need %d: %w", cb.index, cb.minSize, ErrNotEnoughData)
	}

	if quantile > 1 || quantile < 0 {
		return 0, fmt.Errorf("quantile needs to be between 0 and 1, but is %f: %w", quantile, ErrInvalidQuantile)
	}

	// Sort only if necessary
	cb.ensureSorted()

	i := float64(len(cb.sortedRecords)) * quantile
	if i > 0 {
		i--
	}

	return cb.sortedRecords[int(i)], nil
}

// ensureSorted makes sure that sortedRecords is aligned with records.
// It is executed only on demand (Lazy) and reuses existing memory.
func (cb *CircularBuffer) ensureSorted() {
	if !cb.isDirty {
		return
	}

	// Truncate to 0 while keeping the underlying capacity (0 memory allocations)
	cb.sortedRecords = append(cb.sortedRecords[:0], cb.records...)
	sort.Float64s(cb.sortedRecords)
	cb.isDirty = false
}
