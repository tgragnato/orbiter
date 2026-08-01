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
	"fmt"
	"sort"
)

type CircularBuffer struct {
	records       []float64
	sortedRecords []float64
	minSize       int
	maxSize       int
	index         int
	enoughData    bool
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
		sortedRecords: []float64{},
	}
}

// GetAll returns all currently stored elements.
//
// It returns an error until at least minSize elements have been inserted.
func (cb *CircularBuffer) GetAll() ([]float64, error) {
	if !cb.enoughData {
		return []float64{}, fmt.Errorf("not enough data, have %d, need %d", cb.index, cb.minSize)
	}
	return cb.records, nil
}

// Insert adds a value to the buffer.
//
// While len(records) is below maxSize the value is appended, then insertion
// rotates and overwrites the oldest value.
func (cb *CircularBuffer) Insert(value float64) {
	if cb.enoughData && len(cb.records) < cb.maxSize {
		// between minSize and maxSize
		cb.records = append(cb.records, value)
	} else {
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

	// sort records
	toBeSortedRecords := make([]float64, len(cb.records))
	copy(toBeSortedRecords, cb.records)
	sort.Float64s(toBeSortedRecords)
	cb.sortedRecords = toBeSortedRecords
}

// Min returns the smallest stored value in the buffer.
func (cb *CircularBuffer) Min() (float64, error) {
	return cb.Quantile(0)
}

// Max returns the largest stored value in the buffer.
func (cb *CircularBuffer) Max() (float64, error) {
	return cb.Quantile(1)
}

// Median returns the median of stored values in the buffer.
func (cb *CircularBuffer) Median() (float64, error) {
	return cb.Quantile(0.5)
}

// Average returns the arithmetic mean of stored values in the buffer.
func (cb *CircularBuffer) Average() (float64, error) {
	if !cb.enoughData {
		return 0, fmt.Errorf("not enough data, have %d, need %d", cb.index, cb.minSize)
	}

	var total float64
	for _, record := range cb.sortedRecords {
		total += record
	}
	return total / float64(len(cb.sortedRecords)), nil
}

// Quantile returns the value at the provided quantile in [0, 1].
//
// It returns an error if quantile is outside [0, 1] or if there is not enough
// data yet.
func (cb *CircularBuffer) Quantile(quantile float64) (float64, error) {
	if !cb.enoughData {
		return 0, fmt.Errorf("not enough data, have %d, need %d", cb.index, cb.minSize)
	}
	if quantile > 1 || quantile < 0 {
		return 0, fmt.Errorf("quantile needs to be between 0 and 1, but is %f", quantile)
	}

	i := float64(len(cb.sortedRecords)) * quantile
	if i > 0 {
		i--
	}
	return cb.sortedRecords[int(i)], nil
}
