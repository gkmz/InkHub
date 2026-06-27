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

  // 转义动态文本，避免文件名中的特殊字符破坏导航树结构
  escapeHTML(value) {
    return String(value ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  // 导航栏必须展示真实文件名，不能使用文章内容里的标题
  getArticleFileName(article) {
    const relPath = article.relPath || article.path || article.title || '';
    const parts = String(relPath).split(/[\\/]/);
    return parts[parts.length - 1] || article.title || article.id;
  }

  // 文件名里的编号需要参与排序，便于按 01、02 这类前缀查找
  compareArticleByFileName(a, b) {
    return this.getArticleFileName(a).localeCompare(
      this.getArticleFileName(b),
      'zh-CN',
      { numeric: true, sensitivity: 'base' }
    );
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
    const rootArticles = this.renderArticles(this.tree, 0);
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
        ${rootArticles}
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

  // 渲染当前目录下的文章文件
  renderArticles(node, depth) {
    const indent = depth * 14;
    return [...node.articles]
      .sort((a, b) => this.compareArticleByFileName(a, b))
      .map(article => {
        const platformIcons = this.getPlatformIcons(article.id);
        const fileName = this.escapeHTML(this.getArticleFileName(article));
        const relPath = this.escapeHTML(article.relPath || this.getArticleFileName(article));
        const articleID = this.escapeHTML(article.id);
        return `
          <div class="tree-node-header tree-article" data-article="${articleID}"
               style="padding-left: ${indent + 28}px" title="${relPath}">
            <span class="tree-icon">📄</span>
            <span class="tree-label">${fileName}</span>
            ${platformIcons ? `<span class="tree-platforms">${platformIcons}</span>` : ''}
          </div>
        `;
      }).join('');
  }

  renderNode(node, depth) {
    const isExpanded = this.expandedNodes.has(node.path);
    const isSelected = this.selectedPath === node.path;
    const hasChildren = Object.keys(node.children).length > 0;
    // 叶子目录（只有文章）也需要 toggle 控制
    const hasArticles = node.articles.length > 0;
    const isExpandable = hasChildren || hasArticles;
    const indent = depth * 14;

    const articles = this.renderArticles(node, depth);

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
          ${isExpandable
            ? `<span class="tree-toggle ${isExpanded ? 'expanded' : 'collapsed'}"
                     data-toggle="${node.path}"></span>`
            : `<span class="tree-toggle-placeholder"></span>`
          }
          <span class="tree-icon">${hasChildren ? (isExpanded ? '📂' : '📁') : '📂'}</span>
          <span class="tree-label">${node.name}</span>
          <span class="tree-count">${node.count}</span>
        </div>
        ${isExpanded ? articles : ''}
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
        window.location.href = `/article/${encodeURIComponent(articleID)}`;
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
    sessionStorage.setItem('categoryTreeState', JSON.stringify([...this.expandedNodes]));
  }

  loadState() {
    localStorage.removeItem('categoryTreeState');
    try {
      const saved = sessionStorage.getItem('categoryTreeState');
      return saved ? new Set(JSON.parse(saved)) : new Set();
    } catch {
      return new Set();
    }
  }
}

window.CategoryTree = CategoryTree;

// 初始化侧栏和正文之间的拖拽分割条，列表页和文章页共用同一套行为
function initSidebarResize() {
  const resizer = document.getElementById('sidebarResizer');
  const root = document.documentElement;
  const layout = document.querySelector('.layout-container');
  if (!resizer || !layout) return;

  const STORAGE_KEY = 'preview.sidebar.width';
  const MIN_W = 220;
  const MAX_W = 620;
  const stackedQuery = window.matchMedia('(max-width: 1024px)');

  // 根据当前视口限制最大宽度，避免拖到正文完全不可用
  const clamp = (value) => {
    const viewportMax = Math.max(MIN_W, Math.min(MAX_W, window.innerWidth - 360));
    return Math.max(MIN_W, Math.min(viewportMax, value));
  };

  const applyWidth = (value) => {
    const width = clamp(value);
    root.style.setProperty('--sidebar-width', `${width}px`);
    localStorage.setItem(STORAGE_KEY, String(width));
  };

  const saved = Number(localStorage.getItem(STORAGE_KEY));
  if (!Number.isNaN(saved) && saved > 0 && !stackedQuery.matches) {
    applyWidth(saved);
  }

  let dragging = false;

  const stop = () => {
    if (!dragging) return;
    dragging = false;
    layout.classList.remove('resizing');
    document.body.classList.remove('sidebar-resizing');
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', stop);
    window.removeEventListener('pointercancel', stop);
  };

  const onMove = (event) => {
    if (!dragging || stackedQuery.matches) return;
    const layoutLeft = layout.getBoundingClientRect().left;
    applyWidth(event.clientX - layoutLeft);
  };

  resizer.addEventListener('pointerdown', (event) => {
    if (stackedQuery.matches) return;
    event.preventDefault();
    dragging = true;
    layout.classList.add('resizing');
    document.body.classList.add('sidebar-resizing');
    resizer.setPointerCapture?.(event.pointerId);
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', stop);
    window.addEventListener('pointercancel', stop);
  });

  window.addEventListener('resize', () => {
    if (stackedQuery.matches) {
      stop();
      return;
    }

    const current = Number.parseInt(getComputedStyle(root).getPropertyValue('--sidebar-width'), 10);
    if (!Number.isNaN(current)) {
      applyWidth(current);
    }
  });
}

window.initSidebarResize = initSidebarResize;
