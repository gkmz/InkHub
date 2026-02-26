// 文章筛选器
class ArticleFilter {
  constructor(articles, statusManager) {
    this.articles = articles;
    this.statusManager = statusManager;
    this.currentFilter = { type: 'all', platformID: null };
  }

  // 渲染筛选栏
  async render(container) {
    await this.statusManager.loadPlatforms();

    const html = `
      <div class="filter-bar">
        <button class="filter-btn active" data-filter="all">
          📚 全部
        </button>
        <button class="filter-btn" data-filter="unpublished">
          ⏳ 未发布
        </button>
        ${this.statusManager.platforms.map(platform => `
          <button class="filter-btn" data-filter="platform" data-platform="${platform.id}">
            ${platform.icon} ${platform.name}
          </button>
        `).join('')}
      </div>
    `;

    container.innerHTML = html;

    // 绑定事件
    container.querySelectorAll('.filter-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const filterType = e.currentTarget.dataset.filter;
        const platformID = e.currentTarget.dataset.platform;

        this.applyFilter(filterType, platformID);

        // 更新按钮状态
        container.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
        e.currentTarget.classList.add('active');

        // 触发自定义事件
        window.dispatchEvent(new CustomEvent('filterChanged', {
          detail: { filterType, platformID }
        }));
      });
    });
  }

  // 应用筛选
  applyFilter(filterType, platformID = null) {
    this.currentFilter = { type: filterType, platformID };
  }

  // 获取筛选后的文章
  getFilteredArticles() {
    const { type, platformID } = this.currentFilter;

    if (type === 'all') {
      return this.articles;
    }

    if (type === 'unpublished') {
      return this.articles.filter(article => {
        const published = this.statusManager.getPublishedPlatforms(article.id);
        return published.length === 0;
      });
    }

    if (type === 'platform' && platformID) {
      return this.articles.filter(article => {
        return this.statusManager.isPublished(article.id, platformID);
      });
    }

    return this.articles;
  }

  // 更新文章列表显示
  updateArticleList() {
    const filtered = this.getFilteredArticles();
    const filteredIDs = new Set(filtered.map(a => a.id));

    // 显示/隐藏文章卡片
    document.querySelectorAll('.article-card').forEach(card => {
      const articleID = card.dataset.articleId;
      if (filteredIDs.has(articleID)) {
        card.classList.remove('hidden');
      } else {
        card.classList.add('hidden');
      }
    });
  }
}

// 导出
window.ArticleFilter = ArticleFilter;
