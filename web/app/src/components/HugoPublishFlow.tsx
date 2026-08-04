import { Check, CloudUpload, FileText, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { confirmHugoPreview, createHugoPreview, getHugoPreview, getHugoSections, getJob, getPublicationWorkflow } from "../api/client";
import type { HugoPreviewView, HugoSectionView, PublicationFailureView, PublicationWorkflowView, RecoveredHugoPreviewView } from "../api/types";
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
  const [directory, setDirectory] = useState("");
  const [preview, setPreview] = useState<HugoPreviewView | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [workflow, setWorkflow] = useState<PublicationWorkflowView["hugo"]>(null);
  const [filesystemStale, setFilesystemStale] = useState(false);
  onPublishedRef.current = onPublished;

  const showError = useCallback((reason: unknown) => {
    // 页面切换或开发模式重挂载会主动取消旧请求，这不是发布失败。
    if (reason instanceof DOMException && reason.name === "AbortError") return;
    if (mounted.current) toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "Hugo 发布操作失败" });
  }, [toast]);

  /** loadSections 重新扫描 Hugo content 目录，并为恢复预览补齐当前选择。 */
  const loadSections = useCallback(async (signal?: AbortSignal) => {
    const value = await getHugoSections(articleID, signal);
    if (!mounted.current) return value;
    setDiscovery(value);
    if (value.selection_locked) {
      setSection(value.existing_section);
      setDirectory(value.existing_directory ?? "");
    } else if (value.sections.length === 1) {
      setSection(value.sections[0].name);
      const directories = value.sections[0].directories ?? [];
      setDirectory(directories.length === 1 ? directories[0].path : "");
    }
    return value;
  }, [articleID]);

  useEffect(() => {
    mounted.current = true;
    controller.current = new AbortController();
    const pollWorkflow = async () => {
      const value = await getPublicationWorkflow(articleID, controller.current?.signal);
      if (!mounted.current) return;
      setWorkflow(value.hugo);
      if (value.hugo?.preview) setPreview(recoveredPreview(value.hugo.preview));
      // 已同步状态也要重新扫描真实目录；外部删除 Bundle 后不能继续相信数据库状态。
      let currentDiscovery: HugoSectionView | null = null;
      if (!value.hugo || value.hugo.state === "failed" || value.hugo.state === "expired" || value.hugo.state === "published") {
        currentDiscovery = await loadSections(controller.current?.signal);
      }
      if (value.hugo?.state === "published" && currentDiscovery && !currentDiscovery.selection_locked) {
        // 数据库仍有发布记录，但真实 Bundle 已不存在，切换到可重新生成预览的状态。
        setWorkflow(null);
        setPreview(null);
        setFilesystemStale(true);
        setLoading(false);
        return;
      }
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
  }, [articleID, loadSections, showError]);

  const pollPreview = async (previewID: string) => {
    const next = await getHugoPreview(previewID);
    if (!mounted.current) return;
    setPreview(next);
    if (next.state === "preparing") timer.current = window.setTimeout(() => void pollPreview(previewID).catch(showError), 800);
    if (next.state === "failed") toast.show({ kind: "error", message: next.error || "Hugo 发布预览生成失败" });
  };
  const prepare = async () => {
    if (busy) return;
    setBusy(true);
    setPreview(null);
    setWorkflow(null);
    try {
      // 过期/失败预览可能没有完成目录发现，点击重试时主动刷新一次并使用刷新结果继续提交。
      const currentDiscovery = await loadSections();
      const availableSection = currentDiscovery.sections.some((item) => item.name === section)
        ? section
        : currentDiscovery.existing_section || (currentDiscovery.sections.length === 1 ? currentDiscovery.sections[0].name : "");
      const availableDirectories = currentDiscovery.sections.find((item) => item.name === availableSection)?.directories ?? [];
      const availableDirectory = availableDirectories.some((item) => item.path === directory)
        ? directory
        : currentDiscovery.existing_directory || (availableDirectories.length === 1 ? availableDirectories[0].path : "");
      if (!availableSection) throw new Error("未发现可用的 Hugo 发布目录，请检查 Hugo 根目录后重试");
      if (availableDirectories.length > 0 && !availableDirectory) throw new Error("请选择 Hugo 分类目录后重试");
      setSection(availableSection);
      setDirectory(availableDirectory);
      const refreshKey = filesystemStale ? `${Date.now()}` : "";
      const queued = await createHugoPreview(articleID, contentHash, availableSection, availableDirectory, refreshKey);
      setFilesystemStale(false);
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
  if (workflow && !preview && workflow.state !== "failed") return <section className="hugo-publish-flow" aria-live="polite"><p className="hugo-preview-state"><LoaderCircle className="spin" size={16} />{workflow.stage} · {workflow.progress}%</p>{workflow.error && <p className="hugo-flow-error">{workflow.error}</p>}</section>;
  if (!discovery && !preview) return <section className="hugo-publish-flow" aria-live="polite"><LoaderCircle className="spin" size={16} />正在读取 Hugo 发布目录…</section>;
  if (discovery && discovery.sections.length === 0 && !preview) return <section className="hugo-publish-flow"><b>Hugo 中还没有可用发布目录</b><p>请先在 Hugo content 中创建一级目录，再重新读取目录。</p><button type="button" className="secondary compact-button" onClick={() => void loadSections().catch(showError)}>重新读取 Hugo 目录</button></section>;
  const failure = preview?.failure ?? workflow?.failure ?? fallbackFailure(preview?.error || workflow?.error);
  const failed = preview?.state === "failed" || workflow?.state === "failed";
  const directories = discovery?.sections.find((item) => item.name === section)?.directories ?? [];
  const targetReady = Boolean(section) && (directories.length === 0 || Boolean(directory));
  return <section className="hugo-publish-flow" aria-label="Hugo 发布">
    {filesystemStale && <p className="hugo-flow-error">检测到 Hugo 原发布目录已不存在，将按当前文章重新生成。</p>}
    {discovery && <label>发布目录<select aria-label="发布目录" value={section} disabled={discovery.selection_locked || busy} onChange={(event) => { const next = event.target.value; setSection(next); const nextDirectories = discovery.sections.find((item) => item.name === next)?.directories ?? []; setDirectory(nextDirectories.length === 1 ? nextDirectories[0].path : ""); }}><option value="">请选择</option>{discovery.sections.map((item) => <option key={item.name} value={item.name}>{item.name}（{item.article_count} 篇）</option>)}</select></label>}
    {directories.length > 0 && <label>分类目录<select aria-label="分类目录" value={directory} disabled={discovery?.selection_locked || busy} onChange={(event) => setDirectory(event.target.value)}><option value="">请选择</option>{directories.map((item) => <option key={item.path} value={item.path}>{item.path}（{item.article_count} 篇）</option>)}</select></label>}
    {discovery?.selection_locked && <p className="hugo-section-lock">已有文章将继续更新 {discovery.existing_section}{discovery.existing_directory ? `/${discovery.existing_directory}` : ""}</p>}
    {failed && failure && <PublicationFailure failure={failure} />}
    {!preview && <button type="button" className="primary compact-button" disabled={!targetReady || busy} onClick={() => void prepare()}>{busy ? <LoaderCircle className="spin" size={16} /> : <CloudUpload size={16} />}{failed ? "重新生成预览" : "生成发布预览"}</button>}
    {preview?.state === "preparing" && <p className="hugo-preview-state"><LoaderCircle className="spin" size={16} />正在构建真实 Hugo 预览…</p>}
    {preview?.state === "failed" && <button type="button" className="primary compact-button" disabled={busy} onClick={() => void prepare()}>{busy ? <LoaderCircle className="spin" size={16} /> : <CloudUpload size={16} />}重新生成预览</button>}
    {preview && (preview.state === "ready" || preview.state === "expired") && <div className="hugo-artifact-summary">
      <div><span>{preview.change === "added" ? "新增" : "更新"}</span><code>{preview.target_path}</code></div>
      <ul>{preview.files.map((file) => <li key={file.relative_path}><FileText size={14} /><span>{file.relative_path}</span><small>{formatBytes(file.size)}</small></li>)}</ul>
      {preview.diagnostics.map((item) => <p key={item.code} className={`diagnostic-${item.level}`}>{item.message}</p>)}
      {discovery && discovery.sections.length === 0 && <p className="diagnostic-blocking">当前 Hugo content 目录未发现可用发布目录，请恢复文件夹后重新生成预览。</p>}
      <div className="hugo-artifact-actions"><button type="button" className="secondary compact-button" disabled={busy} onClick={() => void prepare()}>重新生成预览</button>{preview.state === "ready" && <button type="button" className="primary compact-button" disabled={busy} onClick={() => void confirm()}>{busy ? <LoaderCircle className="spin" size={16} /> : <Check size={16} />}确认同步到 Hugo</button>}</div>
    </div>}
  </section>;
}

function recoveredPreview(value: RecoveredHugoPreviewView): HugoPreviewView {
  return { id: value.preview_id, content_hash: "", section: value.section, target_path: value.target_path, change: value.change, files: value.files, diagnostics: value.diagnostics, preview_url: value.preview_url, expires_at: value.expires_at, state: value.state, job_id: "", error: value.error, failure: value.failure };
}

function PublicationFailure({ failure }: { failure: PublicationFailureView }) {
  return <div className="hugo-failure" role="alert"><strong>失败阶段：{failureStageLabel(failure.stage)}</strong><p><b>原因</b>{failure.message}</p><p><b>下一步</b>{failure.action}</p></div>;
}

function fallbackFailure(message?: string): PublicationFailureView | undefined {
  return message ? { stage: "prepare", code: "hugo.preview_failed", message, action: "检查发布历史和 Hugo 配置后重新生成预览", retryable: true } : undefined;
}

function failureStageLabel(stage: string) {
  if (stage === "preflight") return "发布前检查";
  if (stage === "deliver") return "更新 Hugo 内容";
  return "生成发布预览";
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KB`;
}
