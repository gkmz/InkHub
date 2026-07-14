import { Check, Clipboard } from "lucide-react";
import { useState } from "react";

/** WeChatActions 强制将准备、复制和人工确认保持为三个独立动作。 */
export function WeChatActions({ copied, onCopy, onConfirm }: { copied: boolean; onCopy: () => Promise<void>; onConfirm: () => void | Promise<void> }) {
  const [hasCopied, setHasCopied] = useState(copied);
  const [copying, setCopying] = useState(false);
  const [error, setError] = useState(false);
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
  return <div className="wechat-actions"><button className="primary" type="button" disabled={copying} onClick={() => void copy()}><Clipboard size={16} />{copying ? "正在复制…" : hasCopied ? "已复制" : "复制格式化内容"}</button>{hasCopied && <button className="secondary" type="button" onClick={() => void onConfirm()}><Check size={16} />草稿已保存</button>}{error && <span className="clipboard-error" role="alert">无法写入剪贴板。<button type="button">查看 HTML</button></span>}</div>;
}
