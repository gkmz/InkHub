# Requirements Document

## Introduction

本需求文档描述了为 Markdown Preview 工具添加文章发布状态跟踪功能。该功能允许用户记录和管理文章在不同平台（微信公众号、知乎等）的发布状态，并通过目录树形结构优化文章列表的浏览体验。

## Glossary

- **System**: Markdown Preview 工具
- **Article**: 系统中的 Markdown 文章
- **Platform**: 发布平台（如微信公众号、知乎等）
- **Publish Status**: 文章在特定平台的发布状态
- **Category Tree**: 基于目录结构的文章分类树
- **Status File**: 存储发布状态的 JSON 文件

## Requirements

### Requirement 1

**User Story:** 作为一个内容创作者，我想要通过目录树形结构浏览文章，这样我可以快速定位到特定分类下的文章。

#### Acceptance Criteria

1. WHEN 用户访问文章列表页 THEN System SHALL 显示基于目录结构的树形导航
2. WHEN 用户点击某个目录节点 THEN System SHALL 展开或折叠该目录下的子目录和文章
3. WHEN 目录被展开 THEN System SHALL 显示该目录下的所有文章列表
4. WHEN 目录包含子目录 THEN System SHALL 在目录名称旁显示子目录数量和文章数量
5. WHEN 用户刷新页面 THEN System SHALL 保持用户上次的展开/折叠状态

### Requirement 2

**User Story:** 作为一个内容创作者，我想要配置支持的发布平台，这样我可以根据自己的需求管理不同的发布渠道。

#### Acceptance Criteria

1. WHEN System 启动时 THEN System SHALL 从配置文件读取支持的发布平台列表
2. WHEN 配置文件不存在 THEN System SHALL 创建默认配置文件包含微信公众号和知乎平台
3. WHEN 配置文件格式错误 THEN System SHALL 记录错误日志并使用默认配置
4. WHEN 平台配置包含名称和标识 THEN System SHALL 验证标识的唯一性
5. WHERE 平台配置包含图标信息 THEN System SHALL 在界面中显示对应图标

### Requirement 3

**User Story:** 作为一个内容创作者，我想要标记文章已发布到某个平台，这样我可以跟踪文章的发布状态。

#### Acceptance Criteria

1. WHEN 用户在文章详情页点击"标记为已发布"按钮 THEN System SHALL 记录该文章在指定平台的发布状态
2. WHEN 记录发布状态 THEN System SHALL 保存发布时间戳
3. WHEN 发布状态被记录 THEN System SHALL 将状态持久化到 JSON 文件
4. WHEN JSON 文件写入失败 THEN System SHALL 返回错误信息给用户
5. WHEN 文章已标记为已发布 THEN System SHALL 允许用户取消发布标记

### Requirement 4

**User Story:** 作为一个内容创作者，我想要在文章列表中看到发布状态，这样我可以快速识别哪些文章已经发布。

#### Acceptance Criteria

1. WHEN 文章列表加载时 THEN System SHALL 读取所有文章的发布状态
2. WHEN 文章已发布到某平台 THEN System SHALL 在文章卡片上显示对应平台的标识
3. WHEN 文章发布到多个平台 THEN System SHALL 显示所有已发布平台的标识
4. WHEN 用户悬停在平台标识上 THEN System SHALL 显示发布时间的详细信息
5. WHEN 文章未发布到任何平台 THEN System SHALL 不显示任何平台标识

### Requirement 5

**User Story:** 作为一个内容创作者，我想要在文章详情页管理发布状态，这样我可以方便地更新文章的发布信息。

#### Acceptance Criteria

1. WHEN 用户查看文章详情 THEN System SHALL 显示所有配置平台的发布状态
2. WHEN 平台未发布 THEN System SHALL 显示"标记为已发布"按钮
3. WHEN 平台已发布 THEN System SHALL 显示发布时间和"取消发布标记"按钮
4. WHEN 用户点击"标记为已发布"按钮 THEN System SHALL 更新发布状态并刷新界面
5. WHEN 用户点击"取消发布标记"按钮 THEN System SHALL 移除发布状态并刷新界面

### Requirement 6

**User Story:** 作为一个内容创作者，我想要发布状态数据以 JSON 格式存储，这样我可以方便地备份和迁移数据。

#### Acceptance Criteria

1. WHEN System 需要存储发布状态 THEN System SHALL 使用 JSON 格式保存数据
2. WHEN JSON 文件不存在 THEN System SHALL 创建新的 JSON 文件
3. WHEN 读取 JSON 文件 THEN System SHALL 验证文件格式的有效性
4. WHEN JSON 文件格式无效 THEN System SHALL 记录错误并返回空状态
5. WHERE 存储路径可配置 THEN System SHALL 从配置文件读取存储路径

### Requirement 7

**User Story:** 作为一个内容创作者，我想要通过 API 管理发布状态，这样我可以通过程序化方式批量更新状态。

#### Acceptance Criteria

1. WHEN 调用标记发布 API THEN System SHALL 接受文章 ID 和平台标识作为参数
2. WHEN 调用取消发布 API THEN System SHALL 接受文章 ID 和平台标识作为参数
3. WHEN 调用查询状态 API THEN System SHALL 返回指定文章的所有发布状态
4. WHEN API 调用成功 THEN System SHALL 返回 200 状态码和操作结果
5. WHEN API 调用失败 THEN System SHALL 返回适当的错误状态码和错误信息

### Requirement 8

**User Story:** 作为一个内容创作者，我想要在列表页按发布状态筛选文章，这样我可以快速找到未发布或已发布的文章。

#### Acceptance Criteria

1. WHEN 用户访问列表页 THEN System SHALL 显示发布状态筛选选项
2. WHEN 用户选择"未发布"筛选 THEN System SHALL 只显示未发布到任何平台的文章
3. WHEN 用户选择特定平台筛选 THEN System SHALL 只显示已发布到该平台的文章
4. WHEN 用户选择"全部"筛选 THEN System SHALL 显示所有文章
5. WHEN 筛选条件改变 THEN System SHALL 保持目录树的展开状态
