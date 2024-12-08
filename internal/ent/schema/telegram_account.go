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
		field.Bytes("data"),
		field.Enum("state").
			Values("New", "CodeSent", "Active", "Error"),
		field.String("status"),
	}
}

func (TelegramAccount) Edges() []ent.Edge {
	return []ent.Edge{}
}
