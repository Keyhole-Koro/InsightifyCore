package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"time"
)

// Workspace holds the schema definition for the Workspace entity.
type Workspace struct {
	ent.Schema
}

// Fields of the Workspace.
func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("workspace_id").
			Unique().
			Immutable(),
		field.String("project_id").
			Unique().
			NotEmpty(),
		field.String("name").
			Default("Workspace"),
		field.String("active_tab_id").
			Default(""),
		field.Time("created_at").
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Workspace.
func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tabs", WorkspaceTab.Type),
	}
}
