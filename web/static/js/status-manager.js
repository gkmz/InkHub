// 状态管理器
class StatusManager {
  constructor() {
    this.platforms = [];
    this.statusData = {};
  }

  // 加载平台配置
  async loadPlatforms() {
    try {
      const response = await fetch('/api/platforms');
      const data = await response.json();
      this.platforms = data.platforms || [];
      return this.platforms;
    } catch (error) {
      console.error('加载平台配置失败:', error);
      return [];
    }
  }

  // 加载所有文章状态
  async loadAllStatus() {
    try {
      const response = await fetch('/api/status');
      const data = await response.json();
      this.statusData = data.articles || {};
      return this.statusData;
    } catch (error) {
      console.error('加载状态数据失败:', error);
      return {};
    }
  }

  // 加载单篇文章状态
  async loadStatus(articleID) {
    try {
      const response = await fetch(`/api/status/${articleID}`);
      const data = await response.json();
      this.statusData[articleID] = data;
      return data;
    } catch (error) {
      console.error('加载文章状态失败:', error);
      return { platforms: {} };
    }
  }

  // 标记为已发布
  async markPublished(articleID, platformID, url = '') {
    try {
      const response = await fetch(`/api/status/${articleID}/${platformID}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ url }),
      });
      const data = await response.json();
      if (data.success) {
        this.statusData[articleID] = data.status;
      }
      return data;
    } catch (error) {
      console.error('标记发布失败:', error);
      return { success: false, error: error.message };
    }
  }

  // 取消发布标记
  async unmarkPublished(articleID, platformID) {
    try {
      const response = await fetch(`/api/status/${articleID}/${platformID}`, {
        method: 'DELETE',
      });
      const data = await response.json();
      if (data.success) {
        this.statusData[articleID] = data.status;
      }
      return data;
    } catch (error) {
      console.error('取消发布标记失败:', error);
      return { success: false, error: error.message };
    }
  }

  // 获取文章状态
  getArticleStatus(articleID) {
    return this.statusData[articleID] || { platforms: {} };
  }

  // 检查文章是否已发布到某平台
  isPublished(articleID, platformID) {
    const status = this.getArticleStatus(articleID);
    return status.platforms && status.platforms[platformID]?.published;
  }

  // 获取文章已发布的平台列表
  getPublishedPlatforms(articleID) {
    const status = this.getArticleStatus(articleID);
    const published = [];
    if (status.platforms) {
      for (const [platformID, info] of Object.entries(status.platforms)) {
        if (info.published) {
          const platform = this.platforms.find(p => p.id === platformID);
          if (platform) {
            published.push({ ...platform, ...info });
          }
        }
      }
    }
    return published;
  }

  // 渲染状态徽章（用于列表页）
  renderStatusBadges(articleID, container) {
    const published = this.getPublishedPlatforms(articleID);

    if (published.length === 0) {
      container.innerHTML = '';
      return;
    }

    const html = published.map(p => {
      const time = new Date(p.publishedAt).toLocaleDateString('zh-CN');
      return `
        <span class="platform-badge" 
              style="background-color: ${p.color}20; color: ${p.color};"
              title="${p.name} - ${time}">
          ${p.icon} ${p.name}
        </span>
      `;
    }).join('');

    container.innerHTML = html;
  }

  // 渲染状态管理面板（用于详情页）
  renderStatusPanel(articleID, container) {
    const status = this.getArticleStatus(articleID);

    const html = `
      <div class="status-panel">
        <h3>📤 发布状态</h3>
        ${this.platforms.map(platform => {
      const info = status.platforms?.[platform.id];
      const isPublished = info?.published;

      return `
            <div class="platform-status">
              <div class="platform-info">
                <span class="platform-icon">${platform.icon}</span>
                <span class="platform-name">${platform.name}</span>
              </div>
              <div class="platform-actions">
                ${isPublished ? `
                  <span class="publish-time">
                    ${new Date(info.publishedAt).toLocaleString('zh-CN')}
                  </span>
                  <span class="status-badge published">✓ 已发布</span>
                  <button class="btn-mark unpublish" 
                          data-article="${articleID}" 
                          data-platform="${platform.id}">
                    取消标记
                  </button>
                ` : `
                  <span class="status-badge unpublished">未发布</span>
                  <button class="btn-mark publish" 
                          data-article="${articleID}" 
                          data-platform="${platform.id}">
                    标记为已发布
                  </button>
                `}
              </div>
            </div>
          `;
    }).join('')}
      </div>
    `;

    container.innerHTML = html;

    // 绑定事件
    container.querySelectorAll('.btn-mark').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        const articleID = e.target.dataset.article;
        const platformID = e.target.dataset.platform;
        const isPublish = e.target.classList.contains('publish');

        if (isPublish) {
          const url = prompt('请输入文章链接（可选）：');
          await this.markPublished(articleID, platformID, url || '');
        } else {
          if (confirm('确定要取消发布标记吗？')) {
            await this.unmarkPublished(articleID, platformID);
          } else {
            return;
          }
        }

        // 重新渲染
        this.renderStatusPanel(articleID, container);
      });
    });
  }
}

// 导出全局实例
window.statusManager = new StatusManager();
