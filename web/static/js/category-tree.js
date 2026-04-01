// 目录树组件
class CategoryTree {
  constructor(articles, statusManager) {
    this.articles = articles;
    this.statusManager = statusManager;
    this.tree = this.buildTree();
    this.expandedNodes = this.loadState();
    this.selectedPath = null; // 当前选中的 folderPath
    this.isDetailPage = window.location.pathname.startsWith('/article/');
  }

  // 构建多层嵌套树
  // 每个节点: { name, children: {}, articles: [], count }
  buildTree() {
    const root = { name: 'root', children: {}, articles: [], count: 0 };

    this.articles.forEach(article => {
      const parts = article.folderPath ? article.folderPath.split('/') : [];
      let node = root;
      let pathSoFar = '';

      parts.forEach(part => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${part}` : part;
        if (!node.children[part]) {
          node.children[part] = { name: part, path: pathSoFar, children: {}, articles: [], count: 0 };
        }
        node = node.children[part];
      });

      node.articles.push(article);

      // 向上累加 count
      let countNode = root;
      countNode.count++;
      parts.forEach(part => {
        countNode = countNode.children[part];
        countNode.count++;
      });
    });

    return root;
  }

  // 获取文章的平台图标
  getPlatformIcons(articleID) {
    if (!this.statusManager) return '';
    const published = this.statusManager.getPublishedPlatforms(articleID);
    if (published.length === 0) return '';
    return published.map(p =>
      `<span class="tree-platform-icon" title="${p.name}">${p.icon}</span>`
    ).join('');
  }

  // 渲染树
  render(container) {
    const html = `
      <div class="category-tree">
        <div class="tree-node">
          <div class="tree-node-header ${!this.selectedPath ? 'active' : ''}"
               data-path="__all__">
            <span class="tree-icon">📚</span>
            <span class="tree-label">全部文章</span>
            <span class="tree-count">${this.tree.count}</span>
          </div>
        </div>
        ${this.renderChildren(this.tree, 0)}
      </div>
    `;
    container.innerHTML = html;
    this.bindEvents(container);
  }

  // 递归渲染子节点
  renderChildren(node, depth) {
    return Object.values(node.children)
      .sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
      .map(child => this.renderNode(child, depth))
      .join('');
  }

  renderNode(node, depth) {
    const isExpanded = this.expandedNodes.has(node.path);
    const isSelected = this.selectedPath === node.path;
    const hasChildren = Object.keys(node.children).length > 0;
    const indent = depth * 14;

    const articles = node.articles.map(article => {
      const platformIcons = this.getPlatformIcons(article.id);
      return `
        <div class="tree-node-header tree-article" data-article="${article.id}"
             style="padding-left: ${indent + 28}px">
          <span class="tree-icon">📄</span>
          <span class="tree-label">${article.title}</span>
          ${platformIcons ? `<span class="tree-platforms">${platformIcons}</span>` : ''}
        </div>
      `;
    }).join('');

    const children = hasChildren && isExpanded
      ? `<div class="tree-children expanded" data-children="${node.path}">
           ${this.renderChildren(node, depth + 1)}
         </div>`
      : hasChildren
        ? `<div class="tree-children" data-children="${node.path}">
             ${this.renderChildren(node, depth + 1)}
           </div>`
        : '';

    return `
      <div class="tree-node">
        <div class="tree-node-header ${isSelected ? 'active' : ''}"
             data-path="${node.path}"
             style="padding-left: ${indent + 4}px">
          ${hasChildren
            ? `<span class="tree-toggle ${isExpanded ? 'expanded' : 'collapsed'}"
                     data-toggle="${node.path}"></span>`
            : `<span class="tree-toggle-placeholder"></span>`
          }
          <span class="tree-icon">${hasChildren ? '📁' : '📂'}</span>
          <span class="tree-label">${node.name}</span>
          <span class="tree-count">${node.count}</span>
        </div>
        ${isExpanded || !hasChildren ? articles : ''}
        ${children}
      </div>
    `;
  }

  // 绑定事件
  bindEvents(container) {
    // 展开/折叠
    container.querySelectorAll('[data-toggle]').forEach(toggle => {
      toggle.addEventListener('click', (e) => {
        e.stopPropagation();
        this.toggleNode(e.target.dataset.toggle);
        const treeContainer = container.querySelector('.category-tree').parentElement;
        this.render(treeContainer);
      });
    });

    // 目录点击：选中该目录过滤文章
    container.querySelectorAll('[data-path]').forEach(header => {
      header.addEventListener('click', (e) => {
        if (e.target.dataset.toggle) return;
        const path = e.currentTarget.dataset.path;

        if (this.isDetailPage) {
          localStorage.setItem('selectedPath', path);
          window.location.href = '/';
          return;
        }

        this.selectedPath = path === '__all__' ? null : path;
        const treeContainer = container.querySelector('.category-tree').parentElement;
        this.render(treeContainer);
        window.dispatchEvent(new CustomEvent('categorySelected', { detail: { path } }));
      });
    });

    // 文章点击
    container.querySelectorAll('[data-article]').forEach(header => {
      header.addEventListener('click', (e) => {
        const articleID = e.currentTarget.dataset.article;
        window.location.href = `/article/${articleID}`;
      });
    });
  }

  toggleNode(path) {
    if (this.expandedNodes.has(path)) {
      this.expandedNodes.delete(path);
    } else {
      this.expandedNodes.add(path);
    }
    this.saveState();
  }

  // 兼容旧的 selectCategory 调用（list.html 中使用）
  selectCategory(category) {
    this.selectedPath = category === 'all' ? null : category;
  }

  // 获取当前选中路径下的所有文章（含子目录）
  getFilteredArticles() {
    if (!this.selectedPath) return this.articles;
    return this.articles.filter(a =>
      a.folderPath === this.selectedPath ||
      a.folderPath.startsWith(this.selectedPath + '/')
    );
  }

  saveState() {
    localStorage.setItem('categoryTreeState', JSON.stringify([...this.expandedNodes]));
  }

  loadState() {
    try {
      const saved = localStorage.getItem('categoryTreeState');
      return saved ? new Set(JSON.parse(saved)) : new Set();
    } catch {
      return new Set();
    }
  }
}

window.CategoryTree = CategoryTree;
