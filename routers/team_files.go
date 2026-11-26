package routers

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudreve/Cloudreve/v4/application/constants"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/middleware"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/dbfs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager/entitysource"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/team"
	"github.com/gin-gonic/gin"
)

type teamFiles struct {
	ctx      context.Context
	manager  manager.FileManager
	ownerKey string
	pageSize int
}

type teamFileInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size"`
	Modified  time.Time `json:"modified"`
}

type teamTrashEntry struct {
	teamFileInfo
	ID int `json:"id"`
}

type teamFileVersion struct {
	ID      int       `json:"id"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	Current bool      `json:"current"`
}

type teamFileAction func(*teamFiles, *gin.Context) (any, error)

func registerTeamFileRoutes(group *gin.RouterGroup) {
	read := middleware.RequiredScopes(types.ScopeFilesRead)
	write := middleware.RequiredScopes(types.ScopeFilesWrite)
	files := group.Group("/:id/files", read)
	files.GET("", teamFileHandler(false, (*teamFiles).list))
	files.POST("", write, teamFileHandler(false, (*teamFiles).createFolder))
	files.PUT("", write, teamFileHandler(false, (*teamFiles).upload))
	files.PATCH("", write, teamFileHandler(false, (*teamFiles).move))
	files.DELETE("", write, teamFileHandler(false, (*teamFiles).softDelete))
	files.GET("/download", teamFileHandler(false, (*teamFiles).download))
	files.GET("/versions", teamFileHandler(false, (*teamFiles).versions))
	files.POST("/versions", write, teamFileHandler(false, (*teamFiles).restoreVersion))

	trash := group.Group("/:id/trash", read)
	trash.GET("", teamFileHandler(true, (*teamFiles).listTrash))
	trash.POST("/:entryId", write, teamFileHandler(true, (*teamFiles).restoreTrash))
	trash.DELETE("/:entryId", write, teamFileHandler(true, (*teamFiles).purgeTrash))
}

func teamFileHandler(adminOnly bool, action teamFileAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		files, err := openTeamFiles(c, adminOnly)
		if err != nil {
			c.JSON(http.StatusOK, serializer.Err(c, err))
			return
		}
		defer files.manager.Recycle()

		data, err := action(files, c)
		if err != nil {
			c.JSON(http.StatusOK, serializer.Err(c, err))
			return
		}
		if !c.Writer.Written() {
			c.JSON(http.StatusOK, serializer.Response{Data: data})
		}
	}
}

func openTeamFiles(c *gin.Context, adminOnly bool) (*teamFiles, error) {
	id, err := positiveTeamFileID(c.Param("id"))
	if err != nil {
		return nil, err
	}
	dep := dependency.FromContext(c)
	actor := inventory.UserFromContext(c)
	workspace, isAdmin, err := team.Access(c, dep.DBClient(), actor.ID, id)
	if err != nil {
		return nil, err
	}
	if adminOnly && !isAdmin {
		return nil, serializer.NewError(serializer.CodeNoPermissionErr, "需要团队管理员权限", nil)
	}
	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	owner, err := dep.UserClient().GetByID(ctx, workspace.StorageOwnerID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "无法加载团队存储", err)
	}
	if !owner.TeamStorage || owner.Status != user.StatusSysBanned || owner.Edges.Group == nil {
		return nil, serializer.NewError(serializer.CodeInternalSetting, "团队存储账号配置无效", nil)
	}
	return &teamFiles{
		ctx:      dbfs.WithTeamStorageOwner(c, owner.ID),
		manager:  manager.NewFileManager(dep, owner),
		ownerKey: hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID),
		pageSize: dep.SettingProvider().DBFS(c).MaxPageSize,
	}, nil
}

func positiveTeamFileID(raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, serializer.NewError(serializer.CodeParamErr, "无效的编号", nil)
	}
	return id, nil
}

func (s *teamFiles) fileURI(raw string, allowRoot bool) (*fs.URI, error) {
	if !utf8.ValidString(raw) || !strings.HasPrefix(raw, "/") || strings.ContainsAny(raw, "\\\x00") ||
		path.Clean(raw) != raw || (!allowRoot && raw == "/") {
		return nil, serializer.NewError(serializer.CodeParamErr, "无效的团队文件路径", nil)
	}
	return &fs.URI{U: &url.URL{
		Scheme: constants.CloudreveScheme,
		User:   url.User(s.ownerKey),
		Host:   string(constants.FileSystemMy),
		Path:   raw,
	}}, nil
}

func teamFileDetails(file fs.File, filePath string) teamFileInfo {
	return teamFileInfo{
		Name: file.DisplayName(), Path: filePath,
		Directory: file.Type() == types.FileTypeFolder,
		Size:      file.Size(), Modified: file.UpdatedAt(),
	}
}

func (s *teamFiles) visitChildren(uri *fs.URI, visit func(fs.File) bool) error {
	args := &manager.ListArgs{Page: 1, PageSize: s.pageSize, Order: "name", OrderDirection: "asc"}
	for {
		parent, result, err := s.manager.List(s.ctx, uri, args)
		if err != nil {
			return err
		}
		if uri.FileSystem() == constants.FileSystemMy && parent.Type() != types.FileTypeFolder {
			return serializer.NewError(serializer.CodeParamErr, "请选择团队文件夹", nil)
		}
		for _, file := range result.Files {
			if !visit(file) {
				return nil
			}
		}
		pagination := result.Pagination
		if pagination == nil || len(result.Files) == 0 {
			return nil
		}
		if pagination.IsCursor {
			if pagination.NextPageToken == "" {
				return nil
			}
			args.PageToken = pagination.NextPageToken
		} else {
			if args.Page*pagination.PageSize >= pagination.TotalItems {
				return nil
			}
			args.Page++
		}
	}
}

func (s *teamFiles) list(c *gin.Context) (any, error) {
	uri, err := s.fileURI(c.DefaultQuery("path", "/"), true)
	if err != nil {
		return nil, err
	}
	files := make([]teamFileInfo, 0)
	err = s.visitChildren(uri, func(file fs.File) bool {
		files = append(files, teamFileDetails(file, path.Join(uri.Path(), file.Name())))
		return true
	})
	return files, err
}

func (s *teamFiles) createFolder(c *gin.Context) (any, error) {
	var params struct {
		Path      string `json:"path"`
		Directory bool   `json:"directory"`
	}
	if err := c.ShouldBindJSON(&params); err != nil || !params.Directory {
		return nil, serializer.NewError(serializer.CodeParamErr, "请输入要创建的文件夹路径", nil)
	}
	uri, err := s.fileURI(params.Path, false)
	if err != nil {
		return nil, err
	}
	_, err = s.manager.Create(s.ctx, uri, types.FileTypeFolder, dbfs.WithErrorOnConflict())
	return nil, err
}

func (s *teamFiles) upload(c *gin.Context) (any, error) {
	uri, err := s.fileURI(c.Query("path"), false)
	if err != nil {
		return nil, err
	}
	if c.Request.ContentLength < 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "上传时需要提供文件大小", nil)
	}
	_, err = s.manager.Update(s.ctx, &fs.UploadRequest{
		Props: &fs.UploadProps{Uri: uri, Size: c.Request.ContentLength},
		File:  c.Request.Body,
		Mode:  fs.ModeOverwrite,
	})
	return nil, err
}

func (s *teamFiles) move(c *gin.Context) (any, error) {
	var params struct {
		Path        string `json:"path"`
		Destination string `json:"destination"`
	}
	if err := c.ShouldBindJSON(&params); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "请输入原路径和目标路径", nil)
	}
	source, err := s.fileURI(params.Path, false)
	if err != nil {
		return nil, err
	}
	destination, err := s.fileURI(params.Destination, false)
	if err != nil {
		return nil, err
	}
	if source.Path() == destination.Path() {
		return nil, nil
	}
	if source.Dir() == destination.Dir() {
		_, err = s.manager.Rename(s.ctx, source, destination.Name())
		return nil, err
	}
	if source.Name() != destination.Name() {
		return nil, serializer.NewError(serializer.CodeParamErr, "请分开移动和重命名文件", nil)
	}
	return nil, s.manager.MoveOrCopy(s.ctx, []*fs.URI{source}, destination.DirUri(), false)
}

func (s *teamFiles) softDelete(c *gin.Context) (any, error) {
	uri, err := s.fileURI(c.Query("path"), false)
	if err != nil {
		return nil, err
	}
	return nil, s.manager.Delete(s.ctx, []*fs.URI{uri})
}

func (s *teamFiles) versionedFile(raw string) (fs.File, error) {
	uri, err := s.fileURI(raw, false)
	if err != nil {
		return nil, err
	}
	file, err := s.manager.Get(s.ctx, uri, dbfs.WithFileEntities())
	if err != nil {
		return nil, err
	}
	if file.Type() != types.FileTypeFile {
		return nil, serializer.NewError(serializer.CodeParamErr, "请选择团队文件", nil)
	}
	return file, nil
}

func (s *teamFiles) versions(c *gin.Context) (any, error) {
	file, err := s.versionedFile(c.Query("path"))
	if err != nil {
		return nil, err
	}
	versions := make([]teamFileVersion, 0)
	for _, entity := range file.Entities() {
		if entity.Type() == types.EntityTypeVersion && entity.UploadSessionID() == nil {
			versions = append(versions, teamFileVersion{
				ID: entity.ID(), Size: entity.Size(), Created: entity.CreatedAt(),
				Current: entity.ID() == file.PrimaryEntityID(),
			})
		}
	}
	return versions, nil
}

func (s *teamFiles) restoreVersion(c *gin.Context) (any, error) {
	var params struct {
		Path    string `json:"path"`
		Version int    `json:"version"`
	}
	if err := c.ShouldBindJSON(&params); err != nil || params.Version <= 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "请输入要恢复的文件版本", nil)
	}
	uri, err := s.fileURI(params.Path, false)
	if err != nil {
		return nil, err
	}
	return nil, s.manager.SetCurrentVersion(s.ctx, uri, params.Version)
}

func (s *teamFiles) download(c *gin.Context) (any, error) {
	file, err := s.versionedFile(c.Query("path"))
	if err != nil {
		return nil, err
	}
	version := file.PrimaryEntityID()
	if raw := c.Query("version"); raw != "" {
		version, err = positiveTeamFileID(raw)
		if err != nil {
			return nil, err
		}
	}
	for _, entity := range file.Entities() {
		if entity.ID() != version || entity.Type() != types.EntityTypeVersion || entity.UploadSessionID() != nil {
			continue
		}
		source, err := s.manager.GetEntitySource(s.ctx, version, fs.WithEntity(entity))
		if err != nil {
			return nil, err
		}
		defer source.Close()
		c.Header("X-Content-Type-Options", "nosniff")
		source.Serve(c.Writer, c.Request, entitysource.WithDownload(true), entitysource.WithDisplayName(file.DisplayName()))
		return nil, nil
	}
	return nil, fs.ErrEntityNotExist
}

func (s *teamFiles) listTrash(_ *gin.Context) (any, error) {
	entries := make([]teamTrashEntry, 0)
	err := s.visitChildren(teamTrashRoot(), func(file fs.File) bool {
		original, err := fs.NewUriFromString(file.Metadata()[dbfs.MetadataRestoreUri])
		if err == nil {
			entries = append(entries, teamTrashEntry{teamFileInfo: teamFileDetails(file, original.Path()), ID: file.ID()})
		}
		return true
	})
	return entries, err
}

func teamTrashRoot() *fs.URI {
	return &fs.URI{U: &url.URL{Scheme: constants.CloudreveScheme, Host: string(constants.FileSystemTrash)}}
}

func (s *teamFiles) findTrashEntry(raw string) (*fs.URI, error) {
	id, err := positiveTeamFileID(raw)
	if err != nil {
		return nil, err
	}
	var uri *fs.URI
	err = s.visitChildren(teamTrashRoot(), func(file fs.File) bool {
		if file.ID() == id {
			uri = file.Uri(false)
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if uri == nil {
		return nil, fs.ErrPathNotExist
	}
	return uri, nil
}

func (s *teamFiles) restoreTrash(c *gin.Context) (any, error) {
	uri, err := s.findTrashEntry(c.Param("entryId"))
	if err != nil {
		return nil, err
	}
	return nil, s.manager.Restore(s.ctx, uri)
}

func (s *teamFiles) purgeTrash(c *gin.Context) (any, error) {
	uri, err := s.findTrashEntry(c.Param("entryId"))
	if err != nil {
		return nil, err
	}
	return nil, s.manager.Delete(s.ctx, []*fs.URI{uri}, fs.WithSkipSoftDelete(true))
}
