package team

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudreve/Cloudreve/v4/ent"
	teamdb "github.com/cloudreve/Cloudreve/v4/ent/team"
	"github.com/cloudreve/Cloudreve/v4/ent/teammember"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

const maxNameLength = 100

// Service 管理当前登录用户的团队及成员关系。
type Service struct {
	client  *ent.Client
	actorID int
}

// TeamInfo 是当前用户可见的团队资料及其角色。
type TeamInfo struct {
	ID   int             `json:"id"`
	Name string          `json:"name"`
	Role teammember.Role `json:"role"`
}

// MemberInfo 是团队成员的公开资料及其角色。
type MemberInfo struct {
	ID    int             `json:"id"`
	Nick  string          `json:"nick"`
	Email string          `json:"email"`
	Role  teammember.Role `json:"role"`
}

// New 为已认证的账号创建团队服务。
func New(client *ent.Client, actorID int) *Service {
	return &Service{client: client, actorID: actorID}
}

// Access 按当前成员关系授权文件请求，并返回团队和管理员身份。
func Access(ctx context.Context, client *ent.Client, actorID, teamID int) (*ent.Team, bool, error) {
	membership, err := findMembership(ctx, client, actorID, teamID)
	if ent.IsNotFound(err) || (err == nil && membership.Edges.Team == nil) {
		return nil, false, serializer.NewError(serializer.CodeNoPermissionErr, "你不是该团队的成员", nil)
	}
	if err != nil {
		return nil, false, serializer.NewError(serializer.CodeDBError, "无法验证团队成员身份", err)
	}
	return membership.Edges.Team, membership.Role == teammember.RoleAdmin, nil
}

// List 列出当前账号加入的团队。
func (s *Service) List(ctx context.Context) ([]TeamInfo, error) {
	memberships, err := s.client.TeamMember.Query().
		Where(teammember.UserIDEQ(s.actorID)).WithTeam().Order(teammember.ByTeamID()).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]TeamInfo, 0, len(memberships))
	for _, membership := range memberships {
		if team := membership.Edges.Team; team != nil {
			result = append(result, TeamInfo{ID: team.ID, Name: team.Name, Role: membership.Role})
		}
	}
	return result, nil
}

// Create 原子创建独立存储账号、团队及首位管理员。
func (s *Service) Create(ctx context.Context, name string, defaultGroupID int) (TeamInfo, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return TeamInfo{}, serializer.NewError(serializer.CodeParamErr, "团队名称须为 1 至 100 个字符", nil)
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return TeamInfo{}, err
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return TeamInfo{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Group.Get(ctx, defaultGroupID); err != nil {
		return TeamInfo{}, err
	}
	// 存储账号仅供文件管理器使用。团队角色变化不影响文件和版本的归属。
	owner, err := tx.User.Create().
		SetEmail(hex.EncodeToString(identity[:]) + "@team-storage.invalid").
		SetNick("团队存储").
		SetStatus(user.StatusSysBanned).
		SetTeamStorage(true).
		SetGroupID(defaultGroupID).
		SetSettings(&types.UserSetting{VersionRetention: true, VersionRetentionMax: 10}).
		Save(ctx)
	if err != nil {
		return TeamInfo{}, err
	}
	team, err := tx.Team.Create().SetName(name).SetStorageOwnerID(owner.ID).Save(ctx)
	if err != nil {
		return TeamInfo{}, err
	}
	if err := tx.TeamMember.Create().SetTeamID(team.ID).SetUserID(s.actorID).SetRole(teammember.RoleAdmin).Exec(ctx); err != nil {
		return TeamInfo{}, err
	}
	if err := tx.Commit(); err != nil {
		return TeamInfo{}, err
	}
	return TeamInfo{ID: team.ID, Name: team.Name, Role: teammember.RoleAdmin}, nil
}

// Detail 查询当前用户所属团队的资料。
func (s *Service) Detail(ctx context.Context, teamID int) (TeamInfo, error) {
	team, admin, err := Access(ctx, s.client, s.actorID, teamID)
	if err != nil {
		return TeamInfo{}, err
	}
	role := teammember.RoleMember
	if admin {
		role = teammember.RoleAdmin
	}
	return TeamInfo{ID: team.ID, Name: team.Name, Role: role}, nil
}

// Members 列出同一团队中的账号及其团队角色。
func (s *Service) Members(ctx context.Context, teamID int) ([]MemberInfo, error) {
	if _, _, err := Access(ctx, s.client, s.actorID, teamID); err != nil {
		return nil, err
	}
	members, err := s.client.TeamMember.Query().Where(teammember.TeamIDEQ(teamID)).
		WithUser().Order(teammember.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]MemberInfo, 0, len(members))
	for _, membership := range members {
		if member := membership.Edges.User; member != nil {
			result = append(result, MemberInfo{ID: member.ID, Nick: member.Nick, Email: member.Email, Role: membership.Role})
		}
	}
	return result, nil
}

// RemoveMember 移除指定成员。现任管理员须先交接管理权。
func (s *Service) RemoveMember(ctx context.Context, teamID, userID int) error {
	return s.writeAsAdmin(ctx, teamID, func(tx *ent.Tx, _ *ent.TeamMember) error {
		return deleteMember(ctx, tx, teamID, userID)
	})
}

// Leave 退出指定团队。现任管理员须先交接管理权。
func (s *Service) Leave(ctx context.Context, teamID int) error {
	return s.writeTeam(ctx, teamID, func(tx *ent.Tx, _ *ent.Team) error {
		return deleteMember(ctx, tx, teamID, s.actorID)
	})
}

// Transfer 一次事务中交接管理员，并保留原管理员的成员资格。
func (s *Service) Transfer(ctx context.Context, teamID, userID int) error {
	return s.writeAsAdmin(ctx, teamID, func(tx *ent.Tx, admin *ent.TeamMember) error {
		if userID == s.actorID {
			return serializer.NewError(serializer.CodeParamErr, "请选择其他成员接任管理员", nil)
		}
		nextAdmin, err := tx.TeamMember.Query().
			Where(teammember.TeamIDEQ(teamID), teammember.UserIDEQ(userID), teammember.HasUserWith(user.StatusEQ(user.StatusActive), user.DeletedAtIsNil())).
			Only(ctx)
		if ent.IsNotFound(err) {
			return serializer.NewError(serializer.CodeNotFound, "没有找到可接任管理员的成员", nil)
		}
		if err != nil {
			return err
		}
		if err := tx.TeamMember.UpdateOneID(nextAdmin.ID).SetRole(teammember.RoleAdmin).Exec(ctx); err != nil {
			return err
		}
		return tx.TeamMember.UpdateOneID(admin.ID).SetRole(teammember.RoleMember).Exec(ctx)
	})
}

func findMembership(ctx context.Context, client *ent.Client, userID, teamID int) (*ent.TeamMember, error) {
	return client.TeamMember.Query().Where(teammember.TeamIDEQ(teamID), teammember.UserIDEQ(userID)).WithTeam().Only(ctx)
}

func deleteMember(ctx context.Context, tx *ent.Tx, teamID, userID int) error {
	member, err := tx.TeamMember.Query().Where(teammember.TeamIDEQ(teamID), teammember.UserIDEQ(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return serializer.NewError(serializer.CodeNotFound, "没有找到该团队成员", nil)
	}
	if err != nil {
		return err
	}
	if member.Role == teammember.RoleAdmin {
		return serializer.NewError(serializer.CodeConflict, "请先交接管理权", nil)
	}
	return tx.TeamMember.DeleteOneID(member.ID).Exec(ctx)
}

func (s *Service) writeAsAdmin(ctx context.Context, teamID int, action func(*ent.Tx, *ent.TeamMember) error) error {
	return s.writeTeam(ctx, teamID, func(tx *ent.Tx, _ *ent.Team) error {
		admin, err := tx.TeamMember.Query().
			Where(teammember.TeamIDEQ(teamID), teammember.UserIDEQ(s.actorID), teammember.RoleEQ(teammember.RoleAdmin)).Only(ctx)
		if ent.IsNotFound(err) {
			return serializer.NewError(serializer.CodeNoPermissionErr, "只有团队管理员可以执行此操作", nil)
		}
		if err != nil {
			return err
		}
		return action(tx, admin)
	})
}

func (s *Service) writeTeam(ctx context.Context, teamID int, action func(*ent.Tx, *ent.Team) error) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 先写团队行以取得数据库写锁，再读成员和邀请。所有团队变更遵循此顺序，
	// 同时加入、移除、撤销邀请和交接时，角色与名额以拿到写锁后的状态为准。
	if err := tx.Team.Update().Where(teamdb.IDEQ(teamID), teamdb.DeletedAtIsNil()).SetUpdatedAt(time.Now()).Exec(ctx); err != nil {
		return err
	}
	team, err := tx.Team.Get(ctx, teamID)
	if ent.IsNotFound(err) {
		return serializer.NewError(serializer.CodeNoPermissionErr, "你不是该团队的成员", nil)
	}
	if err != nil {
		return err
	}
	if err := action(tx, team); err != nil {
		return err
	}
	return tx.Commit()
}
