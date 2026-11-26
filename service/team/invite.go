package team

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"slices"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/teaminvite"
	"github.com/cloudreve/Cloudreve/v4/ent/teammember"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

const (
	inviteLimit      = 10
	inviteTokenBytes = 32
)

// InviteInfo 是团队管理员可见的邀请状态，原始令牌仅在创建时返回。
type InviteInfo struct {
	ID      int    `json:"id"`
	Token   string `json:"token,omitempty"`
	Limit   int    `json:"limit"`
	Used    int    `json:"used"`
	Revoked bool   `json:"revoked"`
}

// Invites 列出该团队发出的邀请及兑换情况。
func (s *Service) Invites(ctx context.Context, teamID int) ([]InviteInfo, error) {
	if _, admin, err := Access(ctx, s.client, s.actorID, teamID); err != nil {
		return nil, err
	} else if !admin {
		return nil, serializer.NewError(serializer.CodeNoPermissionErr, "只有团队管理员可以查看邀请", nil)
	}
	invites, err := s.client.TeamInvite.Query().Where(teaminvite.TeamIDEQ(teamID)).Order(teaminvite.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]InviteInfo, 0, len(invites))
	for _, invite := range invites {
		result = append(result, inviteInfo(invite, ""))
	}
	return result, nil
}

// CreateInvite 为当前团队签发最多可供十人加入的一条随机邀请。
func (s *Service) CreateInvite(ctx context.Context, teamID int) (InviteInfo, error) {
	var result InviteInfo
	err := s.writeAsAdmin(ctx, teamID, func(tx *ent.Tx, _ *ent.TeamMember) error {
		var secret [inviteTokenBytes]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return err
		}
		token := base64.RawURLEncoding.EncodeToString(secret[:])
		invite, err := tx.TeamInvite.Create().SetTeamID(teamID).SetTokenDigest(tokenDigest(token)).Save(ctx)
		if err != nil {
			return err
		}
		result = inviteInfo(invite, token)
		return nil
	})
	return result, err
}

// RevokeInvite 停用指定团队的邀请，已有成员保留访问权限。
func (s *Service) RevokeInvite(ctx context.Context, teamID, inviteID int) error {
	return s.writeAsAdmin(ctx, teamID, func(tx *ent.Tx, _ *ent.TeamMember) error {
		invite, err := tx.TeamInvite.Query().Where(teaminvite.TeamIDEQ(teamID), teaminvite.IDEQ(inviteID)).Only(ctx)
		if ent.IsNotFound(err) {
			return serializer.NewError(serializer.CodeNotFound, "没有找到该团队的邀请", nil)
		}
		if err != nil || invite.Revoked {
			return err
		}
		return tx.TeamInvite.UpdateOneID(invite.ID).SetRevoked(true).Exec(ctx)
	})
}

// Join 兑换邀请。重复访问保留现有角色，新增成员和扣除名额一起提交。
func (s *Service) Join(ctx context.Context, token string) (TeamInfo, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(inviteTokenBytes) {
		return TeamInfo{}, invalidInvite()
	}
	digest := tokenDigest(token)
	// 首次查询仅用于定位团队写锁。拿到写锁后重新读邀请和成员，才决定是否扣额。
	invite, err := s.client.TeamInvite.Query().Where(teaminvite.TokenDigestEQ(digest)).Only(ctx)
	if ent.IsNotFound(err) {
		return TeamInfo{}, invalidInvite()
	}
	if err != nil {
		return TeamInfo{}, err
	}
	var result TeamInfo
	err = s.writeTeam(ctx, invite.TeamID, func(tx *ent.Tx, team *ent.Team) error {
		current, err := tx.TeamInvite.Get(ctx, invite.ID)
		if ent.IsNotFound(err) || (err == nil && current.Revoked) {
			return invalidInvite()
		}
		if err != nil {
			return err
		}
		membership, err := tx.TeamMember.Query().
			Where(teammember.TeamIDEQ(team.ID), teammember.UserIDEQ(s.actorID)).Only(ctx)
		if err == nil {
			result = TeamInfo{ID: team.ID, Name: team.Name, Role: membership.Role}
			return nil
		}
		if !ent.IsNotFound(err) {
			return err
		}
		if err := checkRedemption(current.RedeemedUserIds, s.actorID); err != nil {
			return err
		}
		if err := tx.TeamMember.Create().SetTeamID(team.ID).SetUserID(s.actorID).SetRole(teammember.RoleMember).Exec(ctx); err != nil {
			return err
		}
		redeemed := append(current.RedeemedUserIds, s.actorID)
		if err := tx.TeamInvite.UpdateOneID(current.ID).SetRedeemedUserIds(redeemed).Exec(ctx); err != nil {
			return err
		}
		result = TeamInfo{ID: team.ID, Name: team.Name, Role: teammember.RoleMember}
		return nil
	})
	return result, err
}

func checkRedemption(redeemed []int, actorID int) error {
	if slices.Contains(redeemed, actorID) {
		return serializer.NewError(serializer.CodeNoPermissionErr, "你已使用过此邀请，请向团队管理员索取新邀请", nil)
	}
	if len(redeemed) >= inviteLimit {
		return serializer.NewError(serializer.CodeConflict, "此邀请的名额已用完", nil)
	}
	return nil
}

func tokenDigest(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func invalidInvite() error {
	return serializer.NewError(serializer.CodeNotFound, "此邀请无效或已撤销", nil)
}

func inviteInfo(invite *ent.TeamInvite, token string) InviteInfo {
	return InviteInfo{ID: invite.ID, Token: token, Limit: inviteLimit, Used: len(invite.RedeemedUserIds), Revoked: invite.Revoked}
}
