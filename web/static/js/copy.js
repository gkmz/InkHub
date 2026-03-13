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
            let errorMsg =
              "⚠️ 发布中断：检测到以下图片上传失败，请修复后再试：\n\n" +
              data.logs.join("\n");
            showNotification(errorMsg, "error");
            console.error(errorMsg);
            return; // 终止后续操作
          }

          // 新策略：直接替换 articleContent 然后用 copyArticle 的逻辑
          const articleContent = document.getElementById("articleContent");
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
          await new Promise((resolve) => setTimeout(resolve, 100));

          // 克隆内容并简化代码块
          const clonedContent = articleContent.cloneNode(true);

          // 清除容器的背景色，确保只有代码块有背景
          clonedContent.style.background = "transparent";
          clonedContent.style.backgroundColor = "transparent";

          // 内联所有元素的样式（关键：确保标题等样式被复制）
          inlineStyles(clonedContent);

          // 将所有代码块转换为纯文本格式（保留空格）- 必须在 cleanupWhitespace 之前
          simplifyCodeBlocks(clonedContent);

          // 清理多余的空白字符（不影响代码块）
          cleanupWhitespace(clonedContent);

          // 临时插入到 DOM 中进行复制
          clonedContent.style.position = "absolute";
          clonedContent.style.left = "-9999px";
          document.body.appendChild(clonedContent);

          try {
            // 使用现代 Clipboard API 支持大文件
            const htmlContent = clonedContent.innerHTML;
            const textContent = clonedContent.innerText;

            const clipboardItem = new ClipboardItem({
              "text/html": new Blob([htmlContent], { type: "text/html" }),
              "text/plain": new Blob([textContent], { type: "text/plain" }),
            });

            await navigator.clipboard.write([clipboardItem]);
          } catch (clipboardErr) {
            // 降级到 execCommand（兼容旧浏览器）
            console.warn(
              "Clipboard API 失败，降级到 execCommand:",
              clipboardErr,
            );

            const range = document.createRange();
            range.selectNodeContents(clonedContent);
            const selection = window.getSelection();
            selection.removeAllRanges();
            selection.addRange(range);
            document.execCommand("copy");
            selection.removeAllRanges();
          } finally {
            // 清理临时元素
            document.body.removeChild(clonedContent);
          }

          // 恢复原内容（原内容已经格式化过，不需要再次格式化）
          articleContent.innerHTML = originalHTML;

          let msg = "✅ 发布成功！\n";
          if (data.uploaded && data.uploaded.length > 0) {
            msg += `🚀 已上传 ${data.uploaded.length} 张图片到 GitHub\n`;
          } else {
            msg += "📝 没有发现需要上传的图片（或已全部存在）\n";
          }
          msg += "\n含 CDN 图片链接的内容已复制到剪贴板。";
          showNotification(msg, "success");
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

      // 移除所有 .no-copy 元素
      clonedContent.querySelectorAll(".no-copy").forEach((el) => el.remove());

      // 清除容器的背景色，确保只有代码块有背景
      clonedContent.style.background = "transparent";
      clonedContent.style.backgroundColor = "transparent";

      // 内联所有元素的样式（关键：确保标题等样式被复制）
      inlineStyles(clonedContent);

      // 将所有代码块转换为纯文本格式（保留空格）- 必须在 cleanupWhitespace 之前
      simplifyCodeBlocks(clonedContent);

      // 清理多余的空白字符（不影响代码块）
      cleanupWhitespace(clonedContent);

      // 临时插入到 DOM 中
      clonedContent.style.position = "absolute";
      clonedContent.style.left = "-9999px";
      document.body.appendChild(clonedContent);

      try {
        // 使用现代 Clipboard API 支持大文件
        const htmlContent = clonedContent.innerHTML;
        const textContent = clonedContent.innerText;

        const clipboardItem = new ClipboardItem({
          "text/html": new Blob([htmlContent], { type: "text/html" }),
          "text/plain": new Blob([textContent], { type: "text/plain" }),
        });

        await navigator.clipboard.write([clipboardItem]);

        showNotification(
          "✅ 复制成功！\n\n可直接粘贴到微信公众号后台。\n⚠️ 注意：图片需要手动上传。",
          "success",
        );
      } catch (clipboardErr) {
        // 降级到 execCommand（兼容旧浏览器）
        console.warn("Clipboard API 失败，降级到 execCommand:", clipboardErr);

        const range = document.createRange();
        range.selectNodeContents(clonedContent);

        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);

        const success = document.execCommand("copy");
        selection.removeAllRanges();

        if (success) {
          showNotification(
            "✅ 复制成功！\n\n可直接粘贴到微信公众号后台。\n⚠️ 注意：图片需要手动上传。",
            "success",
          );
        } else {
          throw new Error("execCommand 复制失败");
        }
      } finally {
        // 清除临时元素
        document.body.removeChild(clonedContent);
      }
    } catch (err) {
        showNotification('❌ 复制失败\n\n' + err.message + '\n\n请尝试手动选中文章内容后按 Cmd+C 复制。', 'error');
    }
}
// 内联样式函数：将 CSS 样式应用到元素的 style 属性
function inlineStyles(container) {
    // 标题样式
    container.querySelectorAll('h1').forEach(el => {
        el.style.fontSize = '24px';
        el.style.fontWeight = 'bold';
        el.style.margin = '30px 0 20px';
        el.style.color = '#2c3e50';
        el.style.lineHeight = '1.4';
    });

    container.querySelectorAll('h2').forEach(el => {
        el.style.fontSize = '20px';
        el.style.fontWeight = 'bold';
        el.style.margin = '25px 0 15px';
        el.style.color = '#34495e';
        el.style.lineHeight = '1.4';
    });

    container.querySelectorAll('h3').forEach(el => {
        el.style.fontSize = '18px';
        el.style.fontWeight = 'bold';
        el.style.margin = '20px 0 12px';
        el.style.color = '#34495e';
        el.style.lineHeight = '1.4';
    });

    container.querySelectorAll('h4').forEach(el => {
        el.style.fontSize = '16px';
        el.style.fontWeight = 'bold';
        el.style.margin = '18px 0 10px';
        el.style.color = '#34495e';
    });

    // 段落样式
    container.querySelectorAll('p').forEach(el => {
        el.style.margin = '15px 0';
        el.style.textAlign = 'justify';
        el.style.wordBreak = 'break-word';
    });

    // 强调样式
    container.querySelectorAll('strong').forEach(el => {
        el.style.fontWeight = '600';
        el.style.color = '#2c3e50';
    });

    // 链接样式
    container.querySelectorAll('a').forEach(el => {
        el.style.color = '#3498db';
        el.style.textDecoration = 'none';
        el.style.borderBottom = '1px solid #3498db';
    });

    // 列表样式
    container.querySelectorAll('ul, ol').forEach(el => {
        el.style.paddingLeft = '25px';
        el.style.margin = '15px 0';
        el.style.lineHeight = '1.8';
    });

    container.querySelectorAll('li').forEach(el => {
        el.style.margin = '10px 0';
        el.style.lineHeight = '1.8';
    });

    // 引用块样式
    container.querySelectorAll('blockquote').forEach(el => {
        el.style.borderLeft = '4px solid #42b983';
        el.style.background = '#f9f9f9';
        el.style.padding = '12px 16px';
        el.style.margin = '20px 0';
        el.style.color = '#666';
        el.style.borderRadius = '4px';
    });

    // 行内代码样式
    container.querySelectorAll('code:not(pre code)').forEach(el => {
        el.style.fontFamily = '"SF Mono", Monaco, Menlo, Consolas, "Courier New", monospace';
        el.style.fontSize = '14px';
        el.style.background = '#f6f8fa';
        el.style.padding = '2px 6px';
        el.style.borderRadius = '3px';
        el.style.color = '#e83e8c';
        el.style.whiteSpace = 'pre-wrap';
        el.style.wordBreak = 'break-word';
    });

    // 图片样式
    container.querySelectorAll('img').forEach(el => {
        el.style.maxWidth = '100%';
        el.style.height = 'auto';
        el.style.display = 'block';
        el.style.margin = '20px auto';
        el.style.borderRadius = '8px';
    });

    // 表格样式
    container.querySelectorAll('table').forEach(el => {
        el.style.width = '100%';
        el.style.borderCollapse = 'collapse';
        el.style.margin = '20px 0';
        el.style.fontSize = '14px';
    });

    container.querySelectorAll('th, td').forEach(el => {
        el.style.border = '1px solid #dfe2e5';
        el.style.padding = '10px 12px';
        el.style.textAlign = 'left';
    });

    container.querySelectorAll('th').forEach(el => {
        el.style.background = '#f6f8fa';
        el.style.fontWeight = '600';
        el.style.color = '#24292e';
    });
}
// 清理多余的空白字符
function cleanupWhitespace(container) {
    // 清理所有文本节点中的多余空格（但不包括 pre 和 code 元素）
    const walker = document.createTreeWalker(
        container,
        NodeFilter.SHOW_TEXT,
        {
            acceptNode: function(node) {
                // 跳过 pre 和 code 元素内的文本节点
                let parent = node.parentElement;
                while (parent && parent !== container) {
                    if (parent.tagName === 'PRE' || parent.tagName === 'CODE') {
                        return NodeFilter.FILTER_REJECT;
                    }
                    parent = parent.parentElement;
                }
                return NodeFilter.FILTER_ACCEPT;
            }
        }
    );

    const textNodes = [];
    let node;
    while (node = walker.nextNode()) {
        textNodes.push(node);
    }

    // 清理每个文本节点
    textNodes.forEach(textNode => {
        // 将连续的空白字符（包括换行、制表符）替换为单个空格
        let text = textNode.textContent;
        text = text.replace(/\s+/g, ' ');

        // 如果是块级元素的开头或结尾，去掉首尾空格
        const parent = textNode.parentElement;
        if (parent) {
            const isBlockElement = ['P', 'DIV', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'LI', 'BLOCKQUOTE'].includes(parent.tagName);
            if (isBlockElement) {
                if (textNode === parent.firstChild) {
                    text = text.trimStart();
                }
                if (textNode === parent.lastChild) {
                    text = text.trimEnd();
                }
            }
        }

        textNode.textContent = text;
    });
}



// 简化代码块：重构为简单结构，使用 &nbsp; 确保空格保留
// 简化代码块：确保空格保留，适用于内联样式
function simplifyCodeBlocks(container) {
  // 清除所有非代码块元素的背景色
  const allElements = container.querySelectorAll("*:not(pre):not(code)");
  allElements.forEach((el) => {
    el.style.background = "transparent";
    el.style.backgroundColor = "transparent";
  });

  // 处理代码块
  const preElements = container.querySelectorAll("pre");

  preElements.forEach((pre) => {
    // 设置 pre 的样式
    pre.style.background = "#272822";
    pre.style.backgroundColor = "#272822";
    pre.style.padding = "16px";
    pre.style.borderRadius = "6px";
    pre.style.whiteSpace = "pre";
    pre.style.fontFamily =
      '"SF Mono", Monaco, Menlo, Consolas, "Courier New", monospace';
    pre.style.fontSize = "13px";
    pre.style.lineHeight = "1.5";
    pre.style.color = "#f8f8f2";
    pre.style.overflowX = "auto";

    // 获取 code 元素
    const codeElement = pre.querySelector("code");
    if (codeElement) {
      codeElement.style.background = "transparent";
      codeElement.style.padding = "0";
      codeElement.style.whiteSpace = "pre";
      codeElement.style.display = "block";

      // 将代码内容中的空格替换为 &nbsp; 以确保在微信中正确显示
      replaceSpacesInCode(codeElement);
    }
  });
}

// 递归替换代码元素中的空格为 &nbsp;
function replaceSpacesInCode(element) {
  const childNodes = Array.from(element.childNodes);

  childNodes.forEach((node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      // 文本节点：替换空格和制表符
      let text = node.textContent;
      text = text.replace(/\t/g, "    "); // Tab 转 4 个空格
      text = text.replace(/ /g, "\u00A0"); // 空格转 &nbsp; (使用 Unicode)
      node.textContent = text;
    } else if (node.nodeType === Node.ELEMENT_NODE) {
      // 元素节点：递归处理
      replaceSpacesInCode(node);
    }
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
