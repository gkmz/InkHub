import { Check, ChevronLeft, FolderOpen } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { DirectoryCandidate, HugoSiteInspection, WorkspaceDraft } from "../../api/types";
import { inspectDirectories, inspectHugoDirectory, pickDirectory } from "../../api/client";
import { ContentScopePicker } from "../../components/ContentScopePicker";

const steps = ["选择 Vault", "管理范围", "连接 Hugo", "初始化"];

/** SetupPage 引导用户先确定 Vault，再授权内容范围，最后确认源文件初始化。 */
export function SetupPage({ onComplete }: { onComplete: (draft: WorkspaceDraft) => Promise<void> }) {
  const saved = sessionStorage.getItem("inkhub.setup");
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<WorkspaceDraft>(() => {
    const restored = saved ? JSON.parse(saved) as Partial<WorkspaceDraft> : {};
    return {
      name: restored.name ?? "",
      vault_path: restored.vault_path ?? "",
      hugo_path: restored.hugo_path ?? "",
      wechat_template: "default",
      ai_enabled: false,
      content_roots: restored.content_roots ?? [],
      ignored_folders: restored.ignored_folders ?? [],
      ignored_file_names: restored.ignored_file_names ?? ["index.md", "_index.md"],
    };
  });
  const [submitting, setSubmitting] = useState(false);
  const [pathError, setPathError] = useState("");
  const [hugoError, setHugoError] = useState("");
  const [directories, setDirectories] = useState<DirectoryCandidate[]>([]);
  const [hugoSite, setHugoSite] = useState<HugoSiteInspection | null>(null);
  const [inspecting, setInspecting] = useState(false);
  const [inspectingHugo, setInspectingHugo] = useState(false);
  const [submitError, setSubmitError] = useState("");
  const vaultReady = draft.name.trim() !== "" && draft.vault_path.trim() !== "" && directories.length > 0;
  const scopeReady = draft.content_roots.length > 0;
  const hugoReady = draft.hugo_path?.trim() !== "" && hugoSite?.root === draft.hugo_path;

  useEffect(() => {
    if (!draft.name && !draft.vault_path) return;
    const guard = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [draft.name, draft.vault_path]);

  const update = (next: Partial<WorkspaceDraft>) => {
    const value = { ...draft, ...next };
    setDraft(value);
    sessionStorage.setItem("inkhub.setup", JSON.stringify(value));
  };
  const heading = useMemo(() => steps[step], [step]);
  const loadDirectories = async (vaultPath: string) => {
    setInspecting(true);
    setPathError("");
    try {
      const result = await inspectDirectories(vaultPath);
      setDirectories(result.directories);
    } catch (reason) {
      setDirectories([]);
      setPathError(reason instanceof Error ? reason.message : "无法读取目录");
    } finally {
      setInspecting(false);
    }
  };
  const selectVault = async () => {
    setPathError("");
    try {
      const { path } = await pickDirectory("vault");
      update({ vault_path: path, content_roots: [], ignored_folders: [] });
      await loadDirectories(path);
    } catch (reason) {
      setPathError(reason instanceof Error ? reason.message : "无法选择 Vault");
    }
  };
  const loadHugoSite = async (hugoPath: string) => {
    setInspectingHugo(true);
    setHugoError("");
    try {
      const result = await inspectHugoDirectory(hugoPath);
      setHugoSite(result);
      update({ hugo_path: result.root });
    } catch (reason) {
      setHugoSite(null);
      setHugoError(reason instanceof Error ? reason.message : "无法读取 Hugo 站点");
    } finally {
      setInspectingHugo(false);
    }
  };
  const selectHugo = async () => {
    setHugoError("");
    try {
      const { path } = await pickDirectory("hugo");
      update({ hugo_path: path });
      await loadHugoSite(path);
    } catch (reason) {
      setHugoSite(null);
      setHugoError(reason instanceof Error ? reason.message : "无法选择 Hugo 目录");
    }
  };

  const backButton = step > 0 ? <button className="back" type="button" onClick={() => setStep((current) => current - 1)}><ChevronLeft size={16} />返回上一步</button> : <span />;

  return <main className="setup-page">
    <aside className="setup-rail">
      <div className="brand setup-brand"><span className="brand-mark">I</span><strong>InkHub</strong></div>
      <ol>{steps.map((label, index) => <li key={label} className={index === step ? "active" : index < step ? "done" : ""}><span>{index < step ? <Check size={14} /> : index + 1}</span>{label}</li>)}</ol>
      <p>文章始终保留在本机<br />初始化只补充发布所需身份</p>
    </aside>
    <section className="setup-content">
      <div className="setup-step"><span>{step + 1} / 4</span><p>{steps.join(" · ")}</p></div>
      <div className="setup-form">
        <p className="eyebrow">首次设置</p><h1>{heading}</h1>
        {step === 0 && <>
          <p className="lead">选择 Obsidian Vault。验证通过后，下一步再决定哪些目录交给 InkHub 管理。</p>
          <label>工作区名称<input value={draft.name} onChange={(event) => update({ name: event.target.value })} placeholder="例如：我的文章" /></label>
          <label>Obsidian Vault 路径<div className="path-field"><input value={draft.vault_path} onChange={(event) => { setDirectories([]); setPathError(""); update({ vault_path: event.target.value, content_roots: [], ignored_folders: [] }); }} placeholder="/Users/you/Documents/Vault" /><button type="button" aria-label="选择目录" onClick={selectVault}><FolderOpen size={18} /></button></div></label>
          {pathError && <p className="field-error" role="alert">{pathError}</p>}
          {draft.vault_path && directories.length === 0 && <button className="secondary inspect-directories" type="button" disabled={inspecting} onClick={() => loadDirectories(draft.vault_path)}>{inspecting ? "正在验证…" : "验证并读取目录"}</button>}
          <div className="setup-actions">{backButton}<button className="primary" type="button" disabled={!vaultReady} onClick={() => setStep(1)}>继续</button></div>
        </>}
        {step === 1 && <>
          <p className="lead">至少选择一个管理目录。InkHub 会递归纳入文章，并跳过你明确排除的目录和文件名。</p>
          <ContentScopePicker directories={directories} contentRoots={draft.content_roots} ignoredFolders={draft.ignored_folders} ignoredFileNames={draft.ignored_file_names} onChange={(content_roots, ignored_folders, ignored_file_names) => update({ content_roots, ignored_folders, ignored_file_names })} />
          <div className="setup-actions">{backButton}<button className="primary" type="button" disabled={!scopeReady} onClick={() => setStep(2)}>继续</button></div>
        </>}
        {step === 2 && <>
          <p className="lead">选择包含 Hugo 配置文件的博客根目录。InkHub 会读取配置并定位实际内容目录，不依赖环境变量。</p>
          <label>Hugo 根目录<div className="path-field"><input value={draft.hugo_path ?? ""} onChange={(event) => { setHugoSite(null); setHugoError(""); update({ hugo_path: event.target.value }); }} placeholder="选择包含 hugo.yaml 或 hugo.toml 的目录" /><button type="button" aria-label="选择 Hugo 目录" onClick={selectHugo}><FolderOpen size={18} /></button></div></label>
          {hugoError && <p className="field-error" role="alert">{hugoError}</p>}
          {draft.hugo_path && !hugoSite && <button className="secondary inspect-directories" type="button" disabled={inspectingHugo} onClick={() => loadHugoSite(draft.hugo_path ?? "")}>{inspectingHugo ? "正在检测…" : "检测 Hugo 站点"}</button>}
          {hugoSite && <div className="hugo-site-summary" role="status"><Check size={17} /><span><b>已识别 Hugo 站点</b><small>内容目录：{hugoSite.content_dir} · {hugoSite.sections.length} 个 Section</small></span></div>}
          <div className="setup-actions">{backButton}<button className="primary" type="button" disabled={!hugoReady} onClick={() => setStep(3)}>继续</button></div>
        </>}
        {step === 3 && <>
          <p className="lead">初始化会扫描已选目录。没有 frontmatter 的文章会创建 frontmatter；缺少 Stable ID 的文章会补充 `id`。已有字段和已有身份不会被覆盖。</p>
          <div className="setup-summary"><Check size={18} /><span><b>{draft.name}</b><small>{draft.content_roots.join("、")} · Hugo：{hugoSite?.content_dir}</small></span></div>
          {submitError && <p className="field-error" role="alert">{submitError}</p>}
          <div className="setup-actions">{backButton}<button className="primary" type="button" disabled={submitting} onClick={async () => { setSubmitting(true); setSubmitError(""); try { await onComplete(draft); } catch (reason) { setSubmitError(reason instanceof Error ? reason.message : "创建工作区失败，请重试"); } finally { setSubmitting(false); } }}>{submitting ? "正在初始化…" : "确认并初始化"}</button></div>
        </>}
      </div>
    </section>
  </main>;
}
