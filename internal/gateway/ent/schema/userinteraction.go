package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"time"
)

// UserInteraction holds the schema definition for the UserInteraction entity.
type UserInteraction struct {
	ent.Schema
}

// Fields of the UserInteraction.
func (UserInteraction) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("run_id").
			Unique().
			Immutable(),
		field.Int64("version").
			Default(0),
		field.JSON("nodes", map[string]any{}).
			Default(map[string]any{}),
		field.Time("created_at").
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the UserInteraction.
func (UserInteraction) Edges() []ent.Edge {
	return nil
}
