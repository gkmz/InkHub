# 项目重构说明

## 重构概述

将原本集中在 `main.go` 中的所有代码进行了模块化重构，提高了代码的可维护性和可测试性。

## 新的项目结构

```
markdown-preview/
├── main.go                 # 入口文件（简化版）
├── config/                 # 配置管理
│   └── config.go
├── models/                 # 数据模型
│   ├── article.go         # 文章模型
│   ├── platform.go        # 平台模型
│   └── status.go          # 状态模型
├── scanner/               # 文章扫描
│   ├── scanner.go         # 扫描器实现
│   ├── filetime.go        # 文件时间获取
│   ├── filetime_darwin.go # macOS 实现
│   ├── filetime_linux.go  # Linux 实现
│   └── filetime_windows.go# Windows 实现
├── markdown/              # Markdown 处理
│   └── processor.go       # Markdown 处理器
├── handlers/              # HTTP 处理器
│   ├── handlers.go        # 处理器基础
│   ├── article.go         # 文章相关处理
│   ├── publish.go         # 发布处理
│   ├── platform.go        # 平台管理
│   └── static.go          # 静态资源
├── services/              # 业务服务
│   ├── platform.go        # 平台服务
│   ├── status.go          # 状态服务
│   ├── publisher.go       # 发布服务
│   └── uploader.go        # 上传服务
├── server/                # Web 服务器
│   └── server.go          # 服务器配置和路由
├── utils/                 # 工具函数
│   └── path.go            # 路径工具
└── web/                   # 前端资源
    ├── static/
    └── templates/
```

## 模块职责

### 1. config
- 配置文件加载和管理
- 环境变量处理

### 2. models
- 定义数据结构
- 文章、平台、状态等模型

### 3. scanner
- 扫描文章目录
- 提取文章元数据
- 跨平台文件时间获取

### 4. markdown
- Markdown 解析和渲染
- Frontmatter 处理
- Hugo shortcode 替换
- 图片路径处理
- 列表项优化

### 5. handlers
- HTTP 请求处理
- 路由处理函数
- 响应生成

### 6. services
- 业务逻辑实现
- 平台管理
- 发布状态管理
- 图片上传

### 7. server
- Web 服务器初始化
- 路由配置
- 静态资源管理

### 8. utils
- 通用工具函数
- 项目根目录查找

## 重构优势

1. **模块化**: 每个模块职责单一，易于理解和维护
2. **可测试性**: 各模块可独立测试
3. **可扩展性**: 新功能可以轻松添加到对应模块
4. **代码复用**: 通用功能提取为独立模块
5. **清晰的依赖关系**: 模块间依赖关系明确

## 使用方式

编译和运行方式与之前完全相同：

```bash
# 编译
go build -o markdown-preview

# 运行
./markdown-preview -dir /path/to/posts -port 8080
```

## 后续优化建议

1. 为各模块添加单元测试
2. 添加日志模块统一日志输出
3. 考虑使用依赖注入框架
4. 添加配置验证
5. 实现优雅关闭
