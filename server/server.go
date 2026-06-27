package server

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gkmz/mymedia/tools/wechat-preview/handlers"
)

// Server Web 服务器
type Server struct {
	router  *gin.Engine
	handler *handlers.Handler
	embedFS embed.FS
}

// NewServer 创建服务器
func NewServer(handler *handlers.Handler, embedFS embed.FS) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	return &Server{
		router:  r,
		handler: handler,
		embedFS: embedFS,
	}
}

// Setup 设置路由和静态资源
func (s *Server) Setup(postsDir string) error {
	// 静态资源
	staticFS, _ := fs.Sub(s.embedFS, "web/static")
	s.router.StaticFS("/assets", http.FS(staticFS))

	// 挂载项目根目录
	if s.handler.ProjectRoot != "" {
		s.router.StaticFS("/_local_fs", gin.Dir(s.handler.ProjectRoot, false))
		// 单独挂载项目 assets，避免与前端静态资源路由 /assets 冲突。
		s.router.Static("/_project_assets", filepath.Join(s.handler.ProjectRoot, "assets"))
	}

	// 映射 posts 目录
	s.router.Static("/posts-static", postsDir)

	// 模板
	templatesFS, _ := fs.Sub(s.embedFS, "web/templates")
	s.router.SetHTMLTemplate(loadTemplates(templatesFS))

	// 路由
	s.setupRoutes()

	return nil
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 页面路由
	s.router.GET("/", s.handler.HandleList)
	s.router.GET("/article/:id", s.handler.HandleArticle)

	// API 路由
	s.router.GET("/api/articles", s.handler.APIArticles)
	s.router.GET("/api/articles/:id", s.handler.APIArticleDetail)
	s.router.POST("/api/publish/:id", s.handler.HandlePublish)

	// 平台和状态管理
	s.router.GET("/api/platforms", s.handler.HandleGetPlatforms)
	s.router.GET("/api/status", s.handler.HandleGetAllStatus)
	s.router.GET("/api/status/:articleID", s.handler.HandleGetStatus)
	s.router.POST("/api/status/:articleID/:platformID", s.handler.HandleMarkPublished)
	s.router.DELETE("/api/status/:articleID/:platformID", s.handler.HandleUnmarkPublished)

	// 静态资源
	s.router.GET("/chroma.css", s.handler.HandleChromaCSS)
}

// Run 启动服务器
func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

// loadTemplates 从 embed.FS 加载模板
func loadTemplates(fs fs.FS) *template.Template {
	tmpl, err := template.ParseFS(fs, "*.html")
	if err != nil {
		panic(err)
	}
	return tmpl
}
