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

// ErrNoActiveModel is returned when no active checkpoint exists for a model.
var ErrNoActiveModel = errors.New("no active ML model checkpoint found")

// NewCheckpoint creates a Checkpoint backed by the given database connection.
func NewCheckpoint(db *sql.DB) *Checkpoint {
	return &Checkpoint{db: db}
}

// Save serialises the forest and upserts it to ml_model_checkpoints.
// If isActive is true all other rows are demoted first.
//
//nolint:funlen // multiple SQL operations with error handling are inherently verbose
func (c *Checkpoint) Save(
	ctx context.Context,
	modelName string,
	forest *Forest,
	metrics Metrics,
	isActive bool,
) error {
	data, err := marshalForest(forest)
	if err != nil {
		return fmt.Errorf("marshal forest: %w", err)
	}

	metricsJSON := fmt.Sprintf(`{"fold":%d,"mse":%f,"mae":%f,"sortino":%f}`,
		metrics.Fold, metrics.MSE, metrics.MAE, metrics.Sortino)

	dbTx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() { _ = dbTx.Rollback() }()

	if isActive {
		_, err = dbTx.ExecContext(
			ctx,
			`UPDATE ml_model_checkpoints SET is_active = false WHERE model_name = $1`,
			modelName,
		)
		if err != nil {
			return fmt.Errorf("demote active models: %w", err)
		}
	}

	_, err = dbTx.ExecContext(ctx, `
		INSERT INTO ml_model_checkpoints (model_name, metrics_json, model_data, is_active, created_at)
		VALUES ($1, $2::jsonb, $3, $4, $5)
	`, modelName, metricsJSON, data, isActive, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert checkpoint: %w", err)
	}

	// Prune: keep only the top 5 intermediate checkpoints per model name.
	_, err = dbTx.ExecContext(ctx, `
		DELETE FROM ml_model_checkpoints
		WHERE model_name = $1
		  AND is_active = false
		  AND id NOT IN (
		      SELECT id FROM ml_model_checkpoints
		      WHERE model_name = $1 AND is_active = false
		      ORDER BY created_at DESC
		      LIMIT 5
		  )
	`, modelName)
	if err != nil {
		return fmt.Errorf("prune checkpoints: %w", err)
	}

	err = dbTx.Commit()
	if err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
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

	restoredForest, err := unmarshalForest(data)

	return restoredForest, createdAt, err
}

// marshalForest encodes a Forest to a gob byte slice.
func marshalForest(forest *Forest) ([]byte, error) {
	fd := toForestData(forest)

	var buf bytes.Buffer

	err := gob.NewEncoder(&buf).Encode(fd)
	if err != nil {
		return nil, fmt.Errorf("encode forest: %w", err)
	}

	return buf.Bytes(), nil
}

// unmarshalForest decodes a Forest from a gob byte slice.
func unmarshalForest(data []byte) (*Forest, error) {
	var forestDecode forestData

	err := gob.NewDecoder(bytes.NewReader(data)).Decode(&forestDecode)
	if err != nil {
		return nil, fmt.Errorf("decode forest: %w", err)
	}

	return fromForestData(&forestDecode), nil
}

func toForestData(forest *Forest) *forestData {
	if forest == nil {
		return nil
	}

	trees := make([]*treeData, 0, len(forest.Trees))
	for _, treeItem := range forest.Trees {
		trees = append(trees, &treeData{
			Root:       toNodeData(treeItem.root),
			MaxDepth:   treeItem.maxDepth,
			MinSamples: treeItem.minSamples,
		})
	}

	return &forestData{
		Trees:      trees,
		NFeatures:  forest.nFeatures,
		MaxDepth:   forest.maxDepth,
		MinSamples: forest.minSamples,
	}
}

func toNodeData(currentNode *node) *nodeData {
	if currentNode == nil {
		return nil
	}

	return &nodeData{
		FeatureIdx: currentNode.featureIdx,
		Threshold:  currentNode.threshold,
		Left:       toNodeData(currentNode.left),
		Right:      toNodeData(currentNode.right),
		Prediction: currentNode.prediction,
		IsLeaf:     currentNode.isLeaf,
	}
}

func fromForestData(fData *forestData) *Forest {
	if fData == nil {
		return nil
	}

	trees := make([]*Tree, len(fData.Trees))
	for i, td := range fData.Trees {
		trees[i] = &Tree{
			root:       fromNodeData(td.Root),
			maxDepth:   td.MaxDepth,
			minSamples: td.MinSamples,
		}
	}

	return &Forest{
		Trees:      trees,
		nFeatures:  fData.NFeatures,
		maxDepth:   fData.MaxDepth,
		minSamples: fData.MinSamples,
	}
}

func fromNodeData(nData *nodeData) *node {
	if nData == nil {
		return nil
	}

	return &node{
		featureIdx: nData.FeatureIdx,
		threshold:  nData.Threshold,
		left:       fromNodeData(nData.Left),
		right:      fromNodeData(nData.Right),
		prediction: nData.Prediction,
		isLeaf:     nData.IsLeaf,
	}
}
