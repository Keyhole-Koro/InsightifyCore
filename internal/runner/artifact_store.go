package runner

import "context"

// ArtifactStore provides per-execution artifact file access for workers.
type ArtifactStore interface {
	Read(ctx context.Context, name string) ([]byte, error)
	Write(ctx context.Context, name string, content []byte) error
	Remove(ctx context.Context, name string) error
	List(ctx context.Context) ([]string, error)
}
