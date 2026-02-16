package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

// Artifact holds the schema definition for the Artifact entity.
type Artifact struct {
	ent.Schema
}

// Fields of the Artifact.
func (Artifact) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Unique().
			Immutable(),
		field.String("run_id").
			NotEmpty(),
		field.String("path").
			NotEmpty(),
		field.Time("created_at").
			Default(time.Now),
	}
}

// Edges of the Artifact.
func (Artifact) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("artifacts").
			Unique(),
	}
}

// Indexes of the Artifact.
func (Artifact) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id", "path").Unique(),
	}
}
