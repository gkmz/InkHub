import { Check, Clipboard, ExternalLink } from "lucide-react";
import { useState } from "react";
import { ManualWeChatCopyDialog } from "./ManualWeChatCopyDialog";

const weChatPlatformURL = "https://mp.weixin.qq.com/";

interface WeChatActionsProps {
  html?: string;
  copied: boolean;
  onCopy: () => Promise<void>;
  onConfirm: () => void | Promise<void>;
  onOpenPlatform?: () => void;
}

/** WeChatActions 强制将准备、复制和人工确认保持为三个独立动作。 */
export function WeChatActions({ html = "", copied, onCopy, onConfirm, onOpenPlatform = openWeChatPlatform }: WeChatActionsProps) {
  const [hasCopied, setHasCopied] = useState(copied);
  const [copying, setCopying] = useState(false);
  const [copySucceeded, setCopySucceeded] = useState(false);
  const [error, setError] = useState(false);
  const [manualCopy, setManualCopy] = useState(false);
  const copy = async () => {
    setCopying(true);
    setCopySucceeded(false);
    setError(false);
    try {
      await onCopy();
      setHasCopied(true);
      setCopySucceeded(true);
    } catch {
      setError(true);
    } finally {
      setCopying(false);
    }
  };
  return <div className="wechat-delivery-actions">
    <div className="wechat-actions">
      <button className="primary" type="button" disabled={copying} onClick={() => void copy()}>{copySucceeded ? <Check size={16} /> : <Clipboard size={16} />}{copying ? "正在复制…" : copySucceeded ? "复制成功" : hasCopied ? "已复制" : "复制格式化内容"}</button>
      <button className="secondary" type="button" onClick={onOpenPlatform}><ExternalLink size={16} />打开微信公众平台</button>
      {hasCopied && <button className="secondary" type="button" onClick={() => void onConfirm()}><Check size={16} />草稿已保存</button>}
    </div>
    {copySucceeded && <span className="clipboard-success" role="status"><Check size={14} />已复制到剪贴板，可直接粘贴到微信编辑器。</span>}
    {error && <span className="clipboard-error" role="alert">无法自动写入剪贴板。<button type="button" onClick={() => setManualCopy(true)}>手工复制</button></span>}
    {manualCopy && <ManualWeChatCopyDialog html={html} onClose={() => setManualCopy(false)} />}
  </div>;
}

function openWeChatPlatform() {
  window.open(weChatPlatformURL, "_blank", "noopener,noreferrer");
}
