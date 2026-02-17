package artifact

import (
	"context"
	"errors"
)

// Store defines operations for persisting run artifacts.
type Store interface {
	Put(ctx context.Context, runID, path string, content []byte) error
	Get(ctx context.Context, runID, path string) ([]byte, error)
	GetURL(ctx context.Context, runID, path string) (string, error)
	List(ctx context.Context, runID string) ([]string, error)
}

var ErrNotFound = errors.New("artifact not found")
