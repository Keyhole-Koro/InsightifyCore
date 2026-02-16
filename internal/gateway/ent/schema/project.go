package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Project holds the schema definition for the Project entity.
type Project struct {
	ent.Schema
}

// Fields of the Project.
func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("project_id").
			Unique().
			Immutable(),
		field.String("name").
			StorageKey("project_name").
			Default("Project"),
		field.String("user_id").
			Default(""),
		field.String("repo").
			Default(""),
		field.Bool("is_active").
			Default(false),
	}
}

// Edges of the Project.
func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("artifacts", Artifact.Type),
	}
}
