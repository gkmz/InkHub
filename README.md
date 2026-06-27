# Markdown Preview Tool (WeChat Edition)

个人自用的 专为微信公众号设计的 Markdown 预览与发布工具, 理论上支持各大平台。支持本地预览、一键格式化复制、自动图片上传等高级功能。

## ✨ 核心功能

### 1. 微信风格渲染
- **完美复刻**：默认样式专门针对微信公众号优化（字体、行高、段间距）
- **代码高亮**：集成 `Monokai` 主题，采用 **Inline Style** 技术，确保粘贴到微信后台颜色不丢失
- **脚注优化**：自动将 Markdown 链接转换为文末脚注，符合微信阅读习惯
- **智能格式化**：自动移除文章标题（H1），列表项样式优化

### 2. 图片自动化处理
- **本地预览**：直接解析本地 Markdown 图片路径（如 `./images/demo.png`），所见即所得
- **一键发布**：点击“发布/复制”按钮时：
  - 自动扫描文中图片
  - 自动将 Mermaid 代码块渲染为 SVG 图片（优先本地 `mmdc`，失败自动回退 `kroki.io`）
  - 自动上传至 GitHub CDN (需配置 token)
  - 自动替换为 CDN 链接
  - 自动生成最终 HTML 到剪贴板

### 3. 高效工作流
- **双重复制模式**：
  - **复制原文**：仅应用样式，保持本地图片路径（适合本地调试）
  - **发布/复制**：执行完整的发布流程（上传图片 -> 替换链接 -> 复制 HTML）
- **即时反馈**：右下角浮动通知，实时显示上传进度和结果

## 🚀 快速开始

### 1. 配置环境 (可选)

如果你需要图片上传功能，请参考下方的 [详细配置说明](#%EF%B8%8F-详细配置说明) 创建 `.env` 文件或设置环境变量。如果只是本地预览，可跳过此步。

### 2. 运行工具

本工具支持两种运行模式：

#### 方式 A：源码运行 (推荐开发者)

```bash
# 自动安装依赖
go mod tidy

# 默认运行（扫描当前目录）
go run main.go

# 指定扫描目录
go run main.go -dir ../../posts

# 指定监听地址和端口（默认只监听 127.0.0.1:8080）
go run main.go -host 127.0.0.1 -port 9090
```

#### 方式 B：单文件运行 (推荐发布/分发)

将工具编译为单个可执行文件，无需处理静态资源路径。

1.  **编译**：
    ```bash
    go build -o preview main.go
    ```

2.  **运行**：
    将生成的 `preview` (Windows下为 `preview.exe`) 文件放到任意 Markdown 目录中运行，或者通过命令行指定：

    ```bash
    # 进入你的文章目录，直接运行
    /path/to/preview
    
    # 或者指定目录
    /path/to/preview -dir /path/to/my/posts
    
    # 指定监听地址和端口
    /path/to/preview -host 127.0.0.1 -port 9090
    ```

访问 [http://127.0.0.1:8080](http://127.0.0.1:8080) 即可预览。

默认只监听本机地址，避免局域网设备直接访问你的本地文章和发布接口。如确实需要局域网访问，可以显式传入 `-host 0.0.0.0`，但请只在可信网络中使用。

## ⚙️ 详细配置说明

### 准备工作 (Prerequisites)

为了使用**一键发布/复制**功能，你需要准备：

1.  **一个公开的 GitHub 仓库** (Public Repo)
    -   用于存储文章中的图片。
    -   必须是 Public 的，因为微信公众号后台无法直接读取 Private 仓库的图片。
    -   示例：`yourname/assets`

2.  **一个 GitHub Personal Access Token** (PAT)
    -   权限要求：至少需要 `repo` 或 `public_repo` (针对公开仓库) 权限。
    -   [点击这里生成 Token](https://github.com/settings/tokens)

### 配置文件模板 (.env)

你可以创建 `.env` 文件来配置环境变量，避免每次通过 CLI 传递。

```ini
# 文章根目录 (可选，如果不通过 -dir 指定)
POSTS_DIR=../../posts

# GitHub 配置 (可选，仅用于图片上传)
GITHUB_TOKEN=your_github_token
GITHUB_OWNER=your_username
GITHUB_REPO=your_assets_repo
GITHUB_BRANCH=main
GITHUB_PATH_PREFIX=posts
```

### 环境变量详解

| 变量名 | 必填 | 说明 | 示例 |
| :--- | :--- | :--- | :--- |
| `POSTS_DIR` | ❌ | 本地 Markdown 文章目录 (建议通过 CLI `-dir` 参数指定) | `../../posts` |
| `GITHUB_TOKEN` | ✅ | 你的 GitHub Token (用于上传图片) | `ghp_xxxx` |
| `GITHUB_OWNER` | ✅ | GitHub 用户名 | `gkmz` |
| `GITHUB_REPO` | ✅ | 存放图片的仓库名 | `assets` |
| `GITHUB_BRANCH` | ❌ | 分支名，默认为 `main` | `main` |
| `GITHUB_PATH_PREFIX` | ❌ | **强烈推荐**。图片在仓库中的根目录前缀。<br>设置后，图片将上传到 `<prefix>/<relative-path-from-root>/...` | `posts` |

### 本地文件访问边界

工具只会暴露项目根目录下的文件和文章目录静态资源，不会映射系统根目录。文章引用项目根目录外的本地图片时，工具会尝试复制到 `assets/imported/` 后再预览；复制失败时会保留原路径，不会通过 Web 路由暴露任意系统文件。

---

## 🔬 技术实现原理

本工具通过以下步骤解决微信公众号编辑痛点：

### 1. 样式隔离与内联 (CSS Inlining)
微信公众号编辑器不支持外部 CSS，且对 `<style>` 标签支持有限。
- **解决方案**：后端渲染时，利用 `goldmark-highlighting` 采用 **Inline Styles** 模式，将 CSS 样式直接写入每个 HTML 元素的 `style` 属性中。
- **效果**：无论粘贴到哪里，颜色和样式都能完美保留。

### 2. 智能图片托管 (Auto Image Hosting)
本地写作时使用本地图片 `![demo](./images/demo.png)`，但这无法直接粘贴到网络编辑器。
- **解决方案**：点击发布时，后端自动扫描本地图片引用 -> 检查 GitHub 仓库是否存在同名文件 -> 不存在则调用 GitHub API 上传 -> 获取 `raw.githubusercontent.com` 的 CDN 链接 -> 替换 Markdown 内容。
- **结果**：剪贴板中的 HTML 包含的是可公开访问的网络图片链接。

### 3. 内容管线 (Content Pipeline)
1.  **Read**: 读取本地 Markdown 文件
2.  **Pre-process**: 移除 H1 标题 (避免重复)，优化列表样式
3.  **Upload & Replace**: 并发处理图片上传与链接替换
4.  **Render**: 使用 Goldmark 渲染为 HTML (带 Inline Styles)
5.  **Copy**: 前端通过 Selection API 复制格式化后的 HTML

## 🛠 开发与贡献

欢迎提交 PR 或 Issue！

### 项目结构

```
markdown-preview/
├── main.go              # 服务端核心逻辑 (Gin + Goldmark)
├── services/            # 业务逻辑 (图片上传、发布处理)
├── config/              # 配置加载
├── web/
│   ├── templates/       # HTML 模板
│   └── static/
│       ├── css/         # wechat.css (核心样式)
│       └── js/          # copy.js (前端交互)
```

### 如何贡献

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 提交 Pull Request

## ⚠️ 注意事项

- **图片上传**：依赖 GitHub API，请确保你的网络环境可以访问 GitHub；请求有超时限制，网络异常会返回错误提示。
- **Token 安全**：不要将 `.env` 文件提交到版本控制系统中。
- **本地安全**：默认只监听 `127.0.0.1`。如使用 `-host 0.0.0.0` 暴露到局域网，请确认网络可信。

## License

[Apache 2.0](LICENSE)
