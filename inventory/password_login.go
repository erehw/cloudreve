package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
)

const (
	passwordLoginMaxFailures = 10
	passwordLoginLockTime    = 3 * time.Minute
)

var ErrPasswordLoginLocked = errors.New("password login temporarily locked")

// VerifyPasswordLogin 以读取时的密码和锁定状态为条件写入。并发请求修改状态时重新校验，
// 确保失败次数不丢失，且已被改掉的密码不会用于登录或计数。
func (c *userClient) VerifyPasswordLogin(ctx context.Context, email, password string) (*ent.User, error) {
	for {
		u, err := c.GetByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		if u.Password == "" {
			return nil, ErrorIncorrectPassword
		}
		now := time.Now()
		if u.PasswordLoginLockedUntil > now.UnixMilli() {
			return nil, ErrPasswordLoginLocked
		}

		passwordErr := CheckPassword(u, password)
		failures, lockedUntil := nextPasswordLoginState(u.PasswordLoginFailures, u.PasswordLoginLockedUntil, now, passwordErr == nil)
		if failures != u.PasswordLoginFailures || lockedUntil != u.PasswordLoginLockedUntil {
			changed, err := c.client.User.Update().Where(
				user.IDEQ(u.ID), user.EmailEQ(u.Email), user.PasswordEQ(u.Password), user.DeletedAtIsNil(), user.TeamStorageEQ(false),
				user.PasswordLoginFailuresEQ(u.PasswordLoginFailures),
				user.PasswordLoginLockedUntilEQ(u.PasswordLoginLockedUntil),
			).SetPasswordLoginFailures(failures).SetPasswordLoginLockedUntil(lockedUntil).Save(ctx)
			if err != nil {
				return nil, err
			}
			if changed == 0 {
				continue
			}
		}

		if lockedUntil > now.UnixMilli() {
			return nil, ErrPasswordLoginLocked
		}
		if passwordErr != nil {
			return nil, ErrorIncorrectPassword
		}
		return u, nil
	}
}

func nextPasswordLoginState(failures int, lockedUntil int64, now time.Time, correct bool) (int, int64) {
	if correct {
		return 0, 0
	}
	if lockedUntil != 0 {
		// 调用方已拒绝未到期的锁；期限已过时从本次失败重新计数。
		failures = 0
	}
	failures++
	if failures >= passwordLoginMaxFailures {
		return failures, now.Add(passwordLoginLockTime).UnixMilli()
	}
	return failures, 0
}
