// 处理发布（上传图片并复制）
async function handlePublish() {
    const btn = document.querySelector('.btn-publish');
    if (btn.classList.contains('loading')) return;

    showLoading(btn, '正在上传图片...');
    const articleId = document.getElementById('articleId').value;

    try {
        const response = await fetch(`/api/publish/${articleId}`, {
            method: 'POST'
        });
        const data = await response.json();

        if (data.success) {
            // 严格检查：如果有错误日志，则不允许视为成功，不自动复制
            if (data.logs && data.logs.length > 0) {
                let errorMsg = '⚠️ 发布中断：检测到以下图片上传失败，请修复后再试：\n\n' + data.logs.join('\n');
                showNotification(errorMsg, 'error');
                console.error(errorMsg);
                return; // 终止后续操作
            }

            // 新策略：直接替换 articleContent 然后用 copyArticle 的逻辑
            const articleContent = document.getElementById('articleContent');
            const originalHTML = articleContent.innerHTML;

            // 替换为 CDN 版本
            articleContent.innerHTML = data.content.html;

            // 清除之前的处理标记（因为内容已经被替换）
            delete articleContent.dataset.linksProcessed;

            // 应用格式化
            if (window.formatWechatContent) {
                window.formatWechatContent(articleContent);
            }

            // 等待浏览器完成渲染（关键）
            await new Promise(resolve => setTimeout(resolve, 100));

            // 克隆内容并简化代码块
            const clonedContent = articleContent.cloneNode(true);

            // 清除容器的背景色，确保只有代码块有背景
            clonedContent.style.background = 'transparent';
            clonedContent.style.backgroundColor = 'transparent';

            simplifyCodeBlocks(clonedContent);

            // 临时插入到 DOM 中进行复制
            clonedContent.style.position = 'absolute';
            clonedContent.style.left = '-9999px';
            document.body.appendChild(clonedContent);

            // 使用与 copyArticle 完全相同的逻辑复制
            const range = document.createRange();
            range.selectNodeContents(clonedContent);
            const selection = window.getSelection();
            selection.removeAllRanges();
            selection.addRange(range);
            document.execCommand('copy');
            selection.removeAllRanges();

            // 清理临时元素
            document.body.removeChild(clonedContent);

            // 恢复原内容（原内容已经格式化过，不需要再次格式化）
            articleContent.innerHTML = originalHTML;

            let msg = '✅ 发布成功！\n';
            if (data.uploaded && data.uploaded.length > 0) {
                msg += `🚀 已上传 ${data.uploaded.length} 张图片到 GitHub\n`;
            } else {
                msg += '📝 没有发现需要上传的图片（或已全部存在）\n';
            }
            msg += '\n含 CDN 图片链接的内容已复制到剪贴板。';
            showNotification(msg, 'success');
        } else {
            showNotification('❌ 发布失败: ' + data.error, 'error');
        }
    } catch (err) {
        showNotification('❌ 请求失败: ' + err.message, 'error');
    } finally {
        hideLoading(btn, '🚀 发布/复制');
    }
}

// 辅助函数：复制 HTML 内容（复用 copyArticle 的部分逻辑，但这里不仅要复制 HTML，
// 还要确保图片 src 是远程的。handlePublish 返回的 html 已经是远程链接了）
async function copyToClipboard(htmlString) {
    const tempDiv = document.createElement('div');

    // 关键修正：添加 container class 和内联样式，确保粘贴到微信时格式正确
    tempDiv.className = 'article-content';

    // 注入样式 + 内容
    const style = document.createElement('style');
    style.innerHTML = getInlineStyles(); // 获取 wechat.css 的核心样式
    tempDiv.appendChild(style);

    // 创建一个 wrapper 避免 style 标签直接和内容混在一起可能导致的问题（视具体粘贴目标而定，分离开更稳）
    const contentWrapper = document.createElement('div');
    contentWrapper.innerHTML = htmlString;
    tempDiv.appendChild(contentWrapper);

    tempDiv.style.position = 'absolute';
    tempDiv.style.left = '-9999px';
    document.body.appendChild(tempDiv);

    try {
        const range = document.createRange();
        range.selectNodeContents(tempDiv);
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);
        document.execCommand('copy');
        selection.removeAllRanges();
    } finally {
        document.body.removeChild(tempDiv);
    }
}

function showLoading(btn, text) {
    btn.classList.add('loading');
    btn.dataset.originalText = btn.innerText;
    btn.innerText = text;
    btn.style.opacity = '0.7';
    btn.style.cursor = 'wait';
}

function hideLoading(btn, text) {
    btn.classList.remove('loading');
    btn.innerText = text;
    btn.style.opacity = '1';
    btn.style.cursor = 'pointer';
}
async function copyArticle() {
    const content = document.getElementById('articleContent');

    if (!content) {
        alert('未找到文章内容');
        return;
    }

    try {
        // 克隆内容以避免修改原始 DOM
        const clonedContent = content.cloneNode(true);

        // 清除容器的背景色，确保只有代码块有背景
        clonedContent.style.background = 'transparent';
        clonedContent.style.backgroundColor = 'transparent';

        // 将所有代码块转换为纯文本格式（保留空格）
        simplifyCodeBlocks(clonedContent);

        // 临时插入到 DOM 中进行复制
        clonedContent.style.position = 'absolute';
        clonedContent.style.left = '-9999px';
        document.body.appendChild(clonedContent);

        // 使用 Selection API 复制富文本（包含样式）
        const range = document.createRange();
        range.selectNodeContents(clonedContent);

        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);

        // 执行复制命令
        document.execCommand('copy');

        // 清除选区和临时元素
        selection.removeAllRanges();
        document.body.removeChild(clonedContent);

        showNotification('✅ 复制成功！\n\n可直接粘贴到微信公众号后台。\n⚠️ 注意：图片需要手动上传。', 'success');
    } catch (err) {
        showNotification('❌ 复制失败\n\n' + err.message + '\n\n请尝试手动选中文章内容后按 Cmd+C 复制。', 'error');
    }
}

// 简化代码块：重构为简单结构，使用 &nbsp; 确保空格保留
function simplifyCodeBlocks(container) {
    // 清除所有非代码块元素的背景色
    const allElements = container.querySelectorAll('*:not(pre):not(code)');
    allElements.forEach(el => {
        el.style.background = 'transparent';
        el.style.backgroundColor = 'transparent';
    });

    // 处理代码块
    const preElements = container.querySelectorAll('pre');

    preElements.forEach(pre => {
        // 获取所有 .line 元素
        const lines = pre.querySelectorAll('.line');

        if (lines.length > 0) {
            // 重构代码块：为每一行创建独立的 div
            const newContent = document.createElement('code');
            newContent.style.display = 'block';
            newContent.style.whiteSpace = 'pre-wrap';
            newContent.style.background = 'transparent';

            lines.forEach((line, index) => {
                // 获取这一行的 HTML（保留颜色）
                let lineHTML = line.innerHTML;

                // 如果有 .cl 子元素，使用它的内容
                const clElement = line.querySelector('.cl');
                if (clElement) {
                    lineHTML = clElement.innerHTML;
                }

                // 替换 Tab 为 4 个 &nbsp;
                lineHTML = lineHTML.replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
                lineHTML = lineHTML.replace(/&#9;/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
                lineHTML = lineHTML.replace(/&#x9;/g, '&nbsp;&nbsp;&nbsp;&nbsp;');

                // 关键修改：将所有空格替换为 &nbsp;
                // 但要注意不要替换 HTML 标签内的空格
                lineHTML = lineHTML.replace(/(<[^>]*>)|( )/g, function (match, tag, space) {
                    if (tag) {
                        return tag; // 保留 HTML 标签不变
                    } else {
                        return '&nbsp;'; // 替换空格为 &nbsp;
                    }
                });

                // 创建一个 div 来包裹这一行
                const lineDiv = document.createElement('div');
                lineDiv.innerHTML = lineHTML;
                lineDiv.style.whiteSpace = 'pre-wrap';
                lineDiv.style.margin = '0';
                lineDiv.style.padding = '0';
                lineDiv.style.lineHeight = '1.5';

                newContent.appendChild(lineDiv);
            });

            // 清空 pre 并添加新内容
            pre.innerHTML = '';
            pre.appendChild(newContent);
        }

        // 设置 pre 的样式
        pre.style.background = '#272822';
        pre.style.backgroundColor = '#272822';
        pre.style.padding = '16px';
        pre.style.borderRadius = '6px';
        pre.style.whiteSpace = 'pre-wrap';
        pre.style.fontFamily = '"SF Mono", Monaco, Menlo, Consolas, "Courier New", monospace';
        pre.style.fontSize = '13px';
        pre.style.lineHeight = '1.5';
        pre.style.color = '#f8f8f2';
    });
}

// 获取内联样式（从 wechat.css 提取核心样式）
function getInlineStyles() {
    return `
        body {
            font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", sans-serif;
            font-size: 16px;
            line-height: 1.75;
            color: #333;
            max-width: 750px;
            margin: 0 auto;
            padding: 20px;
        }
        h1 { font-size: 24px; font-weight: bold; margin: 30px 0 20px; color: #2c3e50; }
        h2 { font-size: 20px; font-weight: bold; margin: 25px 0 15px; color: #34495e; }
        h3 { font-size: 18px; font-weight: bold; margin: 20px 0 12px; color: #34495e; }
        p { margin: 15px 0; text-align: justify; }
        strong { font-weight: 600; color: #2c3e50; }
        a { color: #3498db; text-decoration: none; border-bottom: 1px solid #3498db; }
        code {
            font-family: Monaco, Consolas, monospace;
            font-size: 14px;
            background: #f6f8fa;
            padding: 2px 6px;
            border-radius: 3px;
            color: #e83e8c;
            white-space: pre-wrap;
            word-break: break-word;
        }
        pre {
            border-radius: 6px;
            padding: 16px;
            overflow-x: auto;
            margin: 20px 0;
            line-height: 1.5;
            white-space: pre !important;
            word-wrap: normal;
            background: #2d2d2d;
            border: 1px solid #e1e4e8;
        }
        pre code {
            background: transparent;
            padding: 0;
            color: #f8f8f2;
            font-family: inherit;
            white-space: pre !important;
            word-break: normal;
            display: block;
        }
        pre *, pre code * {
            white-space: pre !important;
        }
        blockquote {
            border-left: 4px solid #42b983;
            background: #f9f9f9;
            padding: 12px 16px;
            margin: 20px 0;
            color: #666;
        }
        img {
            max-width: 100%;
            height: auto;
            display: block;
            margin: 20px auto;
            border-radius: 8px;
        }
        ul, ol { padding-left: 25px; margin: 15px 0; }
        li { margin: 8px 0; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { border: 1px solid #dfe2e5; padding: 10px; }
        th { background: #f6f8fa; font-weight: 600; }
    `;
}

// 显示通知
function showNotification(message, type = 'info') {
    const bg = {
        'success': '#d4edda',
        'warning': '#fff3cd',
        'error': '#f8d7da',
        'info': '#d1ecf1'
    }[type] || '#d1ecf1';

    const color = {
        'success': '#155724',
        'warning': '#856404',
        'error': '#721c24',
        'info': '#0c5460'
    }[type] || '#0c5460';

    // 创建通知元素
    const notification = document.createElement('div');
    notification.style.cssText = `
        position: fixed;
        top: 80px;
        right: 20px;
        background: ${bg};
        color: ${color};
        padding: 15px 20px;
        border-radius: 8px;
        box-shadow: 0 4px 12px rgba(0,0,0,0.15);
        z-index: 9999;
        max-width: 300px;
        white-space: pre-line;
        animation: slideIn 0.3s ease;
    `;
    notification.textContent = message;

    // 添加动画
    const style = document.createElement('style');
    style.textContent = `
        @keyframes slideIn {
            from { transform: translateX(400px); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }
    `;
    document.head.appendChild(style);

    document.body.appendChild(notification);

    // 3秒后自动消失
    setTimeout(() => {
        notification.style.animation = 'slideIn 0.3s ease reverse';
        setTimeout(() => notification.remove(), 300);
    }, 3000);
}
