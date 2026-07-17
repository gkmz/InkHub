import { Check, CloudUpload, FileText, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { confirmHugoPreview, createHugoPreview, getHugoPreview, getHugoSections, getJob, getPublicationWorkflow } from "../api/client";
import type { HugoPreviewView, HugoSectionView, PublicationWorkflowView, RecoveredHugoPreviewView } from "../api/types";
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
  const onPublishedRef = useRef(onPublished);
  const timer = useRef<number | null>(null);
  const controller = useRef<AbortController | null>(null);
  const [discovery, setDiscovery] = useState<HugoSectionView | null>(null);
  const [section, setSection] = useState("");
  const [preview, setPreview] = useState<HugoPreviewView | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [workflow, setWorkflow] = useState<PublicationWorkflowView["hugo"]>(null);
  onPublishedRef.current = onPublished;

  const showError = useCallback((reason: unknown) => {
    if (mounted.current) toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "Hugo 发布操作失败" });
  }, [toast]);

  useEffect(() => {
    mounted.current = true;
    controller.current = new AbortController();
    const loadSections = async () => {
      const value = await getHugoSections(articleID, controller.current?.signal);
      if (!mounted.current) return;
      setDiscovery(value);
      if (value.selection_locked) setSection(value.existing_section);
      else if (value.sections.length === 1) setSection(value.sections[0].name);
    };
    const pollWorkflow = async () => {
      const value = await getPublicationWorkflow(articleID, controller.current?.signal);
      if (!mounted.current) return;
      setWorkflow(value.hugo);
      if (value.hugo?.preview) setPreview(recoveredPreview(value.hugo.preview));
      if (!value.hugo) await loadSections();
      if (value.hugo?.state === "preparing" || value.hugo?.state === "delivering") timer.current = window.setTimeout(() => void pollWorkflow().catch(showError), 800);
      if (value.hugo?.state === "published") await onPublishedRef.current();
      setLoading(false);
    };
    void pollWorkflow().catch((reason: unknown) => { showError(reason); if (mounted.current) setLoading(false); });
    return () => {
      mounted.current = false;
      controller.current?.abort();
      if (timer.current !== null) window.clearTimeout(timer.current);
    };
  }, [articleID, showError]);

  const pollPreview = async (previewID: string) => {
    const next = await getHugoPreview(previewID);
    if (!mounted.current) return;
    setPreview(next);
    if (next.state === "preparing") timer.current = window.setTimeout(() => void pollPreview(previewID).catch(showError), 800);
    if (next.state === "failed") toast.show({ kind: "error", message: next.error || "Hugo 发布预览生成失败" });
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

  if (loading) return <section className="hugo-publish-flow" aria-live="polite"><LoaderCircle className="spin" size={16} />正在恢复 Hugo 发布状态…</section>;
  if (workflow && !preview) return <section className="hugo-publish-flow" aria-live="polite"><p className="hugo-preview-state"><LoaderCircle className="spin" size={16} />{workflow.stage} · {workflow.progress}%</p>{workflow.error && <p className="hugo-flow-error">{workflow.error}</p>}</section>;
  if (!discovery && !preview) return <section className="hugo-publish-flow" aria-live="polite"><LoaderCircle className="spin" size={16} />正在读取 Hugo 发布目录…</section>;
  if (discovery && discovery.sections.length === 0) return <section className="hugo-publish-flow"><b>Hugo 中还没有可用发布目录</b><p>请先在 Hugo content 中创建一级目录，再重新打开发布流程。</p></section>;
  return <section className="hugo-publish-flow" aria-label="Hugo 发布">
    {discovery && <label>发布目录<select aria-label="发布目录" value={section} disabled={discovery.selection_locked || busy} onChange={(event) => setSection(event.target.value)}><option value="">请选择</option>{discovery.sections.map((item) => <option key={item.name} value={item.name}>{item.name}（{item.article_count} 篇）</option>)}</select></label>}
    {discovery?.selection_locked && <p className="hugo-section-lock">已有文章将继续更新 {discovery.existing_section}</p>}
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

function recoveredPreview(value: RecoveredHugoPreviewView): HugoPreviewView {
  return { id: value.preview_id, content_hash: "", section: value.section, target_path: value.target_path, change: value.change, files: value.files, diagnostics: value.diagnostics, preview_url: value.preview_url, expires_at: value.expires_at, state: value.state, job_id: "", error: value.error };
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KB`;
}
