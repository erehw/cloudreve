package team

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/ent/teammember"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func teamFixture(t *testing.T) (*ent.Client, int) {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "team.db")) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	group, err := client.Group.Create().SetName("默认用户组").SetPermissions(&boolset.BooleanSet{}).Save(t.Context())
	require.NoError(t, err)
	return client, group.ID
}

func teamUser(t *testing.T, client *ent.Client, groupID, index int) *Service {
	t.Helper()
	account, err := client.User.Create().
		SetEmail(fmt.Sprintf("team-user-%d@example.test", index)).
		SetNick(fmt.Sprintf("成员 %d", index)).SetGroupID(groupID).Save(t.Context())
	require.NoError(t, err)
	return New(client, account.ID)
}

func requireTeamError(t *testing.T, code int, err error) {
	t.Helper()
	var appError serializer.AppError
	require.ErrorAs(t, err, &appError)
	require.Equal(t, code, appError.Code)
}

func TestTeamOwnershipAndMemberPermissions(t *testing.T) {
	ctx := t.Context()
	client, groupID := teamFixture(t)
	admin := teamUser(t, client, groupID, 1)
	member := teamUser(t, client, groupID, 2)
	outsider := teamUser(t, client, groupID, 3)
	name := strings.Repeat("文", 100)
	team, err := admin.Create(ctx, " "+name+" ", groupID)
	require.NoError(t, err)
	require.Equal(t, name, team.Name)
	model, isAdmin, err := Access(ctx, client, admin.actorID, team.ID)
	require.NoError(t, err)
	require.True(t, isAdmin)
	owner, err := client.User.Get(ctx, model.StorageOwnerID)
	require.NoError(t, err)
	require.True(t, owner.TeamStorage)
	require.Equal(t, user.StatusSysBanned, owner.Status)
	require.Empty(t, owner.Password)
	ownerGroupID, err := owner.QueryGroup().OnlyID(ctx)
	require.NoError(t, err)
	require.Equal(t, groupID, ownerGroupID)
	require.True(t, owner.Settings.VersionRetention)
	require.Equal(t, 10, owner.Settings.VersionRetentionMax)

	_, err = outsider.Detail(ctx, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	_, err = outsider.Members(ctx, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	requireTeamError(t, serializer.CodeConflict, admin.Leave(ctx, team.ID))
	requireTeamError(t, serializer.CodeConflict, admin.RemoveMember(ctx, team.ID, admin.actorID))
	requireTeamError(t, serializer.CodeNotFound, admin.Transfer(ctx, team.ID, outsider.actorID))

	invite, err := admin.CreateInvite(ctx, team.ID)
	require.NoError(t, err)
	joined, err := member.Join(ctx, invite.Token)
	require.NoError(t, err)
	require.Equal(t, teammember.RoleMember, joined.Role)
	_, err = member.Invites(ctx, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	_, err = member.CreateInvite(ctx, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	requireTeamError(t, serializer.CodeNoPermissionErr, member.RemoveMember(ctx, team.ID, admin.actorID))
	requireTeamError(t, serializer.CodeNoPermissionErr, member.Transfer(ctx, team.ID, outsider.actorID))

	results, err := member.List(ctx)
	require.NoError(t, err)
	require.Equal(t, []TeamInfo{joined}, results)
	members, err := member.Members(ctx, team.ID)
	require.NoError(t, err)
	require.Equal(t, []MemberInfo{
		{ID: admin.actorID, Nick: "成员 1", Email: "team-user-1@example.test", Role: teammember.RoleAdmin},
		{ID: member.actorID, Nick: "成员 2", Email: "team-user-2@example.test", Role: teammember.RoleMember},
	}, members)

	require.NoError(t, admin.Transfer(ctx, team.ID, member.actorID))
	requireTeamError(t, serializer.CodeNoPermissionErr, admin.RevokeInvite(ctx, team.ID, invite.ID))
	requireTeamError(t, serializer.CodeConflict, member.Leave(ctx, team.ID))
	require.NoError(t, admin.Leave(ctx, team.ID))
	_, _, err = Access(ctx, client, admin.actorID, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	updated, isAdmin, err := Access(ctx, client, member.actorID, team.ID)
	require.NoError(t, err)
	require.True(t, isAdmin)
	require.Equal(t, model.StorageOwnerID, updated.StorageOwnerID)
}

func TestTeamInviteConcurrentLimitRevokeAndRemoval(t *testing.T) {
	ctx := t.Context()
	client, groupID := teamFixture(t)
	admin := teamUser(t, client, groupID, 1)
	team, err := admin.Create(ctx, "团队", groupID)
	require.NoError(t, err)
	otherTeam, err := admin.Create(ctx, "另一个团队", groupID)
	require.NoError(t, err)
	invite, err := admin.CreateInvite(ctx, team.ID)
	require.NoError(t, err)
	require.NotEmpty(t, invite.Token)
	require.Equal(t, 10, invite.Limit)
	requireTeamError(t, serializer.CodeNotFound, admin.RevokeInvite(ctx, otherTeam.ID, invite.ID))

	accounts := make([]*Service, 12)
	for i := range accounts {
		accounts[i] = teamUser(t, client, groupID, i+2)
	}
	start := make(chan struct{})
	results := make([]error, len(accounts)*2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			<-start
			_, results[i] = accounts[i/2].Join(ctx, invite.Token)
		})
	}
	close(start)
	wg.Wait()

	var removed *Service
	joined := 0
	for i, account := range accounts {
		if results[i*2] == nil {
			require.NoError(t, results[i*2+1])
			joined++
			removed = account
		} else {
			requireTeamError(t, serializer.CodeConflict, results[i*2])
			requireTeamError(t, serializer.CodeConflict, results[i*2+1])
		}
	}
	require.Equal(t, 10, joined)
	require.NoError(t, admin.RemoveMember(ctx, team.ID, removed.actorID))
	_, _, err = Access(ctx, client, removed.actorID, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	_, err = removed.Join(ctx, invite.Token)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)

	invites, err := admin.Invites(ctx, team.ID)
	require.NoError(t, err)
	require.Equal(t, []InviteInfo{{ID: invite.ID, Limit: 10, Used: 10}}, invites)
	require.NoError(t, admin.RevokeInvite(ctx, team.ID, invite.ID))
	require.NoError(t, admin.RevokeInvite(ctx, team.ID, invite.ID))
	_, err = removed.Join(ctx, invite.Token)
	requireTeamError(t, serializer.CodeNotFound, err)

	newInvite, err := admin.CreateInvite(ctx, team.ID)
	require.NoError(t, err)
	_, err = removed.Join(ctx, newInvite.Token)
	require.NoError(t, err)
	invites, err = admin.Invites(ctx, team.ID)
	require.NoError(t, err)
	require.Equal(t, []InviteInfo{{ID: invite.ID, Limit: 10, Used: 10, Revoked: true}, {ID: newInvite.ID, Limit: 10, Used: 1}}, invites)
}

func TestTeamFailedCreationRollsBackStorageOwner(t *testing.T) {
	client, groupID := teamFixture(t)
	ctx := context.Background()
	_, err := New(client, 100).Create(ctx, "团队", groupID)
	require.Error(t, err)
	owners, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, owners)
	teams, err := client.Team.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, teams)
}

func TestTeamInviteFailedRedemptionRollsBackMembership(t *testing.T) {
	ctx := t.Context()
	client, groupID := teamFixture(t)
	admin := teamUser(t, client, groupID, 1)
	member := teamUser(t, client, groupID, 2)
	team, err := admin.Create(ctx, "团队", groupID)
	require.NoError(t, err)
	invite, err := admin.CreateInvite(ctx, team.ID)
	require.NoError(t, err)

	writeErr := errors.New("邀请扣额失败")
	client.TeamInvite.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Op() == ent.OpUpdateOne {
				return nil, writeErr
			}
			return next.Mutate(ctx, mutation)
		})
	})
	_, err = member.Join(ctx, invite.Token)
	require.ErrorIs(t, err, writeErr)
	_, _, err = Access(ctx, client, member.actorID, team.ID)
	requireTeamError(t, serializer.CodeNoPermissionErr, err)
	invites, err := admin.Invites(ctx, team.ID)
	require.NoError(t, err)
	require.Equal(t, []InviteInfo{{ID: invite.ID, Limit: 10}}, invites)
}
