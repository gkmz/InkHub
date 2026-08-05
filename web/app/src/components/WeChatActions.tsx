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
  const [error, setError] = useState(false);
  const [manualCopy, setManualCopy] = useState(false);
  const copy = async () => {
    setCopying(true);
    setError(false);
    try {
      await onCopy();
      setHasCopied(true);
    } catch {
      setError(true);
    } finally {
      setCopying(false);
    }
  };
  return <div className="wechat-delivery-actions">
    <div className="wechat-actions">
      <button className="primary" type="button" disabled={copying} onClick={() => void copy()}><Clipboard size={16} />{copying ? "正在复制…" : hasCopied ? "已复制" : "复制格式化内容"}</button>
      <button className="secondary" type="button" onClick={onOpenPlatform}><ExternalLink size={16} />打开微信公众平台</button>
      {hasCopied && <button className="secondary" type="button" onClick={() => void onConfirm()}><Check size={16} />草稿已保存</button>}
    </div>
    {error && <span className="clipboard-error" role="alert">无法自动写入剪贴板。<button type="button" onClick={() => setManualCopy(true)}>手工复制</button></span>}
    {manualCopy && <ManualWeChatCopyDialog html={html} onClose={() => setManualCopy(false)} />}
  </div>;
}

function openWeChatPlatform() {
  window.open(weChatPlatformURL, "_blank", "noopener,noreferrer");
}
