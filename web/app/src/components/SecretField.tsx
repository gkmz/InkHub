import { KeyRound, Trash2 } from "lucide-react";
import { useState } from "react";

/** SecretField 只显示是否已保存，输入框永远不回显 Secret。 */
export function SecretField({ label, saved }: { label: string; saved: boolean }) {
  const [value, setValue] = useState("");
  return <label className="secret-field"><span><KeyRound size={15} />{label}<small>{saved ? "已安全保存" : "尚未保存"}</small></span><div><input type="password" aria-label={label} value={value} onChange={(event) => setValue(event.target.value)} placeholder={saved ? "输入新值以替换" : "输入 Secret"} autoComplete="new-password" />{saved && <button type="button" aria-label={`删除 ${label}`}><Trash2 size={15} /></button>}</div></label>;
}
