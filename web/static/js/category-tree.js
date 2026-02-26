// 目录树组件
class CategoryTree {
  constructor(articles) {
    this.articles = articles;
    this.tree = this.buildTree();
    this.expandedNodes = this.loadState();
    this.selectedCategory = null;
  }

  // 构建树形结构
  buildTree() {
    const tree = {};

    this.articles.forEach(article => {
      const series = article.series || '其他';
      if (!tree[series]) {
        tree[series] = {
          name: series,
          articles: [],
          count: 0
        };
      }
      tree[series].articles.push(article);
      tree[series].count++;
    });

    return tree;
  }

  // 渲染树
  render(container) {
    const categories = Object.keys(this.tree).sort();

    const html = `
      <div class="category-tree">
        <div class="tree-node">
          <div class="tree-node-header ${!this.selectedCategory ? 'active' : ''}" 
               data-category="all">
            <span class="tree-icon">📚</span>
            <span class="tree-label">全部文章</span>
            <span class="tree-count">${this.articles.length}</span>
          </div>
        </div>
        ${categories.map(category => {
      const node = this.tree[category];
      const isExpanded = this.expandedNodes.has(category);
      const isSelected = this.selectedCategory === category;

      return `
            <div class="tree-node">
              <div class="tree-node-header ${isSelected ? 'active' : ''}" 
                   data-category="${category}">
                <span class="tree-toggle ${isExpanded ? 'expanded' : 'collapsed'}" 
                      data-toggle="${category}"></span>
                <span class="tree-icon">📁</span>
                <span class="tree-label">${category}</span>
                <span class="tree-count">${node.count}</span>
              </div>
              <div class="tree-children ${isExpanded ? 'expanded' : ''}" 
                   data-children="${category}">
                ${node.articles.map(article => `
                  <div class="tree-node-header" data-article="${article.id}">
                    <span class="tree-icon">📄</span>
                    <span class="tree-label">${article.title}</span>
                  </div>
                `).join('')}
              </div>
            </div>
          `;
    }).join('')}
      </div>
    `;

    container.innerHTML = html;

    // 绑定事件
    this.bindEvents(container);
  }

  // 绑定事件
  bindEvents(container) {
    // 展开/折叠
    container.querySelectorAll('[data-toggle]').forEach(toggle => {
      toggle.addEventListener('click', (e) => {
        e.stopPropagation();
        const category = e.target.dataset.toggle;
        this.toggleNode(category);
        this.render(container.parentElement);
      });
    });

    // 选择分类
    container.querySelectorAll('[data-category]').forEach(header => {
      header.addEventListener('click', (e) => {
        const category = e.currentTarget.dataset.category;
        this.selectCategory(category);
        this.render(container.parentElement);

        // 触发自定义事件
        window.dispatchEvent(new CustomEvent('categorySelected', {
          detail: { category }
        }));
      });
    });

    // 选择文章
    container.querySelectorAll('[data-article]').forEach(header => {
      header.addEventListener('click', (e) => {
        const articleID = e.currentTarget.dataset.article;
        window.location.href = `/article/${articleID}`;
      });
    });
  }

  // 展开/折叠节点
  toggleNode(category) {
    if (this.expandedNodes.has(category)) {
      this.expandedNodes.delete(category);
    } else {
      this.expandedNodes.add(category);
    }
    this.saveState();
  }

  // 选择分类
  selectCategory(category) {
    this.selectedCategory = category === 'all' ? null : category;
  }

  // 获取当前分类的文章
  getFilteredArticles() {
    if (!this.selectedCategory) {
      return this.articles;
    }
    return this.tree[this.selectedCategory]?.articles || [];
  }

  // 保存状态到 localStorage
  saveState() {
    localStorage.setItem('categoryTreeState', JSON.stringify([...this.expandedNodes]));
  }

  // 从 localStorage 加载状态
  loadState() {
    try {
      const saved = localStorage.getItem('categoryTreeState');
      return saved ? new Set(JSON.parse(saved)) : new Set();
    } catch {
      return new Set();
    }
  }
}

// 导出
window.CategoryTree = CategoryTree;
