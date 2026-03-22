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
    container.style.fontFamily = '"Source Sans 3", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';
    container.style.fontSize = '15px';
    container.style.lineHeight = '1.6';
    container.style.color = '#34495e';
    container.style.letterSpacing = '0';
    container.style.wordSpacing = '0.05rem';
    container.style.background = 'transparent';

    container.querySelectorAll('h1').forEach(el => {
        el.style.fontSize = '2rem';
        el.style.fontWeight = '600';
        el.style.margin = '0 0 1rem';
        el.style.color = '#2c3e50';
        el.style.lineHeight = '1.3';
    });

    container.querySelectorAll('h2').forEach(el => {
        el.style.fontSize = '1.75rem';
        el.style.fontWeight = '600';
        el.style.margin = '2.8em 0 0.9rem';
        el.style.color = '#2c3e50';
        el.style.lineHeight = '1.32';
    });

    container.querySelectorAll('h3').forEach(el => {
        el.style.fontSize = '1.5rem';
        el.style.fontWeight = '600';
        el.style.margin = '2.2em 0 0.7rem';
        el.style.color = '#2c3e50';
        el.style.lineHeight = '1.34';
    });

    container.querySelectorAll('h4').forEach(el => {
        el.style.fontSize = '1.25rem';
        el.style.fontWeight = '600';
        el.style.margin = '1.8em 0 0.55rem';
        el.style.color = '#2c3e50';
        el.style.lineHeight = '1.35';
    });

    container.querySelectorAll('h5').forEach(el => {
        el.style.fontSize = '1rem';
        el.style.fontWeight = '600';
        el.style.margin = '24px 0 0.4rem';
        el.style.color = '#2c3e50';
    });

    container.querySelectorAll('h6').forEach(el => {
        el.style.fontSize = '1rem';
        el.style.fontWeight = '600';
        el.style.margin = '24px 0 0.4rem';
        el.style.color = '#777';
    });

    container.querySelectorAll('p').forEach(el => {
        el.style.margin = '1.35em 0';
        el.style.lineHeight = '1.85';
        el.style.wordBreak = 'break-word';
    });

    container.querySelectorAll('strong').forEach(el => {
        el.style.fontWeight = '600';
        el.style.color = '#2c3e50';
    });

    container.querySelectorAll('em').forEach(el => {
        el.style.color = '#7f8c8d';
    });

    container.querySelectorAll('a').forEach(el => {
        el.style.color = '#42b883';
        el.style.fontWeight = '600';
        el.style.textDecoration = 'none';
    });

    container.querySelectorAll('sup.reference-index').forEach(el => {
        el.style.marginLeft = '3px';
        el.style.color = '#42b883';
        el.style.fontSize = '0.72em';
        el.style.fontWeight = '700';
        el.style.lineHeight = '0';
    });

    container.querySelectorAll('ul, ol').forEach(el => {
        el.style.paddingLeft = '1.5rem';
        el.style.lineHeight = '1.85';
        el.style.margin = '1.1em 0 1.3em';
    });

    container.querySelectorAll('li').forEach(el => {
        el.style.margin = '0.6rem 0';
    });

    container.querySelectorAll('blockquote').forEach(el => {
        el.style.borderLeft = '4px solid #42b883';
        el.style.padding = '0.55em 0 0.55em 18px';
        el.style.margin = '1.9em 0';
        el.style.color = '#858585';
        el.style.background = 'rgba(66, 184, 131, 0.04)';
        el.style.borderRadius = '0 10px 10px 0';
    });

    container.querySelectorAll('blockquote p').forEach(el => {
        el.style.margin = '0.55em 0';
        el.style.fontWeight = '600';
    });

    container.querySelectorAll('code:not(pre code)').forEach(el => {
        el.style.fontFamily = '"Roboto Mono", "SFMono-Regular", Monaco, Consolas, monospace';
        el.style.fontSize = '0.8rem';
        el.style.background = '#ffffff';
        el.style.border = '1px solid #f4bfd7';
        el.style.padding = '3px 6px';
        el.style.borderRadius = '6px';
        el.style.color = '#d63384';
        el.style.whiteSpace = 'pre-wrap';
        el.style.wordBreak = 'break-word';
    });

    container.querySelectorAll('.mac-image-frame').forEach(el => {
        el.style.margin = '1.8em 0 2em';
        el.style.background = '#f5f7fa';
        el.style.border = '1px solid #e8edf3';
        el.style.borderRadius = '16px';
        el.style.overflow = 'hidden';
        el.style.boxShadow = '0 18px 40px rgba(31, 45, 61, 0.08)';
    });

    container.querySelectorAll('.mac-image-toolbar').forEach(el => {
        el.style.display = 'flex';
        el.style.alignItems = 'center';
        el.style.gap = '8px';
        el.style.padding = '12px 14px';
        el.style.background = 'linear-gradient(180deg, #fdfefe 0%, #eef3f7 100%)';
        el.style.borderBottom = '1px solid #e6edf2';
    });

    container.querySelectorAll('.mac-image-dot').forEach(el => {
        el.style.width = '10px';
        el.style.height = '10px';
        el.style.borderRadius = '999px';
        el.style.display = 'inline-block';
    });

    container.querySelectorAll('.mac-image-dot.dot-red').forEach(el => {
        el.style.background = '#ff5f57';
    });

    container.querySelectorAll('.mac-image-dot.dot-yellow').forEach(el => {
        el.style.background = '#febc2e';
    });

    container.querySelectorAll('.mac-image-dot.dot-green').forEach(el => {
        el.style.background = '#28c840';
    });

    container.querySelectorAll('.mac-image-body').forEach(el => {
        el.style.padding = '12px';
        el.style.background = '#fff';
    });

    container.querySelectorAll('.mac-image-body img, img').forEach(el => {
        el.style.maxWidth = '100%';
        el.style.height = 'auto';
        el.style.display = 'block';
        el.style.margin = '0 auto';
    });

    container.querySelectorAll('table').forEach(el => {
        el.style.width = '100%';
        el.style.display = 'block';
        el.style.overflow = 'auto';
        el.style.borderCollapse = 'collapse';
        el.style.borderSpacing = '0';
        el.style.marginBottom = '1rem';
    });

    container.querySelectorAll('tr').forEach((el, index) => {
        el.style.borderTop = '1px solid #ccc';
        if (index % 2 === 1) {
            el.style.background = '#f8f8f8';
        }
    });

    container.querySelectorAll('th, td').forEach(el => {
        el.style.border = '1px solid #ddd';
        el.style.padding = '6px 13px';
        el.style.textAlign = 'left';
    });

    container.querySelectorAll('th').forEach(el => {
        el.style.fontWeight = '700';
        el.style.color = '#2c3e50';
    });

    container.querySelectorAll('.references-section').forEach(el => {
        el.style.marginTop = '2.4em';
        el.style.padding = '10px 14px';
        el.style.background = 'rgba(66, 184, 131, 0.045)';
        el.style.borderLeft = '3px solid #42b883';
        el.style.borderRadius = '8px';
    });

    container.querySelectorAll('.references-section h3').forEach(el => {
        el.style.margin = '0 0 10px';
        el.style.fontSize = '0.95rem';
        el.style.fontWeight = '600';
        el.style.color = '#2c3e50';
    });

    container.querySelectorAll('.references-section ul').forEach(el => {
        el.style.margin = '0';
        el.style.paddingLeft = '0';
        el.style.listStyle = 'none';
    });

    container.querySelectorAll('.references-section li').forEach(el => {
        el.style.display = 'block';
        el.style.margin = '0.35rem 0';
        el.style.color = '#5c6975';
        el.style.lineHeight = '1.7';
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
    pre.style.position = "relative";
    pre.style.background = "#1a1b26";
    pre.style.backgroundColor = "#1a1b26";
    pre.style.padding = "0 1.1rem";
    pre.style.borderRadius = "12px";
    pre.style.whiteSpace = "pre";
    pre.style.fontFamily =
      '"Roboto Mono", "SFMono-Regular", Monaco, Consolas, "Courier New", monospace';
    pre.style.fontSize = "0.82rem";
    pre.style.lineHeight = "1.7";
    pre.style.color = "#c0caf5";
    pre.style.overflowX = "auto";
    pre.style.margin = "1.5em 0";
    pre.style.border = "1px solid #2f3549";

    // 获取 code 元素
    const codeElement = pre.querySelector("code");
    if (codeElement) {
      codeElement.style.background = "transparent";
      codeElement.style.padding = "0";
      codeElement.style.whiteSpace = "pre";
      codeElement.style.display = "block";
      codeElement.style.margin = "0 2px";
      codeElement.style.padding = "2.3em 6px 1.1em";
      codeElement.style.lineHeight = "inherit";
      codeElement.style.color = "#c0caf5";
      codeElement.style.fontSize = "0.82rem";

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
            font-family: "Source Sans 3", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
            font-size: 15px;
            line-height: 1.6;
            color: #34495e;
            max-width: 960px;
            margin: 0 auto;
            padding: 0 15px 40px;
        }
        h1 { font-size: 2rem; font-weight: 600; margin: 0 0 1rem; color: #2c3e50; line-height: 1.3; }
        h2 { font-size: 1.75rem; font-weight: 600; margin: 2.8em 0 0.9rem; color: #2c3e50; line-height: 1.32; }
        h3 { font-size: 1.5rem; font-weight: 600; margin: 2.2em 0 0.7rem; color: #2c3e50; line-height: 1.34; }
        h4 { font-size: 1.25rem; font-weight: 600; margin: 1.8em 0 0.55rem; color: #2c3e50; line-height: 1.35; }
        p { margin: 1.35em 0; line-height: 1.85; word-break: break-word; }
        ul, ol { padding-left: 1.5rem; line-height: 1.85; margin: 1.1em 0 1.3em; }
        li { margin: 0.6rem 0; }
        strong { font-weight: 600; color: #2c3e50; }
        em { color: #7f8c8d; }
        a { color: #42b883; text-decoration: none; font-weight: 600; }
        blockquote {
            border-left: 4px solid #42b883;
            color: #858585;
            margin: 1.9em 0;
            padding: 0.55em 0 0.55em 18px;
            background: rgba(66, 184, 131, 0.04);
            border-radius: 0 10px 10px 0;
        }
        code {
            font-family: "Roboto Mono", Monaco, Consolas, monospace;
            font-size: 0.8rem;
            background: #ffffff;
            border: 1px solid #f4bfd7;
            padding: 3px 6px;
            border-radius: 6px;
            color: #d63384;
            white-space: pre-wrap;
            word-break: break-word;
        }
        pre {
            border-radius: 12px;
            padding: 0 1.1rem;
            overflow-x: auto;
            margin: 1.5em 0;
            line-height: 1.7;
            white-space: pre !important;
            word-wrap: normal;
            background: #1a1b26;
            border: 1px solid #2f3549;
        }
        pre code {
            background: transparent;
            border: none;
            padding: 2.3em 6px 1.1em;
            color: #c0caf5;
            font-family: inherit;
            white-space: pre !important;
            word-break: normal;
            display: block;
        }
        pre *, pre code * {
            white-space: pre !important;
        }
        img {
            max-width: 100%;
            height: auto;
            display: block;
            margin: 0 auto;
        }
        .mac-image-frame {
            margin: 1.8em 0 2em;
            background: #f5f7fa;
            border: 1px solid #e8edf3;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 18px 40px rgba(31, 45, 61, 0.08);
        }
        .mac-image-toolbar {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 12px 14px;
            background: linear-gradient(180deg, #fdfefe 0%, #eef3f7 100%);
            border-bottom: 1px solid #e6edf2;
        }
        .mac-image-dot {
            width: 10px;
            height: 10px;
            border-radius: 999px;
            display: inline-block;
        }
        .dot-red { background: #ff5f57; }
        .dot-yellow { background: #febc2e; }
        .dot-green { background: #28c840; }
        .mac-image-body {
            padding: 12px;
            background: #fff;
        }
        table {
            width: 100%;
            display: block;
            overflow: auto;
            border-collapse: collapse;
            border-spacing: 0;
            margin-bottom: 1rem;
        }
        tr { border-top: 1px solid #ccc; }
        tr:nth-child(2n) { background: #f8f8f8; }
        th, td { border: 1px solid #ddd; padding: 6px 13px; text-align: left; }
        th { font-weight: 700; color: #2c3e50; }
        .references-section {
            margin-top: 2.4em;
            padding: 10px 14px;
            background: rgba(66, 184, 131, 0.045);
            border-left: 3px solid #42b883;
            border-radius: 8px;
        }
        .references-section h3 {
            margin: 0 0 10px;
            font-size: 0.95rem;
            font-weight: 600;
            color: #2c3e50;
        }
        .references-section ul {
            margin: 0;
            padding-left: 0;
            list-style: none;
        }
        .references-section li {
            display: block;
            margin: 0.35rem 0;
            color: #5c6975;
            line-height: 1.7;
        }
        sup.reference-index {
            margin-left: 3px;
            color: #42b883;
            font-size: 0.72em;
            font-weight: 700;
            line-height: 0;
        }
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
