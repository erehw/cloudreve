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
		Limit *int `json:"limit" binding:"omitempty,gte=1,lte=2147483647"`
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
	invite, err := inventory.CreateRegistrationInvite(c, client, limit)
	if err != nil {
		c.JSON(http.StatusOK, serializer.DBErr(c, "无法生成注册邀请码。", err))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, serializer.Response{Data: invite})
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
