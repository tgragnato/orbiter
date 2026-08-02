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
		TrainSize:  150,
		TestSize:   50,
		Embargo:    5,
		NTrees:     3,
		MaxDepth:   2,
		MinSamples: 5,
	}

	engine := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine.Start(ctx, samples, cfg)

	select {
	case result := <-engine.Results:
		if result.BestForest == nil {
			t.Error("expected non-nil BestForest")
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
	cfg := WalkForwardConfig{TrainSize: 150, TestSize: 50, Embargo: 5, NTrees: 3, MaxDepth: 2, MinSamples: 5}

	engine := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	engine.Start(ctx, samples, cfg)
	time.Sleep(5 * time.Millisecond)
	engine.Pause()
	if s := engine.Status(); s != StatusPaused && s != StatusRunning && s != StatusDone {
		t.Errorf("unexpected status after Pause: %d", s)
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
	cfg := WalkForwardConfig{TrainSize: 150, TestSize: 50, Embargo: 5, NTrees: 5, MaxDepth: 3, MinSamples: 5}

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
	cfg := WalkForwardConfig{TrainSize: 150, TestSize: 50, Embargo: 5, NTrees: 2, MaxDepth: 2, MinSamples: 5}
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
	f := NewForest(5, 3, 2)
	f.Fit(samples, 5)

	data, err := marshalForest(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("marshalled to empty bytes")
	}

	f2, err := unmarshalForest(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Predictions should be identical after round-trip.
	var feat [featureCount]float64
	feat[FeatRSI] = 0.5
	p1 := f.Predict(feat)
	p2 := f2.Predict(feat)
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
	fd := &forestData{
		Trees: []*treeData{
			{
				Root:       &nodeData{IsLeaf: true, Prediction: 0.5},
				MaxDepth:   3,
				MinSamples: 2,
			},
		},
		NFeatures:  4,
		MaxDepth:   3,
		MinSamples: 2,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(fd); err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	var out forestData
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		t.Fatalf("gob decode: %v", err)
	}
	if len(out.Trees) != 1 {
		t.Errorf("decoded %d trees, want 1", len(out.Trees))
	}
}
