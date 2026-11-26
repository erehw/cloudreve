package inventory

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/stretchr/testify/require"
)

func openAccountTestDB(t *testing.T, file string) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite3", file)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	client := ent.NewClient(ent.Driver(entsql.OpenDB("sqlite3", db)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestBootstrapSiteAdministratorResumesAfterFileFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	client := openAccountTestDB(t, filepath.Join(dir, "test.db"))
	require.NoError(t, client.Schema.Create(ctx))
	group, err := client.Group.Create().SetName("Admin").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	require.NoError(t, err)
	require.Equal(t, defaultAdminGroupID, group.ID)

	passwordPath := filepath.Join(dir, "unavailable", initialAdminPasswordFile)
	created, err := bootstrapSiteAdministrator(ctx, client, passwordPath)
	require.Error(t, err)
	require.False(t, created)
	require.Zero(t, client.User.Query().CountX(ctx))
	require.False(t, client.Setting.Query().Where(setting.NameEQ(initialAdminCreated)).ExistX(ctx))

	require.NoError(t, os.Mkdir(filepath.Dir(passwordPath), 0700))
	created, err = bootstrapSiteAdministrator(ctx, client, passwordPath)
	require.NoError(t, err)
	require.True(t, created)
	file, err := os.ReadFile(passwordPath)
	require.NoError(t, err)
	emailLine, passwordLine, ok := strings.Cut(strings.TrimSpace(string(file)), "\n")
	require.True(t, ok)
	email, ok := strings.CutPrefix(emailLine, "邮箱：")
	require.True(t, ok)
	address, err := mail.ParseAddress(email)
	require.NoError(t, err)
	require.Equal(t, email, address.Address)
	require.Regexp(t, `^admin-[a-z2-7]{26}@cloudreve\.invalid$`, email)
	password, ok := strings.CutPrefix(passwordLine, "密码：")
	require.True(t, ok)
	require.GreaterOrEqual(t, len(password), 20)
	admin := client.User.Query().OnlyX(ctx)
	require.Equal(t, email, admin.Email)
	require.Equal(t, user.StatusActive, admin.Status)
	require.Equal(t, defaultAdminGroupID, admin.GroupUsers)
	require.NoError(t, CheckPassword(admin, password))

	users := NewUserClient(client)
	updated, err := users.UpdatePassword(ctx, admin, "NewPassword123")
	require.NoError(t, err)
	created, err = bootstrapSiteAdministrator(ctx, client, passwordPath)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, updated.Password, client.User.Query().OnlyX(ctx).Password)
	unchanged, err := os.ReadFile(passwordPath)
	require.NoError(t, err)
	require.Equal(t, file, unchanged)
}

func TestConcurrentBootstrapSiteAdministratorCreatesOneAccount(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	client := openAccountTestDB(t, filepath.Join(dir, "test.db"))
	require.NoError(t, client.Schema.Create(ctx))
	require.NoError(t, client.Group.Create().SetName("Admin").SetPermissions(&boolset.BooleanSet{}).Exec(ctx))
	passwordPath := filepath.Join(dir, initialAdminPasswordFile)

	type outcome struct {
		created bool
		err     error
	}
	const attempts = 8
	start := make(chan struct{})
	results := make(chan outcome, attempts)
	for range attempts {
		go func() {
			<-start
			created, err := bootstrapSiteAdministrator(ctx, client, passwordPath)
			results <- outcome{created: created, err: err}
		}()
	}
	close(start)
	created := 0
	for range attempts {
		result := <-results
		require.NoError(t, result.err)
		if result.created {
			created++
		}
	}
	require.Equal(t, 1, created)
	admin := client.User.Query().OnlyX(ctx)
	file, err := os.ReadFile(passwordPath)
	require.NoError(t, err)
	_, password, ok := strings.Cut(strings.TrimSpace(string(file)), "\n密码：")
	require.True(t, ok)
	require.True(t, strings.HasPrefix(string(file), "邮箱："+admin.Email+"\n"))
	require.NoError(t, CheckPassword(admin, password))
}

func TestSameVersionMigrationPreservesExistingAccount(t *testing.T) {
	ctx := context.Background()
	client := openAccountTestDB(t, filepath.Join(t.TempDir(), "existing.db"))
	require.NoError(t, client.Schema.Create(ctx))
	require.NoError(t, client.Setting.Create().SetName(DBVersionPrefix+"4.14.0").SetValue("installed").Exec(ctx))
	require.True(t, needMigration(client, ctx, "4.14.0"))
	log := logging.NewConsoleLogger(logging.LevelError)
	store := cache.NewMemoStore("", log)
	require.NoError(t, migrateDefaultStoragePolicy(log, client, ctx))
	require.NoError(t, migrateAdminGroup(log, client, ctx))
	existing, err := NewUserClient(client).Create(ctx, &NewUserArgs{
		Email: "existing@example.com", PlainPassword: "ExistingPassword123", Status: user.StatusActive, GroupID: defaultAdminGroupID,
	})
	require.NoError(t, err)
	_, err = InitializeDBClient(log, client, store, "4.14.0")
	require.NoError(t, err)
	require.False(t, needMigration(client, ctx, "4.14.0"))
	require.True(t, client.Setting.Query().Where(setting.NameEQ(requiredTeamSchema)).ExistX(ctx))
	admin := client.User.Query().OnlyX(ctx)
	require.Equal(t, existing.Email, admin.Email)
	require.Equal(t, existing.Password, admin.Password)
}

func TestChangeRegistrationInviteKeepsUsageAndInvalidatesOldCode(t *testing.T) {
	ctx := context.Background()
	client := openAccountTestDB(t, filepath.Join(t.TempDir(), "invites.db"))
	require.NoError(t, client.Schema.Create(ctx))

	invite, err := CreateRegistrationInvite(ctx, client, 2, "  研发组-2026  ")
	require.NoError(t, err)
	require.Equal(t, "研发组-2026", invite.Code)
	require.NoError(t, ReserveRegistrationInvite(ctx, client, invite.Code))
	_, err = CreateRegistrationInvite(ctx, client, 2, "研发组-2026")
	require.ErrorIs(t, err, ErrRegistrationInviteCodeInUse)
	other, err := CreateRegistrationInvite(ctx, client, 2, "other-code")
	require.NoError(t, err)
	_, err = UpdateRegistrationInviteCode(ctx, client, invite.ID, other.Code)
	require.ErrorIs(t, err, ErrRegistrationInviteCodeInUse)
	_, err = UpdateRegistrationInviteCode(ctx, client, invite.ID, "   ")
	require.ErrorIs(t, err, ErrInvalidRegistrationInviteCode)
	_, err = UpdateRegistrationInviteCode(ctx, client, invite.ID, strings.Repeat("码", 129))
	require.ErrorIs(t, err, ErrInvalidRegistrationInviteCode)

	changed, err := UpdateRegistrationInviteCode(ctx, client, invite.ID, "  业务组-2026  ")
	require.NoError(t, err)
	require.Equal(t, RegistrationInvite{ID: invite.ID, Code: "业务组-2026", Used: 1, Limit: 2}, changed)
	require.ErrorIs(t, ReserveRegistrationInvite(ctx, client, invite.Code), ErrInvalidRegistrationInvite)
	require.NoError(t, ReserveRegistrationInvite(ctx, client, changed.Code))
	require.ErrorIs(t, ReserveRegistrationInvite(ctx, client, changed.Code), ErrInvalidRegistrationInvite)

	listed, err := ListRegistrationInvites(ctx, client)
	require.NoError(t, err)
	require.Equal(t, RegistrationInvite{ID: invite.ID, Used: 2, Limit: 2}, listed[1])
	_, err = RevokeRegistrationInvite(ctx, client, invite.ID)
	require.NoError(t, err)
	_, err = UpdateRegistrationInviteCode(ctx, client, invite.ID, "inactive-code")
	require.True(t, ent.IsNotFound(err))
}

func TestPasswordLoginLockPersistsAndResets(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "login.db")
	client := openAccountTestDB(t, file)
	require.NoError(t, client.Schema.Create(ctx))
	group, err := client.Group.Create().SetName("User").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	require.NoError(t, err)
	users := NewUserClient(client)
	const address = "user@example.com"
	const password = "GoodPassword123"
	u, err := users.Create(ctx, &NewUserArgs{Email: address, PlainPassword: password, Status: user.StatusActive, GroupID: group.ID})
	require.NoError(t, err)
	_, err = users.VerifyPasswordLogin(ctx, address, "wrong")
	require.ErrorIs(t, err, ErrorIncorrectPassword)
	_, err = users.VerifyPasswordLogin(ctx, address, password)
	require.NoError(t, err)
	require.Zero(t, client.User.GetX(ctx, u.ID).PasswordLoginFailures)

	const attempts = 18
	start := make(chan struct{})
	results := make(chan error, attempts)
	started := time.Now()
	for range attempts {
		go func() {
			<-start
			_, err := users.VerifyPasswordLogin(ctx, address, "wrong")
			results <- err
		}()
	}
	close(start)
	rejected := 0
	for range attempts {
		err := <-results
		if errors.Is(err, ErrorIncorrectPassword) {
			rejected++
		} else {
			require.ErrorIs(t, err, ErrPasswordLoginLocked)
		}
	}
	locked := client.User.GetX(ctx, u.ID)
	require.Equal(t, passwordLoginMaxFailures-1, rejected)
	require.Equal(t, passwordLoginMaxFailures, locked.PasswordLoginFailures)
	require.GreaterOrEqual(t, locked.PasswordLoginLockedUntil, started.Add(passwordLoginLockTime).UnixMilli())
	require.LessOrEqual(t, locked.PasswordLoginLockedUntil, time.Now().Add(passwordLoginLockTime).UnixMilli())
	require.NoError(t, client.Close())

	client = openAccountTestDB(t, file)
	users = NewUserClient(client)
	_, err = users.VerifyPasswordLogin(ctx, address, password)
	require.ErrorIs(t, err, ErrPasswordLoginLocked)
	require.Equal(t, locked.PasswordLoginLockedUntil, client.User.GetX(ctx, u.ID).PasswordLoginLockedUntil)

	require.NoError(t, client.User.UpdateOneID(u.ID).SetPasswordLoginLockedUntil(time.Now().Add(-time.Second).UnixMilli()).Exec(ctx))
	_, err = users.VerifyPasswordLogin(ctx, address, "wrong")
	require.ErrorIs(t, err, ErrorIncorrectPassword)
	reset := client.User.GetX(ctx, u.ID)
	require.Equal(t, 1, reset.PasswordLoginFailures)
	require.Zero(t, reset.PasswordLoginLockedUntil)
	_, err = users.VerifyPasswordLogin(ctx, address, password)
	require.NoError(t, err)
}
