package ml

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"time"
)

// nodeData is the exported, gob-serialisable mirror of node.
type nodeData struct {
	FeatureIdx int
	Threshold  float64
	Left       *nodeData
	Right      *nodeData
	Prediction float64
	IsLeaf     bool
}

// treeData is the exported, gob-serialisable mirror of Tree.
type treeData struct {
	Root       *nodeData
	MaxDepth   int
	MinSamples int
}

// forestData is the exported, gob-serialisable mirror of Forest.
type forestData struct {
	Trees      []*treeData
	NFeatures  int
	MaxDepth   int
	MinSamples int
}

// Checkpoint persists a trained Forest to PostgreSQL and loads it back.
type Checkpoint struct {
	db *sql.DB
}

// NewCheckpoint creates a Checkpoint backed by the given database connection.
func NewCheckpoint(db *sql.DB) *Checkpoint {
	return &Checkpoint{db: db}
}

// Save serialises the forest and upserts it to ml_model_checkpoints.
// If isActive is true all other rows are demoted first.
func (c *Checkpoint) Save(ctx context.Context, modelName string, f *Forest, m Metrics, isActive bool) error {
	data, err := marshalForest(f)
	if err != nil {
		return fmt.Errorf("marshal forest: %w", err)
	}

	metricsJSON := fmt.Sprintf(`{"fold":%d,"mse":%f,"mae":%f,"sortino":%f}`, m.Fold, m.MSE, m.MAE, m.Sortino)

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if isActive {
		if _, err := tx.ExecContext(ctx, `UPDATE ml_model_checkpoints SET is_active = false WHERE model_name = $1`, modelName); err != nil {
			return fmt.Errorf("demote active models: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ml_model_checkpoints (model_name, metrics_json, model_data, is_active, created_at)
		VALUES ($1, $2::jsonb, $3, $4, $5)
	`, modelName, metricsJSON, data, isActive, time.Now().UTC()); err != nil {
		return fmt.Errorf("insert checkpoint: %w", err)
	}

	// Prune: keep only the top 5 intermediate checkpoints per model name.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM ml_model_checkpoints
		WHERE model_name = $1
		  AND is_active = false
		  AND id NOT IN (
		      SELECT id FROM ml_model_checkpoints
		      WHERE model_name = $1 AND is_active = false
		      ORDER BY created_at DESC
		      LIMIT 5
		  )
	`, modelName); err != nil {
		return fmt.Errorf("prune checkpoints: %w", err)
	}

	return tx.Commit()
}

// LoadActive loads the currently active model for the given name.
// Returns the forest and the time it was created (for throttle initialisation).
// Returns ErrNoActiveModel when no active checkpoint exists.
func (c *Checkpoint) LoadActive(ctx context.Context, modelName string) (*Forest, time.Time, error) {
	var data []byte
	var createdAt time.Time
	err := c.db.QueryRowContext(ctx, `
		SELECT model_data, created_at FROM ml_model_checkpoints
		WHERE model_name = $1 AND is_active = true
		ORDER BY created_at DESC
		LIMIT 1
	`, modelName).Scan(&data, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNoActiveModel
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("query active checkpoint: %w", err)
	}
	f, err := unmarshalForest(data)
	return f, createdAt, err
}

// ErrNoActiveModel is returned when no active checkpoint exists for a model.
var ErrNoActiveModel = fmt.Errorf("no active ML model checkpoint found")

// marshalForest encodes a Forest to a gob byte slice.
func marshalForest(f *Forest) ([]byte, error) {
	fd := toForestData(f)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(fd); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unmarshalForest decodes a Forest from a gob byte slice.
func unmarshalForest(data []byte) (*Forest, error) {
	var fd forestData
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&fd); err != nil {
		return nil, fmt.Errorf("decode forest: %w", err)
	}
	return fromForestData(&fd), nil
}

func toForestData(f *Forest) *forestData {
	if f == nil {
		return nil
	}
	trees := make([]*treeData, len(f.Trees))
	for i, t := range f.Trees {
		trees[i] = &treeData{
			Root:       toNodeData(t.root),
			MaxDepth:   t.maxDepth,
			MinSamples: t.minSamples,
		}
	}
	return &forestData{
		Trees:      trees,
		NFeatures:  f.nFeatures,
		MaxDepth:   f.maxDepth,
		MinSamples: f.minSamples,
	}
}

func toNodeData(n *node) *nodeData {
	if n == nil {
		return nil
	}
	return &nodeData{
		FeatureIdx: n.featureIdx,
		Threshold:  n.threshold,
		Left:       toNodeData(n.left),
		Right:      toNodeData(n.right),
		Prediction: n.prediction,
		IsLeaf:     n.isLeaf,
	}
}

func fromForestData(fd *forestData) *Forest {
	if fd == nil {
		return nil
	}
	trees := make([]*Tree, len(fd.Trees))
	for i, td := range fd.Trees {
		trees[i] = &Tree{
			root:       fromNodeData(td.Root),
			maxDepth:   td.MaxDepth,
			minSamples: td.MinSamples,
		}
	}
	return &Forest{
		Trees:      trees,
		nFeatures:  fd.NFeatures,
		maxDepth:   fd.MaxDepth,
		minSamples: fd.MinSamples,
	}
}

func fromNodeData(nd *nodeData) *node {
	if nd == nil {
		return nil
	}
	return &node{
		featureIdx: nd.FeatureIdx,
		threshold:  nd.Threshold,
		left:       fromNodeData(nd.Left),
		right:      fromNodeData(nd.Right),
		prediction: nd.Prediction,
		isLeaf:     nd.IsLeaf,
	}
}
