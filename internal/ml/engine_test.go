//nolint:testpackage // accesses unexported symbols (makeSamples, marshalForest, unmarshalForest, etc.)
package ml

import (
	"bytes"
	"context"
	"encoding/gob"
	"testing"
	"time"
)

func TestEngineStartAndComplete(t *testing.T) {
	t.Parallel()

	samples := makeSamples(400)
	cfg := WalkForwardConfig{
		TrainSize:        150,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           3,
		FeaturesPerSplit: 0,
		MaxDepth:         2,
		MinSamples:       5,
	}

	engine := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	engine.Start(ctx, samples, cfg)

	select {
	case result := <-engine.Results:
		if result.Forest == nil {
			t.Error("expected non-nil Forest")
		}

		if len(result.AllFolds) == 0 {
			t.Error("expected at least one fold")
		}
	case <-ctx.Done():
		t.Fatal("engine timed out")
	}
}

func TestEnginePauseResume(t *testing.T) {
	t.Parallel()

	// Large sample set to give time to pause/resume.
	samples := makeSamples(500)
	cfg := WalkForwardConfig{
		TrainSize:        150,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           3,
		FeaturesPerSplit: 0,
		MaxDepth:         2,
		MinSamples:       5,
	}

	engine := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	defer cancel()

	engine.Start(ctx, samples, cfg)
	time.Sleep(5 * time.Millisecond)
	engine.Pause()

	engineStatus := engine.Status()
	if engineStatus != StatusPaused && engineStatus != StatusRunning && engineStatus != StatusDone {
		t.Errorf("unexpected status after Pause: %d", engineStatus)
	}

	engine.Resume()

	// Wait for completion.
	select {
	case <-engine.Results:
	case <-ctx.Done():
		t.Fatal("engine did not complete after resume")
	}
}

func TestEngineCancel(t *testing.T) {
	t.Parallel()

	samples := makeSamples(500)
	cfg := WalkForwardConfig{
		TrainSize:        150,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           5,
		FeaturesPerSplit: 0,
		MaxDepth:         3,
		MinSamples:       5,
	}

	engine := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	engine.Start(ctx, samples, cfg)
	time.Sleep(5 * time.Millisecond)
	engine.Cancel()

	// Engine should stop; wait until Done or timeout.
	deadline := time.After(5 * time.Second)

	for {
		select {
		case <-deadline:
			t.Fatal("engine did not stop after Cancel")
		default:
			if engine.Status() == StatusDone {
				return
			}

			time.Sleep(time.Millisecond)
		}
	}
}

func TestEngineStartIdempotent(t *testing.T) {
	t.Parallel()

	samples := makeSamples(400)
	cfg := WalkForwardConfig{
		TrainSize:        150,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           2,
		FeaturesPerSplit: 0,
		MaxDepth:         2,
		MinSamples:       5,
	}
	engine := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	engine.Start(ctx, samples, cfg)
	engine.Start(ctx, samples, cfg) // second call should be no-op
	<-engine.Results
}

func TestForestGobRoundTrip(t *testing.T) {
	t.Parallel()

	samples := makeSamples(200)
	testForest := NewForest(5, 3, 2, 0)
	testForest.Fit(samples, 5)

	data, err := marshalForest(testForest)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("marshalled to empty bytes")
	}

	restoredForest, err := unmarshalForest(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Predictions should be identical after round-trip.
	var feat [featureCount]float64

	feat[FeatRSI] = 0.5
	p1 := testForest.Predict(feat)
	p2 := restoredForest.Predict(feat)

	if p1 != p2 {
		t.Errorf("prediction mismatch after gob round-trip: %f vs %f", p1, p2)
	}
}

func TestUnmarshalCorrupted(t *testing.T) {
	t.Parallel()

	_, err := unmarshalForest([]byte("not gob data"))
	if err == nil {
		t.Error("expected error for corrupted data")
	}
}

func TestNodeDataGobRegistered(t *testing.T) {
	t.Parallel()

	// Verify that encoding/gob can handle our tree types without panic.
	testForestData := &forestData{
		Trees: []*treeData{
			{
				Root: &nodeData{
					FeatureIdx: 0,
					Threshold:  0,
					Left:       nil,
					Right:      nil,
					IsLeaf:     true,
					Prediction: 0.5,
				},
				MaxDepth:   3,
				MinSamples: 2,
			},
		},
		NFeatures:  4,
		MaxDepth:   3,
		MinSamples: 2,
	}

	var buf bytes.Buffer

	err := gob.NewEncoder(&buf).Encode(testForestData)
	if err != nil {
		t.Fatalf("gob encode: %v", err)
	}

	var out forestData

	err = gob.NewDecoder(&buf).Decode(&out)
	if err != nil {
		t.Fatalf("gob decode: %v", err)
	}

	if len(out.Trees) != 1 {
		t.Errorf("decoded %d trees, want 1", len(out.Trees))
	}
}
