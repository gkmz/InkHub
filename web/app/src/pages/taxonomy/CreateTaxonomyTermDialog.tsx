import { Check, Eye, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { APIError, applyTaxonomyTerm, previewTaxonomyTerm } from "../../api/client";
import type { TaxonomyChangePreview, TaxonomyOverview, TaxonomyTermCommand } from "../../api/types";
import { useToast } from "../../components/toast";

interface CreateTaxonomyTermDialogProps {
  overview: TaxonomyOverview;
  kind?: "category" | "series";
  noun?: "类目" | "系列";
  onClose: () => void;
  onApplied: (overview: TaxonomyOverview) => void;
  onCreated?: (name: string) => void;
}

/** CreateTaxonomyTermDialog 在写入 Hugo 前强制展示 Provider 生成的原生文件变更。 */
export function CreateTaxonomyTermDialog({ overview, kind = "category", noun = "类目", onClose, onApplied, onCreated }: CreateTaxonomyTermDialogProps) {
  const toast = useToast();
  const nameInput = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [preview, setPreview] = useState<TaxonomyChangePreview | null>(null);
  const [busy, setBusy] = useState<"preview" | "apply" | null>(null);

  useEffect(() => { nameInput.current?.focus(); }, []);
  useEffect(() => {
    // 文件变更正在生成或应用时禁止关闭，避免用户误以为后台操作已取消。
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape" && !busy) onClose(); };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [busy, onClose]);
  const command = (): TaxonomyTermCommand => ({
    provider_id: overview.provider_id ?? "",
    kind,
    name: name.trim(),
    description: description.trim(),
    aliases: [],
    expected_revision: preview?.expected_revision ?? overview.revision ?? "",
  });
  const showError = (error: unknown) => toast.show({ kind: "error", message: error instanceof APIError ? error.message : "操作失败，请稍后重试" });

  async function loadPreview() {
    if (!name.trim()) return;
    setBusy("preview");
    try { setPreview(await previewTaxonomyTerm(command())); } catch (error) { showError(error); } finally { setBusy(null); }
  }

  async function applyChange() {
    if (!preview) return;
    setBusy("apply");
    try {
      const next = await applyTaxonomyTerm(command());
      onApplied(next);
      onCreated?.(name.trim());
      toast.show({ kind: "success", message: `${noun}已创建` });
      onClose();
    } catch (error) { showError(error); } finally { setBusy(null); }
  }

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose(); }}>
    <section className="taxonomy-dialog" role="dialog" aria-modal="true" aria-labelledby="create-taxonomy-title">
      <header><div><p className="eyebrow">Hugo {kind === "category" ? "categories" : "series"}</p><h2 id="create-taxonomy-title">新建{noun}</h2></div><button type="button" aria-label="关闭" disabled={Boolean(busy)} onClick={onClose}><X size={18} /></button></header>
      <div className="taxonomy-form">
        <label htmlFor="taxonomy-name">{noun}名称</label>
        <input ref={nameInput} id="taxonomy-name" maxLength={160} value={name} onChange={(event) => { setName(event.target.value); setPreview(null); }} />
        <label htmlFor="taxonomy-description">{noun}说明</label>
        <textarea id="taxonomy-description" maxLength={1000} rows={3} value={description} onChange={(event) => { setDescription(event.target.value); setPreview(null); }} />
      </div>
      {preview && <div className="taxonomy-change-preview"><p>将写入 {preview.files.length} 个 Hugo 文件</p>{preview.files.map((file) => <article key={file.relative_path}><strong>{file.relative_path}</strong><pre><code>{file.after}</code></pre></article>)}</div>}
      <footer><button className="secondary" type="button" disabled={Boolean(busy)} onClick={onClose}>取消</button>{preview ? <button className="primary" type="button" disabled={Boolean(busy)} onClick={() => void applyChange()}><Check size={16} />{busy === "apply" ? `正在创建${noun}…` : `确认创建${noun}`}</button> : <button className="primary" type="button" disabled={!name.trim() || Boolean(busy)} onClick={() => void loadPreview()}><Eye size={16} />{busy === "preview" ? "正在生成…" : "预览变更"}</button>}</footer>
    </section>
  </div>;
}
