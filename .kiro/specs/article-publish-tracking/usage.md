# 使用说明

## 功能概述

文章发布状态跟踪功能已经实现，包括以下特性：

1. ✅ 目录树形导航 - 按系列浏览文章
2. ✅ 发布状态管理 - 标记文章发布到不同平台
3. ✅ 状态筛选 - 按发布状态筛选文章
4. ✅ 平台配置 - 支持自定义发布平台
5. ✅ 数据持久化 - JSON 格式存储

## 启动服务

```bash
cd tools/markdown-preview
./markdown-preview -dir ../../content/posts -port 8080
```

或者使用环境变量：

```bash
export POSTS_DIR="../../content/posts"
./markdown-preview
```

## 使用流程

### 1. 浏览文章列表

访问 http://localhost:8080

页面包含三个主要区域：
- 左侧：目录树导航
- 顶部：筛选按钮
- 右侧：文章列表

### 2. 使用目录树

- 点击"全部文章"查看所有文章
- 点击分类名称查看该分类下的文章
- 点击分类前的箭头展开/折叠子文章列表
- 点击文章名称直接跳转到文章详情

### 3. 使用筛选功能

顶部筛选栏提供以下选项：
- 📚 全部 - 显示所有文章
- ⏳ 未发布 - 只显示未发布到任何平台的文章
- 💬 微信公众号 - 只显示已发布到微信的文章
- 🎓 知乎 - 只显示已发布到知乎的文章
- 💎 掘金 - 只显示已发布到掘金的文章
- 📝 CSDN - 只显示已发布到 CSDN 的文章

### 4. 标记发布状态

在文章详情页：
1. 查看"📤 发布状态"面板
2. 每个平台显示当前状态（已发布/未发布）
3. 点击"标记为已发布"按钮
4. 可选：输入文章链接
5. 点击"取消标记"可以取消发布状态

### 5. 查看发布状态

在文章列表页，每篇文章下方会显示已发布平台的徽章：
- 徽章显示平台图标和名称
- 鼠标悬停可查看发布时间
- 不同平台使用不同颜色区分

## 配置文件

### 平台配置 (config/platforms.json)

首次运行时会自动创建默认配置，包含：
- 微信公众号
- 知乎
- 掘金
- CSDN

可以手动编辑添加更多平台：

```json
{
  "platforms": [
    {
      "id": "custom",
      "name": "自定义平台",
      "icon": "🌐",
      "color": "#FF5722"
    }
  ]
}
```

### 发布状态数据 (config/publish-status.json)

自动生成和维护，格式如下：

```json
{
  "articles": {
    "golang_20220802-GoLang教程——变量": {
      "platforms": {
        "wechat": {
          "published": true,
          "publishedAt": "2024-02-20T10:30:00Z",
          "url": "https://mp.weixin.qq.com/s/xxx"
        }
      }
    }
  }
}
```

## API 接口

### 获取平台列表
```
GET /api/platforms
```

### 获取所有文章状态
```
GET /api/status
```

### 获取单篇文章状态
```
GET /api/status/:articleID
```

### 标记为已发布
```
POST /api/status/:articleID/:platformID
Content-Type: application/json

{
  "url": "https://example.com/article"
}
```

### 取消发布标记
```
DELETE /api/status/:articleID/:platformID
```

## 数据备份

发布状态数据存储在 `config/publish-status.json`，建议：

1. 定期备份该文件
2. 将其加入版本控制（如果需要团队共享）
3. 或者添加到 .gitignore（如果是个人使用）

## 故障排除

### 状态数据丢失

如果 `config/publish-status.json` 文件损坏或丢失：
1. 服务会自动创建新的空文件
2. 从备份恢复（如果有）
3. 重新标记文章发布状态

### 平台配置错误

如果 `config/platforms.json` 格式错误：
1. 删除该文件
2. 重启服务，会自动创建默认配置
3. 重新添加自定义平台

### 目录树状态丢失

目录树的展开/折叠状态存储在浏览器 localStorage 中：
- 清除浏览器缓存会丢失状态
- 不同浏览器的状态独立
- 可以手动清除：打开浏览器控制台，执行 `localStorage.removeItem('categoryTreeState')`

## 未来扩展

计划中的功能：
- [ ] 批量标记发布状态
- [ ] 发布统计面板
- [ ] 从平台 API 自动同步状态
- [ ] 导出发布报告
- [ ] 发布提醒功能
