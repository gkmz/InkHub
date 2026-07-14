import { AlertTriangle, CheckCircle2, Info } from "lucide-react";
import type { CheckResult } from "../api/types";

/** Checks 按严重程度展示发布前检查及受影响渠道。 */
export function Checks({ items }: { items: CheckResult[] }) {
  return <section className="tool-section checks"><div className="tool-heading"><h2>检查结果</h2><span>{items.filter((item) => item.level !== "passed").length} 项待处理</span></div>{items.map((item) => <article key={item.id} className={`check-${item.level}`}>{item.level === "passed" ? <CheckCircle2 /> : item.level === "blocking" ? <AlertTriangle /> : <Info />}<div><b>{item.title}</b><p>{item.detail}</p><small>{item.channel}</small></div></article>)}</section>;
}
