# Design Document

## 系统架构

### 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Frontend (Web)                        │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ List Page    │  │ Article Page │  │ Status Panel │      │
│  │ - Tree Nav   │  │ - Content    │  │ - Platforms  │      │
│  │ - Filters    │  │ - Publish    │  │ - Actions    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                            ↕ HTTP/JSON
┌─────────────────────────────────────────────────────────────┐
│                      Backend (Go/Gin)                        │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Article      │  │ Platform     │  │ Status       │      │
│  │ Service      │  │ Service      │  │ Service      │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                            ↕
┌─────────────────────────────────────────────────────────────┐
│                      Data Layer                              │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐                         │
│  │ platforms.   │  │ publish-     │                         │
│  │ json         │  │ status.json  │                         │
│  └──────────────┘  └──────────────┘                         │
└─────────────────────────────────────────────────────────────┘
```

## 数据模型

### Platform 配置 (platforms.json)

```json
{
  "platforms": [
    {
      "id": "wechat",
      "name": "微信公众号",
      "icon": "wechat",
      "color": "#07C160"
    },
    {
      "id": "zhihu",
      "name": "知乎",
      "icon": "zhihu",
      "color": "#0084FF"
    },
    {
      "id": "juejin",
      "name": "掘金",
      "icon": "juejin",
      "color": "#1E80FF"
    }
  ]
}
```

### Publish Status (publish-status.json)

```json
{
  "articles": {
    "golang_20220802-GoLang教程——变量": {
      "wechat": {
        "published": true,
        "publishedAt": "2024-02-20T10:30:00Z",
        "url": "https://mp.weixin.qq.com/s/xxx"
      },
      "zhihu": {
        "published": true,
        "publishedAt": "2024-02-20T11:00:00Z",
        "url": "https://zhuanlan.zhihu.com/p/xxx"
      }
    }
  }
}
```

## 后端设计

### 新增 Go 结构体

```go
// models/platform.go
type Platform struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Icon  string `json:"icon"`
    Color string `json:"color"`
}

type PlatformConfig struct {
    Platforms []Platform `json:"platforms"`
}

// models/status.go
type PublishInfo struct {
    Published   bool      `json:"published"`
    PublishedAt time.Time `json:"publishedAt"`
    URL         string    `json:"url,omitempty"`
}

type ArticleStatus struct {
    Platforms map[string]*PublishInfo `json:"platforms"` // key: platform ID
}

type PublishStatusData struct {
    Articles map[string]*ArticleStatus `json:"articles"` // key: article ID
}
```

### 新增 Service

```go
// services/platform.go
type PlatformService struct {
    configPath string
    platforms  []Platform
}

func (s *PlatformService) Load() error
func (s *PlatformService) GetAll() []Platform
func (s *PlatformService) GetByID(id string) *Platform

// services/status.go
type StatusService struct {
    dataPath string
    data     *PublishStatusData
}

func (s *StatusService) Load() error
func (s *StatusService) Save() error
func (s *StatusService) GetArticleStatus(articleID string) *ArticleStatus
func (s *StatusService) MarkPublished(articleID, platformID string, url string) error
func (s *StatusService) UnmarkPublished(articleID, platformID string) error
```

### 新增 API 路由

```go
// main.go
r.GET("/api/platforms", handleGetPlatforms)
r.GET("/api/status/:articleID", handleGetStatus)
r.POST("/api/status/:articleID/:platformID", handleMarkPublished)
r.DELETE("/api/status/:articleID/:platformID", handleUnmarkPublished)
r.GET("/api/articles/filter", handleFilterArticles)
```

## 前端设计

### 目录树组件 (category-tree.js)

```javascript
class CategoryTree {
  constructor(articles) {
    this.articles = articles;
    this.tree = this.buildTree();
    this.expandedNodes = new Set();
  }
  
  buildTree() {
    // 根据 article.Series 构建树形结构
    // 返回: { name, children: [], articles: [] }
  }
  
  render(container) {
    // 渲染树形导航
  }
  
  toggleNode(nodeId) {
    // 展开/折叠节点
  }
  
  saveState() {
    // 保存到 localStorage
  }
  
  loadState() {
    // 从 localStorage 恢复
  }
}
```

### 状态管理组件 (status-manager.js)

```javascript
class StatusManager {
  constructor() {
    this.platforms = [];
    this.statusData = {};
  }
  
  async loadPlatforms() {
    // GET /api/platforms
  }
  
  async loadStatus(articleID) {
    // GET /api/status/:articleID
  }
  
  async markPublished(articleID, platformID, url) {
    // POST /api/status/:articleID/:platformID
  }
  
  async unmarkPublished(articleID, platformID) {
    // DELETE /api/status/:articleID/:platformID
  }
  
  renderStatusBadges(articleID, container) {
    // 渲染平台标识徽章
  }
  
  renderStatusPanel(articleID, container) {
    // 渲染详情页的状态管理面板
  }
}
```

### 筛选组件 (filter.js)

```javascript
class ArticleFilter {
  constructor(articles, statusManager) {
    this.articles = articles;
    this.statusManager = statusManager;
    this.currentFilter = 'all';
  }
  
  render(container) {
    // 渲染筛选按钮组
  }
  
  applyFilter(filterType, platformID) {
    // 'all' | 'unpublished' | 'platform:xxx'
  }
  
  getFilteredArticles() {
    // 返回筛选后的文章列表
  }
}
```

### 页面更新

#### list.html 更新

```html
<div class="container">
  <!-- 筛选栏 -->
  <div id="filter-bar" class="filter-bar">
    <button class="filter-btn active" data-filter="all">全部</button>
    <button class="filter-btn" data-filter="unpublished">未发布</button>
    <!-- 动态生成平台筛选按钮 -->
  </div>
  
  <!-- 左侧：目录树 -->
  <div class="sidebar">
    <div id="category-tree"></div>
  </div>
  
  <!-- 右侧：文章列表 -->
  <div class="main-content">
    <div id="article-list">
      <!-- 文章卡片，包含状态徽章 -->
    </div>
  </div>
</div>
```

#### article.html 更新

```html
<div class="article-header">
  <h1>{{ .title }}</h1>
  
  <!-- 发布状态面板 -->
  <div id="publish-status-panel" class="status-panel">
    <!-- 动态生成平台状态 -->
  </div>
</div>
```

## 实现步骤

### Phase 1: 后端基础

1. 创建 models 包，定义数据结构
2. 实现 PlatformService，支持加载和读取平台配置
3. 实现 StatusService，支持 CRUD 操作
4. 创建默认 platforms.json 配置文件
5. 添加 API 路由和 handler

### Phase 2: 前端基础

1. 实现 StatusManager 类
2. 在 article.html 中集成状态管理面板
3. 实现标记/取消发布功能
4. 添加状态徽章显示

### Phase 3: 目录树导航

1. 实现 CategoryTree 类
2. 重构 list.html，添加树形导航
3. 实现展开/折叠功能
4. 添加状态持久化（localStorage）

### Phase 4: 筛选功能

1. 实现 ArticleFilter 类
2. 添加筛选按钮组
3. 实现筛选逻辑
4. 集成到列表页

### Phase 5: 优化和测试

1. 添加加载状态和错误处理
2. 优化 UI/UX
3. 测试各种边界情况
4. 性能优化

## 文件结构

```
markdown-preview/
├── config/
│   ├── platforms.json          # 新增：平台配置
│   └── publish-status.json     # 新增：发布状态数据
├── models/                     # 新增：数据模型
│   ├── platform.go
│   └── status.go
├── services/
│   ├── platform.go             # 新增：平台服务
│   ├── status.go               # 新增：状态服务
│   ├── publisher.go
│   └── uploader.go
├── web/
│   ├── static/
│   │   ├── css/
│   │   │   ├── status.css      # 新增：状态样式
│   │   │   └── tree.css        # 新增：树形导航样式
│   │   └── js/
│   │       ├── category-tree.js # 新增：目录树组件
│   │       ├── status-manager.js # 新增：状态管理
│   │       └── filter.js       # 新增：筛选组件
│   └── templates/
│       ├── list.html           # 更新
│       └── article.html        # 更新
└── main.go                     # 更新：添加新路由
```

## 技术选型

- 后端：Go + Gin（已有）
- 前端：原生 JavaScript（保持轻量）
- 数据存储：JSON 文件（简单、易备份）
- 状态管理：localStorage（前端状态持久化）
- 图标：使用 Unicode emoji 或简单的 SVG

## 安全考虑

1. API 访问控制：当前为本地工具，暂不需要认证
2. 文件路径验证：防止路径遍历攻击
3. JSON 解析：添加错误处理，防止格式错误导致崩溃
4. 输入验证：验证 platformID 和 articleID 的合法性

## 性能考虑

1. 懒加载：目录树节点按需展开
2. 缓存：前端缓存平台配置和状态数据
3. 批量操作：支持批量标记发布状态（未来扩展）
4. 索引：为常用查询添加内存索引

## 未来扩展

1. 批量操作：批量标记多篇文章的发布状态
2. 统计面板：显示各平台的发布统计
3. 同步功能：从平台 API 自动同步发布状态
4. 导出功能：导出发布报告
5. 提醒功能：提醒未发布的文章
