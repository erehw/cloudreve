package inventory

import (
	"context"
	"database/sql"
	"errors"
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

	require.NoError(t, os.Mkdir(filepath.Dir(passwordPath), 0700))
	created, err = bootstrapSiteAdministrator(ctx, client, passwordPath)
	require.NoError(t, err)
	require.True(t, created)
	file, err := os.ReadFile(passwordPath)
	require.NoError(t, err)
	password := strings.TrimSpace(string(file))
	require.GreaterOrEqual(t, len(password), 20)
	admin := client.User.Query().OnlyX(ctx)
	require.Equal(t, initialAdminEmail, admin.Email)
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
	require.Equal(t, existing.Password, client.User.Query().OnlyX(ctx).Password)
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
