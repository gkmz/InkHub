import { Check, ChevronLeft, FolderOpen, Sparkles } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { DirectoryCandidate, WorkspaceDraft } from "../../api/types";
import { inspectDirectories, pickDirectory } from "../../api/client";
import { ContentScopePicker } from "../../components/ContentScopePicker";

const steps = ["选择内容库", "确认博客", "配置微信", "AI 与扫描"];

/** SetupPage 将首次初始化限制为四个连续、可恢复的决定。 */
export function SetupPage({ onComplete }: { onComplete: (draft: WorkspaceDraft) => Promise<void> }) {
  const saved = sessionStorage.getItem("inkhub.setup");
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<WorkspaceDraft>(() => {
    const restored = saved ? JSON.parse(saved) as Partial<WorkspaceDraft> : {};
    return {
      name: restored.name ?? "",
      vault_path: restored.vault_path ?? "",
      hugo_path: restored.hugo_path,
      wechat_template: restored.wechat_template ?? "default",
      ai_enabled: restored.ai_enabled ?? false,
      content_roots: restored.content_roots ?? [],
      ignored_folders: restored.ignored_folders ?? [],
      ignored_file_names: restored.ignored_file_names ?? ["index.md", "_index.md"],
    };
  });
  const [submitting, setSubmitting] = useState(false);
  const [pathError, setPathError] = useState("");
  const [hugoPathError, setHugoPathError] = useState("");
  const [directories, setDirectories] = useState<DirectoryCandidate[]>([]);
  const [inspecting, setInspecting] = useState(false);
  const valid = draft.name.trim() !== "" && draft.vault_path.trim() !== "" && draft.content_roots.length > 0;
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
  const next = () => setStep((current) => Math.min(3, current + 1));
  const loadDirectories = async (vaultPath: string) => {
    setInspecting(true);
    setPathError("");
    try {
      const result = await inspectDirectories(vaultPath);
      setDirectories(result.directories);
    } catch (reason) {
      setPathError(reason instanceof Error ? reason.message : "无法读取目录");
    } finally {
      setInspecting(false);
    }
  };

  return (
    <main className="setup-page">
      <aside className="setup-rail">
        <div className="brand setup-brand"><span className="brand-mark">I</span><strong>InkHub</strong></div>
        <ol>{steps.map((label, index) => <li key={label} className={index === step ? "active" : index < step ? "done" : ""}><span>{index < step ? <Check size={14} /> : index + 1}</span>{label}</li>)}</ol>
        <p>文章留在你的电脑上<br />InkHub 只建立本地索引</p>
      </aside>
      <section className="setup-content">
        <div className="setup-step"><span>{step + 1} / 4</span><p>{steps.join(" · ")}</p></div>
        <div className="setup-form">
          <p className="eyebrow">首次设置</p><h1>{heading}</h1>
          {step === 0 && <>
            <p className="lead">先告诉 InkHub 你的文章放在哪里。内容不会被搬走或上传。</p>
            <label>工作区名称<input value={draft.name} onChange={(event) => update({ name: event.target.value })} placeholder="例如：我的文章" /></label>
            <label>Obsidian Vault 路径<div className="path-field"><input value={draft.vault_path} onChange={(event) => { setPathError(""); setDirectories([]); update({ vault_path: event.target.value, content_roots: [], ignored_folders: [] }); }} placeholder="/Users/you/Documents/Vault" /><button type="button" aria-label="选择目录" onClick={() => { setPathError(""); pickDirectory("vault").then(({ path }) => { update({ vault_path: path, content_roots: [], ignored_folders: [] }); return loadDirectories(path); }).catch((reason: Error) => setPathError(reason.message)); }}><FolderOpen size={18} /></button></div></label>
            {pathError && <p className="field-error" role="alert">{pathError}，你仍可手工输入路径。</p>}
            {draft.vault_path && directories.length === 0 && <button className="secondary inspect-directories" type="button" disabled={inspecting} onClick={() => loadDirectories(draft.vault_path)}>{inspecting ? "正在读取…" : "读取目录"}</button>}
            {directories.length > 0 && <ContentScopePicker directories={directories} contentRoots={draft.content_roots} ignoredFolders={draft.ignored_folders} ignoredFileNames={draft.ignored_file_names} onChange={(content_roots, ignored_folders, ignored_file_names) => update({ content_roots, ignored_folders, ignored_file_names })} />}
            <button className="primary" type="button" disabled={!valid} onClick={next}>继续</button>
          </>}
          {step === 1 && <>
            <p className="lead">连接 Hugo 后可以把审核完成的文章同步到博客。这一步可以稍后完成。</p>
            <label>Hugo 根目录<div className="path-field"><input value={draft.hugo_path ?? ""} onChange={(event) => { setHugoPathError(""); update({ hugo_path: event.target.value }); }} placeholder="可选" /><button type="button" aria-label="选择 Hugo 目录" onClick={() => { setHugoPathError(""); pickDirectory("hugo").then(({ path }) => update({ hugo_path: path })).catch((reason: Error) => setHugoPathError(reason.message)); }}><FolderOpen size={18} /></button></div></label>
            {hugoPathError && <p className="field-error" role="alert">{hugoPathError}，你仍可手工输入路径。</p>}
            <div className="button-row"><button className="secondary" type="button" onClick={next}>暂不配置博客</button><button className="primary" type="button" onClick={next}>继续</button></div>
          </>}
          {step === 2 && <>
            <p className="lead">选择微信公众号排版模板。图片托管可以稍后在设置中配置。</p>
            <div className="template-options">
              {[{ value: "default", name: "InkHub Default", description: "清晰层级，适合技术文章" }, { value: "minimal", name: "InkHub Minimal", description: "轻量留白，适合短文" }, { value: "classic", name: "InkHub Classic（原版）", description: "绿色强调，保留原版排版" }].map((template) => <button key={template.value} type="button" className={draft.wechat_template === template.value ? "selected" : ""} onClick={() => update({ wechat_template: template.value })}><span className={`template-preview ${template.value}`}><i /><i /><i /></span><b>{template.name}</b><small>{template.description}</small></button>)}
            </div>
            <button className="primary" type="button" onClick={next}>继续</button>
          </>}
          {step === 3 && <>
            <p className="lead">AI 默认关闭。不开启也能完整审核和发布文章。</p>
            <label className="toggle-row"><span><Sparkles size={18} /><b>启用 AI 建议</b><small>仅在你主动请求时发送内容</small></span><input type="checkbox" checked={draft.ai_enabled} onChange={(event) => update({ ai_enabled: event.target.checked })} /></label>
            <div className="setup-summary"><Check size={18} /><span><b>准备扫描 {draft.name}</b><small>{draft.vault_path}</small></span></div>
            <button className="primary" type="button" disabled={submitting} onClick={async () => { setSubmitting(true); await onComplete(draft); setSubmitting(false); }}>{submitting ? "正在创建…" : "创建工作区"}</button>
          </>}
          {step > 0 && <button className="back" type="button" onClick={() => setStep((current) => current - 1)}><ChevronLeft size={16} />返回上一步</button>}
        </div>
      </section>
    </main>
  );
}
