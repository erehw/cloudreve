package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type RegistrationInvite struct {
	ent.Schema
}

func (RegistrationInvite) Fields() []ent.Field {
	return []ent.Field{
		field.String("code_digest").MaxLen(64).Unique().Sensitive(),
		field.Int("limit").Positive().Immutable(),
		field.Int("used").Default(0).NonNegative(),
		field.Bool("revoked").Default(false),
	}
}

func (RegistrationInvite) Mixin() []ent.Mixin {
	return []ent.Mixin{CommonMixin{}}
}
