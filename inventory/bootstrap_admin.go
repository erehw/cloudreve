package inventory

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/schema"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/debug"
)

const (
	initialAdminEmail        = "tom@nona.cn"
	initialAdminPasswordFile = "管理员密码.txt"
	defaultAdminGroupID      = 1
)

// bootstrapSiteAdministrator 在系统用户组建好后、写入数据库版本前运行。
// 先在未提交的事务里创建账号，再发布密码文件，最后提交。文件写入失败时账号回滚，
// 下次启动可重新初始化；账号一旦提交，重启就保留其现有密码。
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

	password := rand.Text()
	users := userClient{client: tx.Client()}
	if _, err = users.createUser(ctx, &NewUserArgs{
		Email:         initialAdminEmail,
		PlainPassword: password,
		Status:        user.StatusActive,
		GroupID:       defaultAdminGroupID,
	}); err != nil {
		return false, err
	}
	if err = writeInitialAdminPassword(passwordPath, password); err != nil {
		return false, fmt.Errorf("无法写入管理员密码文件: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func writeInitialAdminPassword(path, password string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".管理员密码-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.WriteString(password + "\n"); err != nil {
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
