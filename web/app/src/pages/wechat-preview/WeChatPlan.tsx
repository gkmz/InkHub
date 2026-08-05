import { Check, Image, LoaderCircle } from "lucide-react";
import { useEffect, useState } from "react";
import { confirmWeChatPlan, getWeChatPlan } from "../../api/client";
import type { WeChatPlanView } from "../../api/types";
import type { MermaidTheme } from "../../api/types";
import { useToast } from "../../components/toast";

/** WeChatPlan 在任何外部写入前展示模板和本地图片清单。 */
export function WeChatPlan({ articleID, templateID, mermaidTheme, onConfirmed }: { articleID: string; templateID: string; mermaidTheme: MermaidTheme; onConfirmed: () => void }) {
  const toast = useToast();
  const [plan, setPlan] = useState<WeChatPlanView | null>(null);
  const [loading, setLoading] = useState(true);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setPlan(null);
    setError("");
    void getWeChatPlan(articleID, templateID, mermaidTheme, controller.signal).then(setPlan).catch((reason: unknown) => {
      if (!(reason instanceof DOMException && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "微信准备计划读取失败");
    }).finally(() => setLoading(false));
    return () => controller.abort();
  }, [articleID, mermaidTheme, templateID]);

  const confirm = async () => {
    if (!plan?.ready || confirming) return;
    setConfirming(true);
    setError("");
    try {
      await confirmWeChatPlan(articleID, plan.plan_token);
      toast.show({ kind: "success", message: "微信内容开始准备" });
      onConfirmed();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "微信内容准备失败");
    } finally {
      setConfirming(false);
    }
  };

  if (loading) return <section className="wechat-plan" aria-live="polite"><LoaderCircle className="spin" size={16} />正在检查微信图片…</section>;
  return <section className="wechat-plan" aria-label="微信准备计划">
    <h2>准备清单</h2>
    {plan && plan.images.length === 0 && <p><Image size={16} />本篇没有需要上传的本地图片</p>}
    {plan && plan.images.length > 0 && <ul>{plan.images.map((item) => <li key={item.reference}><Image size={15} /><span>{item.reference}<small>{item.media_type} · {formatBytes(item.size)}</small></span><b>{item.state === "reuse" ? "已存在，直接复用" : "将上传"}</b></li>)}</ul>}
    {plan?.diagnostics.map((item) => <p key={item.code} className="wechat-plan-error">{item.message}</p>)}
    {error && <p className="wechat-plan-error" role="alert">{error}</p>}
    <button className="primary" type="button" disabled={!plan?.ready || confirming} onClick={() => void confirm()}>{confirming ? <LoaderCircle className="spin" size={15} /> : <Check size={15} />}{confirming ? "正在提交…" : "确认并准备"}</button>
  </section>;
}

function formatBytes(value: number) {
  return value < 1024 ? `${value} B` : `${(value / 1024).toFixed(1)} KB`;
}
