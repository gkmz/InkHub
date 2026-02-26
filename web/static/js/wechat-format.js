// 微信公众号格式化工具：自动处理外链为文末引用
document.addEventListener('DOMContentLoaded', function () {
    processLinks();
});

// 导出函数供其他模块使用（如 copy.js）
window.formatWechatContent = processLinks;

function processLinks(container) {
    // 如果没有传入容器，则查找页面中的 .article-content
    const content = container || document.querySelector('.article-content');
    if (!content) return;

    // 检查是否已经处理过（避免重复处理）
    if (content.dataset.linksProcessed === 'true') {
        return;
    }

    // 获取所有链接
    const links = content.querySelectorAll('a');
    if (links.length === 0) return;

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
        sup.textContent = `[${index}]`;
        sup.style.marginLeft = '2px';
        sup.style.color = '#999';

        link.parentNode.insertBefore(sup, link.nextSibling);

        index++;
    });

    // 如果有引用，在文末添加引用列表
    if (references.length > 0) {
        appendReferences(content, references);
    }

    // 标记为已处理
    content.dataset.linksProcessed = 'true';
}

function appendReferences(container, references) {
    // 创建引用容器
    const refSection = document.createElement('div');
    refSection.className = 'references-section';
    refSection.style.marginTop = '40px';
    refSection.style.paddingTop = '20px';
    refSection.style.borderTop = '1px solid #eee';

    // 标题
    const title = document.createElement('h3');
    title.textContent = '引用链接';
    title.style.fontSize = '16px';
    title.style.fontWeight = 'bold';
    title.style.marginBottom = '15px';
    refSection.appendChild(title);

    // 列表
    const list = document.createElement('ul');
    list.style.paddingLeft = '0';
    list.style.listStyle = 'none';

    references.forEach(ref => {
        const item = document.createElement('li');
        item.style.fontSize = '14px';
        item.style.color = '#666';
        item.style.marginBottom = '8px';
        item.style.lineHeight = '1.6';
        item.style.display = 'block'; // 覆盖 wechat.css 可能的 list-item

        // 格式：[1] 链接文本: https://...
        // 使用 span class="li-text" 包裹，防止微信编辑器自动换行
        item.innerHTML = `
            <span class="li-text">
                <span style="color: #999; margin-right: 5px;">[${ref.index}]</span>
                ${escapeHtml(ref.text)}: 
                <span style="color: #333; word-break: break-all;">${ref.href}</span>
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
