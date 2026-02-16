package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

// WorkspaceTab holds the schema definition for the WorkspaceTab entity.
type WorkspaceTab struct {
	ent.Schema
}

// Fields of the WorkspaceTab.
func (WorkspaceTab) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("tab_id").
			Unique().
			Immutable(),
		field.String("workspace_id").
			NotEmpty(),
		field.String("title").
			Default("Tab"),
		field.String("run_id").
			Default(""),
		field.Int("order_index").
			Default(0),
		field.Bool("is_pinned").
			Default(false),
		field.Time("created_at").
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the WorkspaceTab.
func (WorkspaceTab) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workspace", Workspace.Type).
			Ref("tabs").
			Field("workspace_id").
			Unique().
			Required(),
	}
}

// Indexes of the WorkspaceTab.
func (WorkspaceTab) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
	}
}
