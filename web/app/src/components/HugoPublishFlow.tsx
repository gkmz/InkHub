import { Check, CloudUpload, FileText, LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { confirmHugoPreview, createHugoPreview, getHugoPreview, getHugoSections, getJob } from "../api/client";
import type { HugoPreviewView, HugoSectionView } from "../api/types";
import { useToast } from "./toast";

interface HugoPublishFlowProps {
  articleID: string;
  contentHash: string;
  onPublished: () => void | Promise<void>;
}

/** HugoPublishFlow 管理 Section 选择、Artifact 预览和确认交付闭环。 */
export function HugoPublishFlow({ articleID, contentHash, onPublished }: HugoPublishFlowProps) {
  const toast = useToast();
  const mounted = useRef(true);
  const [discovery, setDiscovery] = useState<HugoSectionView | null>(null);
  const [section, setSection] = useState("");
  const [preview, setPreview] = useState<HugoPreviewView | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void getHugoSections(articleID, controller.signal).then((value) => {
      if (!mounted.current) return;
      setDiscovery(value);
      if (value.selection_locked) setSection(value.existing_section);
      else if (value.sections.length === 1) setSection(value.sections[0].name);
    }).catch((reason: unknown) => {
      if (!(reason instanceof DOMException && reason.name === "AbortError")) toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "Hugo 目录读取失败" });
    });
    return () => { mounted.current = false; controller.abort(); };
  }, [articleID, toast]);

  const pollPreview = async (previewID: string) => {
    const next = await getHugoPreview(previewID);
    if (!mounted.current) return;
    setPreview(next);
    if (next.state === "preparing") window.setTimeout(() => void pollPreview(previewID).catch(showError), 800);
    if (next.state === "failed") toast.show({ kind: "error", message: next.error || "Hugo 发布预览生成失败" });
  };
  const showError = (reason: unknown) => {
    if (mounted.current) toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "Hugo 发布操作失败" });
  };
  const prepare = async () => {
    if (!section || busy) return;
    setBusy(true);
    setPreview(null);
    try {
      const queued = await createHugoPreview(articleID, contentHash, section);
      await pollPreview(queued.id);
    } catch (reason) {
      showError(reason);
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  const confirm = async () => {
    if (!preview || preview.state !== "ready" || busy) return;
    setBusy(true);
    try {
      const delivery = await confirmHugoPreview(preview.id);
      let job = await getJob(delivery.job_id);
      while (mounted.current && (job.state === "queued" || job.state === "running")) {
        await new Promise((resolve) => window.setTimeout(resolve, 800));
        job = await getJob(delivery.job_id);
      }
      if (job.state !== "succeeded") throw new Error("Hugo 同步失败，请检查诊断后重试");
      toast.show({ kind: "success", message: "文章已同步到 Hugo" });
      await onPublished();
    } catch (reason) {
      showError(reason);
    } finally {
      if (mounted.current) setBusy(false);
    }
  };

  if (!discovery) return <section className="hugo-publish-flow" aria-live="polite"><LoaderCircle className="spin" size={16} />正在读取 Hugo 发布目录…</section>;
  if (discovery.sections.length === 0) return <section className="hugo-publish-flow"><b>Hugo 中还没有可用发布目录</b><p>请先在 Hugo content 中创建一级目录，再重新打开发布流程。</p></section>;
  return <section className="hugo-publish-flow" aria-label="Hugo 发布">
    <label>发布目录<select aria-label="发布目录" value={section} disabled={discovery.selection_locked || busy} onChange={(event) => setSection(event.target.value)}><option value="">请选择</option>{discovery.sections.map((item) => <option key={item.name} value={item.name}>{item.name}（{item.article_count} 篇）</option>)}</select></label>
    {discovery.selection_locked && <p className="hugo-section-lock">已有文章将继续更新 {discovery.existing_section}</p>}
    {!preview && <button type="button" className="primary compact-button" disabled={!section || busy} onClick={() => void prepare()}>{busy ? <LoaderCircle className="spin" size={16} /> : <CloudUpload size={16} />}生成发布预览</button>}
    {preview?.state === "preparing" && <p className="hugo-preview-state"><LoaderCircle className="spin" size={16} />正在构建真实 Hugo 预览…</p>}
    {preview && (preview.state === "ready" || preview.state === "expired") && <div className="hugo-artifact-summary">
      <div><span>{preview.change === "added" ? "新增" : "更新"}</span><code>{preview.target_path}</code></div>
      <ul>{preview.files.map((file) => <li key={file.relative_path}><FileText size={14} /><span>{file.relative_path}</span><small>{formatBytes(file.size)}</small></li>)}</ul>
      {preview.diagnostics.map((item) => <p key={item.code} className={`diagnostic-${item.level}`}>{item.message}</p>)}
      {preview.state === "expired" ? <button type="button" className="secondary compact-button" disabled={busy} onClick={() => void prepare()}>重新生成预览</button> : <button type="button" className="primary compact-button" disabled={busy} onClick={() => void confirm()}>{busy ? <LoaderCircle className="spin" size={16} /> : <Check size={16} />}确认同步到 Hugo</button>}
    </div>}
  </section>;
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KB`;
}
