package inventory

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/schema"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/debug"
)

const (
	initialAdminPasswordFile = "管理员密码.txt"
	initialAdminCreated      = "db_initial_admin_created"
	defaultAdminGroupID      = 1
)

// bootstrapSiteAdministrator 在系统用户组建好后、写入数据库版本前运行。
// 事务内先取得初始化标记，再创建账号并发布凭据文件，最后提交。文件写入失败时
// 标记和账号都回滚，下次启动可以重试。
func bootstrapSiteAdministrator(ctx context.Context, client *ent.Client, passwordPath string) (bool, error) {
	installed, err := client.Setting.Query().Where(setting.NameHasPrefix(DBVersionPrefix)).Exist(ctx)
	if err != nil || installed {
		return false, err
	}
	existingUsers, err := client.User.Query().Exist(schema.SkipSoftDelete(ctx))
	if err != nil || existingUsers {
		return false, err
	}

	ctx = context.WithValue(ctx, debug.SkipDbLogging{}, true)
	tx, err := client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// 利用设置名的唯一约束，让同时启动的进程中只有一个能写入凭据文件。
	err = tx.Setting.Create().SetName(initialAdminCreated).SetValue("true").Exec(ctx)
	if ent.IsConstraintError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	email := "admin-" + strings.ToLower(rand.Text()) + "@cloudreve.invalid"
	password := rand.Text()
	users := userClient{client: tx.Client()}
	if _, err = users.createUser(ctx, &NewUserArgs{
		Email:         email,
		Nick:          "Admin",
		PlainPassword: password,
		Status:        user.StatusActive,
		GroupID:       defaultAdminGroupID,
	}); err != nil {
		return false, err
	}
	if err = writeInitialAdminCredentials(passwordPath, email, password); err != nil {
		return false, fmt.Errorf("无法写入管理员密码文件: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func writeInitialAdminCredentials(path, email, password string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".管理员密码-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := fmt.Fprintf(file, "邮箱：%s\n密码：%s\n", email, password); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
