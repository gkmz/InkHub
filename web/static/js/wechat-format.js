// 微信公众号格式化工具：自动处理外链为文末引用
document.addEventListener('DOMContentLoaded', function () {
    formatWechatContent();
});

// 导出函数供其他模块使用（如 copy.js）
window.formatWechatContent = formatWechatContent;

function formatWechatContent(container) {
    const content = container || document.querySelector('.article-content');
    if (!content) return;

    processImages(content);
    processLinks(content);
}

function processLinks(container) {
    // 如果没有传入容器，则查找页面中的 .article-content
    const content = container || document.querySelector('.article-content');
    if (!content) return;

    // 检查是否已经处理过（避免重复处理）
    if (content.dataset.linksProcessed === 'true') return;

    // 获取所有链接
    const links = content.querySelectorAll('a');
    const references = [];
    let index = 1;

    links.forEach(link => {
        const href = link.getAttribute('href');
        const text = link.innerText;

        // 忽略无效链接、锚点链接、javascript链接
        if (!href || href.startsWith('#') || href.startsWith('javascript:')) return;

        // 忽略图片链接（如果有的话，通常 markdown 图片渲染为 img 标签，但有时会有链接包裹）
        if (link.querySelector('img')) return;

        // 忽略已经在 code 块里的链接
        if (link.closest('pre') || link.closest('code')) return;

        // 记录引用
        references.push({
            index: index,
            text: text,
            href: href
        });

        // 修改 DOM：在链接后添加上标
        // 微信不支持外链跳转，所以我们可以保留a标签样式（蓝色），但实际上点击无效（在微信里），
        // 或者保留a标签但标明不可跳转。
        // 这里我们保留a标签，但添加上标 [n]
        const sup = document.createElement('sup');
        sup.className = 'reference-index';
        sup.textContent = `[${index}]`;
        sup.style.marginLeft = '3px';
        sup.style.color = '#42b883';
        sup.style.fontSize = '0.72em';
        sup.style.fontWeight = '700';

        link.parentNode.insertBefore(sup, link.nextSibling);

        index++;
    });

    // 如果有引用，在文末添加引用列表
    if (references.length > 0 && !content.querySelector('.references-section')) {
        appendReferences(content, references);
    }

    // 标记为已处理
    content.dataset.linksProcessed = 'true';
}

function processImages(content) {
    if (content.dataset.imagesProcessed === 'true') return;

    const images = content.querySelectorAll('img');
    images.forEach((img) => {
        if (img.closest('pre, code, .mac-image-frame')) return;

        const paragraph = img.parentElement;
        const meaningfulChildren = paragraph
            ? Array.from(paragraph.childNodes).filter((node) => {
                  if (node.nodeType === Node.TEXT_NODE) {
                      return node.textContent.trim() !== '';
                  }
                  return true;
              })
            : [];
        const isStandaloneImage =
            paragraph &&
            paragraph.tagName === 'P' &&
            meaningfulChildren.length === 1;

        if (!isStandaloneImage) return;

        const frame = document.createElement('figure');
        frame.className = 'mac-image-frame';

        const toolbar = document.createElement('div');
        toolbar.className = 'mac-image-toolbar';
        toolbar.innerHTML = `
            <span class="mac-image-dot dot-red"></span>
            <span class="mac-image-dot dot-yellow"></span>
            <span class="mac-image-dot dot-green"></span>
        `;

        const body = document.createElement('div');
        body.className = 'mac-image-body';

        const src = (img.getAttribute('src') || '').toLowerCase();
        const alt = (img.getAttribute('alt') || '').toLowerCase();
        const isMermaidDiagram =
            src.includes('generated-mermaid/') ||
            src.includes('mermaid-') ||
            alt.includes('mermaid');
        if (isMermaidDiagram) {
            frame.classList.add('mermaid-diagram-frame');
            body.classList.add('mermaid-diagram-body');
            img.classList.add('mermaid-diagram-image');
        }

        paragraph.parentNode.insertBefore(frame, paragraph);
        body.appendChild(img);
        frame.appendChild(toolbar);
        frame.appendChild(body);
        paragraph.remove();
    });

    content.dataset.imagesProcessed = 'true';
}

function appendReferences(container, references) {
    // 创建引用容器
    const refSection = document.createElement('div');
    refSection.className = 'references-section';
    refSection.style.marginTop = '2.4em';
    refSection.style.padding = '12px 16px';
    refSection.style.backgroundColor = '#f8f9fa';
    refSection.style.borderLeft = '4px solid #42b883';
    refSection.style.borderRadius = '2px';

    // 标题
    const title = document.createElement('h3');
    title.textContent = '引用链接';
    title.style.fontSize = '1rem';
    title.style.fontWeight = '600';
    title.style.color = '#2c3e50';
    title.style.margin = '0 0 10px';
    refSection.appendChild(title);

    // 列表
    const list = document.createElement('ul');
    list.style.margin = '0';
    list.style.paddingLeft = '0';
    list.style.listStyle = 'none';

    references.forEach(ref => {
        const item = document.createElement('li');
        item.style.fontSize = '14px';
        item.style.color = '#5c6975';
        item.style.margin = '0.45rem 0';
        item.style.lineHeight = '1.55';
        item.style.display = 'block'; // 覆盖 wechat.css 可能的 list-item

        item.innerHTML = `
            <span class="li-text">
                <span class="reference-label" style="color: #42b883; font-weight: 700; margin-right: 6px;">[${ref.index}]</span>
                ${escapeHtml(ref.text)}: 
                <span class="reference-url" style="color: #34495e; word-break: break-all;">${ref.href}</span>
            </span>
        `;
        list.appendChild(item);
    });

    refSection.appendChild(list);
    container.appendChild(refSection);
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
