package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type TelegramAccount struct {
	ent.Schema
}

func (TelegramAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Comment("Phone number without +"),
		field.String("code").
			Optional().
			Nillable(),
		field.Time("code_at").
			Optional().
			Nillable(),
		field.Enum("state").
			Values("New", "CodeSent", "Active", "Error").Default("New"),
		field.String("status"),
		field.Bytes("session").
			Optional().
			Nillable(),
	}
}

func (TelegramAccount) Edges() []ent.Edge {
	return []ent.Edge{}
}
