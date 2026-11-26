package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// TeamInvite 保存邀请摘要和已用名额，成员离开后兑换记录依然有效。
type TeamInvite struct {
	ent.Schema
}

func (TeamInvite) Fields() []ent.Field {
	return []ent.Field{
		field.Int("team_id").Immutable(),
		field.String("token_digest").MaxLen(64).Unique().Immutable().Sensitive(),
		field.JSON("redeemed_user_ids", []int{}).Default([]int{}),
		field.Bool("revoked").Default(false),
	}
}

func (TeamInvite) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).Ref("invites").Field("team_id").Unique().Required().Immutable(),
	}
}
