import { Check, Circle, CircleAlert } from "lucide-react";

/** PublicationTrack 用自然语言展示审核到渠道交付的三段状态。 */
export function PublicationTrack({ review, hugo, wechat, xiaohongshu = "尚未准备" }: { review: string; hugo: string; wechat: string; xiaohongshu?: string }) {
  return <section className="publication-track" aria-label="发布进度">{[["审核", review], ["Hugo", hugo], ["微信", wechat], ["小红书", xiaohongshu]].map(([label, state], index) => <div key={label} className={state.includes("失败") ? "failed" : state.includes("已") ? "complete" : "pending"}>{index > 0 && <i />}<span>{state.includes("失败") ? <CircleAlert /> : state.includes("已") ? <Check /> : <Circle />}</span><b>{label}</b><small>{state}</small></div>)}</section>;
}
