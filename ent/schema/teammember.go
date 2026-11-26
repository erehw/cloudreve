package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamMember 保存团队与站点账号之间当前有效的成员关系。
type TeamMember struct {
	ent.Schema
}

func (TeamMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int("team_id").Immutable(),
		field.Int("user_id").Immutable(),
		field.Enum("role").Values("admin", "member"),
	}
}

func (TeamMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).Ref("members").Field("team_id").Unique().Required().Immutable(),
		edge.To("user", User.Type).Field("user_id").Unique().Required().Immutable(),
	}
}

func (TeamMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id", "user_id").Unique(),
		index.Fields("user_id"),
	}
}
