package inventory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/registrationinvite"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
)

const (
	DefaultRegistrationInviteLimit = 10
	DefaultUserGroupID             = 2
)

var (
	ErrInvalidRegistrationInvite = errors.New("registration invite is missing, invalid, revoked, or exhausted")
	ErrInvalidRegistrationGroup  = errors.New("public registration requires a regular user group")
)

type RegistrationInvite struct {
	ID      int    `json:"id"`
	Code    string `json:"code,omitempty"`
	Limit   int    `json:"limit"`
	Used    int    `json:"used"`
	Revoked bool   `json:"revoked"`
}

func CreateRegistrationInvite(ctx context.Context, client *ent.Client, limit int) (RegistrationInvite, error) {
	code := rand.Text()
	invite, err := client.RegistrationInvite.Create().SetCodeDigest(registrationInviteDigest(code)).SetLimit(limit).Save(ctx)
	if err != nil {
		return RegistrationInvite{}, err
	}
	result := registrationInviteResult(invite)
	result.Code = code
	return result, nil
}

func ListRegistrationInvites(ctx context.Context, client *ent.Client) ([]RegistrationInvite, error) {
	invites, err := client.RegistrationInvite.Query().Order(registrationinvite.ByID(sql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]RegistrationInvite, 0, len(invites))
	for _, invite := range invites {
		result = append(result, registrationInviteResult(invite))
	}
	return result, nil
}

func RevokeRegistrationInvite(ctx context.Context, client *ent.Client, id int) (RegistrationInvite, error) {
	invite, err := client.RegistrationInvite.UpdateOneID(id).SetRevoked(true).Save(ctx)
	if err != nil {
		return RegistrationInvite{}, err
	}
	return registrationInviteResult(invite), nil
}

// ReserveRegistrationInvite 必须在注册事务的第一条数据库语句调用。先占用名额可避免
// 并发超额及 SQLite 读事务升级失败，后续注册失败时由同一个事务回滚名额。
func ReserveRegistrationInvite(ctx context.Context, client *ent.Client, code string) error {
	if strings.TrimSpace(code) == "" {
		return ErrInvalidRegistrationInvite
	}
	changed, err := client.RegistrationInvite.Update().Where(
		registrationinvite.CodeDigestEQ(registrationInviteDigest(code)),
		registrationinvite.RevokedEQ(false),
		sql.FieldsLT(registrationinvite.FieldUsed, registrationinvite.FieldLimit),
	).AddUsed(1).Save(ctx)
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrInvalidRegistrationInvite
	}
	return nil
}

// ReleaseRegistrationInvite 归还本事务为续注册预留的名额，续注册只更新已有待激活账号。
func ReleaseRegistrationInvite(ctx context.Context, client *ent.Client, code string) error {
	_, err := client.RegistrationInvite.Update().Where(
		registrationinvite.CodeDigestEQ(registrationInviteDigest(code)),
	).AddUsed(-1).Save(ctx)
	return err
}

func RegistrationGroup(ctx context.Context, client *ent.Client, preferredID int) (int, error) {
	preferred, err := client.Group.Get(ctx, preferredID)
	if err != nil && !ent.IsNotFound(err) {
		return 0, err
	}
	if isRegistrationGroup(preferred) {
		return preferredID, nil
	}
	if preferredID != DefaultUserGroupID {
		fallback, err := client.Group.Get(ctx, DefaultUserGroupID)
		if err != nil && !ent.IsNotFound(err) {
			return 0, err
		}
		if isRegistrationGroup(fallback) {
			return fallback.ID, nil
		}
	}
	return 0, ErrInvalidRegistrationGroup
}

func isRegistrationGroup(group *ent.Group) bool {
	return group != nil && group.Permissions != nil &&
		!group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) &&
		!group.Permissions.Enabled(int(types.GroupPermissionIsAnonymous))
}

func registrationInviteResult(invite *ent.RegistrationInvite) RegistrationInvite {
	return RegistrationInvite{ID: invite.ID, Limit: invite.Limit, Used: invite.Used, Revoked: invite.Revoked}
}

func registrationInviteDigest(code string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(digest[:])
}
