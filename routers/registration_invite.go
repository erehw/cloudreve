package routers

import (
	"errors"
	"io"
	"net/http"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/middleware"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/routers/controllers"
	"github.com/gin-gonic/gin"
)

func registerAccountInviteRoutes(admin *gin.RouterGroup) {
	invites := admin.Group("registration-invites")
	invites.GET("", listRegistrationInvites)
	invites.POST("", middleware.RequiredScopes(types.ScopeAdminWrite), createRegistrationInvite)
	invites.PATCH(":id", middleware.RequiredScopes(types.ScopeAdminWrite), updateRegistrationInviteCode)
	invites.DELETE(":id", middleware.RequiredScopes(types.ScopeAdminWrite), revokeRegistrationInvite)
}

func listRegistrationInvites(c *gin.Context) {
	client := dependency.FromContext(c).DBClient()
	invites, err := inventory.ListRegistrationInvites(c, client)
	if err != nil {
		c.JSON(http.StatusOK, serializer.DBErr(c, "无法读取注册邀请码。", err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{Data: invites})
}

func createRegistrationInvite(c *gin.Context) {
	var input struct {
		Limit *int   `json:"limit" binding:"omitempty,gte=1,lte=2147483647"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeParamErr, "使用次数必须为正整数。", nil))
		return
	}
	limit := inventory.DefaultRegistrationInviteLimit
	if input.Limit != nil {
		limit = *input.Limit
	}
	client := dependency.FromContext(c).DBClient()
	invite, err := inventory.CreateRegistrationInvite(c, client, limit, input.Code)
	writeRegistrationInvite(c, invite, err)
}

func updateRegistrationInviteCode(c *gin.Context) {
	var target struct {
		ID int `uri:"id" binding:"required,gt=0"`
	}
	if err := c.ShouldBindUri(&target); err != nil {
		c.JSON(http.StatusOK, controllers.ErrorResponse(err))
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeParamErr, "请输入新的注册邀请码。", nil))
		return
	}
	client := dependency.FromContext(c).DBClient()
	invite, err := inventory.UpdateRegistrationInviteCode(c, client, target.ID, input.Code)
	writeRegistrationInvite(c, invite, err)
}

func writeRegistrationInvite(c *gin.Context, invite inventory.RegistrationInvite, err error) {
	switch {
	case errors.Is(err, inventory.ErrInvalidRegistrationInviteCode):
		c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeParamErr, "请输入 1 到 128 个字符的邀请码。", nil))
	case errors.Is(err, inventory.ErrRegistrationInviteCodeInUse):
		c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeConflict, "这个邀请码已被使用，请换一个。", nil))
	case ent.IsNotFound(err):
		c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeNotFound, "注册邀请码不存在或已停用。", nil))
	case err != nil:
		c.JSON(http.StatusOK, serializer.DBErr(c, "无法保存注册邀请码。", err))
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, serializer.Response{Data: invite})
	}
}

func revokeRegistrationInvite(c *gin.Context) {
	var input struct {
		ID int `uri:"id" binding:"required,gt=0"`
	}
	if err := c.ShouldBindUri(&input); err != nil {
		c.JSON(http.StatusOK, controllers.ErrorResponse(err))
		return
	}
	client := dependency.FromContext(c).DBClient()
	invite, err := inventory.RevokeRegistrationInvite(c, client, input.ID)
	if ent.IsNotFound(err) {
		c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeNotFound, "注册邀请码不存在。", nil))
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, serializer.DBErr(c, "无法停用注册邀请码。", err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{Data: invite})
}
