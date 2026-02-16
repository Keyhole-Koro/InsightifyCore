package schema

import "entgo.io/ent"

// Artifact holds the schema definition for the Artifact entity.
type Artifact struct {
	ent.Schema
}

// Fields of the Artifact.
func (Artifact) Fields() []ent.Field {
	return nil
}

// Edges of the Artifact.
func (Artifact) Edges() []ent.Edge {
	return nil
}
