package routers

import (
	"errors"
	"net/http"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	teamservice "github.com/cloudreve/Cloudreve/v4/service/team"
	"github.com/gin-gonic/gin"
)

type teamURI struct {
	TeamID   int `uri:"id" binding:"omitempty,gt=0"`
	UserID   int `uri:"uid" binding:"omitempty,gt=0"`
	InviteID int `uri:"inviteId" binding:"omitempty,gt=0"`
}

func registerTeamRoutes(team *gin.RouterGroup) {
	team.GET("", teamEndpoint(func(c *gin.Context, s *teamservice.Service, _ teamURI) (any, error) {
		return s.List(c)
	}))
	team.POST("", teamEndpoint(func(c *gin.Context, s *teamservice.Service, _ teamURI) (any, error) {
		var request struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "请输入团队名称", nil)
		}
		groupID := dependency.FromContext(c).SettingProvider().DefaultGroup(c)
		return s.Create(c, request.Name, groupID)
	}))
	team.POST("/join", teamEndpoint(func(c *gin.Context, s *teamservice.Service, _ teamURI) (any, error) {
		var request struct {
			Token string `json:"token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "请输入团队邀请令牌", nil)
		}
		return s.Join(c, request.Token)
	}))
	team.GET("/:id", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return s.Detail(c, uri.TeamID)
	}))
	team.GET("/:id/members", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return s.Members(c, uri.TeamID)
	}))
	team.DELETE("/:id/members/:uid", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return nil, s.RemoveMember(c, uri.TeamID, uri.UserID)
	}))
	team.POST("/:id/members/:uid/transfer", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return nil, s.Transfer(c, uri.TeamID, uri.UserID)
	}))
	team.DELETE("/:id/membership", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return nil, s.Leave(c, uri.TeamID)
	}))
	team.GET("/:id/invites", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return s.Invites(c, uri.TeamID)
	}))
	team.POST("/:id/invites", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return s.CreateInvite(c, uri.TeamID)
	}))
	team.DELETE("/:id/invites/:inviteId", teamEndpoint(func(c *gin.Context, s *teamservice.Service, uri teamURI) (any, error) {
		return nil, s.RevokeInvite(c, uri.TeamID, uri.InviteID)
	}))
}

func teamEndpoint(action func(*gin.Context, *teamservice.Service, teamURI) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		var uri teamURI
		if err := c.ShouldBindUri(&uri); err != nil ||
			(c.Param("id") != "" && uri.TeamID <= 0) ||
			(c.Param("uid") != "" && uri.UserID <= 0) ||
			(c.Param("inviteId") != "" && uri.InviteID <= 0) {
			c.JSON(http.StatusOK, serializer.ErrWithDetails(c, serializer.CodeParamErr, "团队、成员或邀请编号无效", nil))
			return
		}
		actor := inventory.UserFromContext(c)
		s := teamservice.New(dependency.FromContext(c).DBClient(), actor.ID)
		data, err := action(c, s, uri)
		if err != nil {
			var appError serializer.AppError
			if errors.As(err, &appError) {
				c.JSON(http.StatusOK, serializer.Err(c, err))
			} else {
				c.JSON(http.StatusOK, serializer.DBErr(c, "无法完成团队操作", err))
			}
			return
		}
		c.JSON(http.StatusOK, serializer.Response{Data: data})
	}
}
