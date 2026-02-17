package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ArtifactFile holds binary artifact content by (run_id, path).
type ArtifactFile struct {
	ent.Schema
}

func (ArtifactFile) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Unique().
			Immutable(),
		field.String("run_id").
			NotEmpty(),
		field.String("path").
			NotEmpty(),
		field.Bytes("content").
			Default([]byte{}),
		field.Int64("size").
			NonNegative(),
		field.Time("created_at").
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (ArtifactFile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id", "path").Unique(),
		index.Fields("run_id"),
	}
}
