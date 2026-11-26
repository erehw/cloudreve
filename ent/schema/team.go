package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Team 保存团队及其独立的文件归属账号。
type Team struct {
	ent.Schema
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Int("storage_owner_id").Unique().Immutable(),
	}
}

func (Team) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("storage_owner", User.Type).Field("storage_owner_id").Unique().Required().Immutable(),
		edge.To("members", TeamMember.Type),
		edge.To("invites", TeamInvite.Type),
	}
}

func (Team) Mixin() []ent.Mixin {
	return []ent.Mixin{CommonMixin{}}
}
