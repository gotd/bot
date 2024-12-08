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
		field.String("code"),
		field.Time("code_at"),
		field.Enum("state").
			Values("New", "CodeSent", "Active", "Error").Default("New"),
		field.String("status"),
		field.Bytes("session").Nillable(),
	}
}

func (TelegramAccount) Edges() []ent.Edge {
	return []ent.Edge{}
}
