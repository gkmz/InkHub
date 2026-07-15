import { Activity, Bot, Database, Globe2, RefreshCw, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { getSettings, previewContentScope, saveContentScope } from "../../api/client";
import type { SettingsView } from "../../api/types";
import { ContentScopePicker } from "../../components/ContentScopePicker";
import { SecretField } from "../../components/SecretField";
import { TemplatePicker } from "../../components/TemplatePicker";
import { useToast } from "../../components/toast";

/** SettingsPage 按工作区、AI、发布渠道和诊断分组保存设置。 */
export function SettingsPage() {
  const toast = useToast();
  const [settings, setSettings] = useState<SettingsView | null>(null);
  const [scopeResult, setScopeResult] = useState("");
  const [savingScope, setSavingScope] = useState(false);
  const [scopePreview, setScopePreview] = useState<{ added: number; removed: number } | null>(null);
  const [diagnosing, setDiagnosing] = useState(false);
  useEffect(() => { const controller = new AbortController(); void getSettings(controller.signal).then(setSettings); return () => controller.abort(); }, []);
  if (!settings) return <div className="page-state">正在读取设置…</div>;
  const updateScope = (content_roots: string[], ignored_folders: string[], ignored_file_names: string[]) => {
    setScopePreview(null);
    setScopeResult("");
    setSettings({ ...settings, content_roots, ignored_folders, ignored_file_names });
  };
  const inspectScopeChange = async () => {
    setSavingScope(true);
    setScopeResult("");
    try {
      setScopePreview(await previewContentScope(settings.content_roots, settings.ignored_folders, settings.ignored_file_names));
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "无法预览变更";
      setScopeResult(message);
      toast.show({ kind: "error", message });
    } finally {
      setSavingScope(false);
    }
  };
  const persistScope = async () => {
    setSavingScope(true);
    setScopeResult("");
    try {
      const result = await saveContentScope(settings.content_roots, settings.ignored_folders, settings.ignored_file_names);
      setScopeResult(`已索引 ${result.indexed} 篇，失败 ${result.failed} 篇`);
      setScopePreview(null);
      toast.show({ kind: result.failed > 0 ? "info" : "success", message: "内容范围已保存并完成重扫" });
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "保存失败";
      setScopeResult(message);
      toast.show({ kind: "error", message });
    } finally {
      setSavingScope(false);
    }
  };
  const refreshDiagnostics = async () => {
    setDiagnosing(true);
    try {
      const refreshed = await getSettings();
      // 重新诊断只更新检查结果，不能覆盖用户尚未保存的目录选择。
      setSettings((current) => current ? { ...current, diagnostics: refreshed.diagnostics } : refreshed);
      toast.show({ kind: "success", message: "诊断已更新" });
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "重新诊断失败";
      toast.show({ kind: "error", message });
    } finally {
      setDiagnosing(false);
    }
  };
  return <div className="settings-page">
    <SettingsSection icon={<Database />} title="工作区" description="内容位置和本地索引">
      <label>工作区名称<input value={settings.workspace_name} readOnly /></label>
      <label>Obsidian Vault<input value={settings.vault_path} readOnly /></label>
      <ContentScopePicker directories={settings.directories} contentRoots={settings.content_roots} ignoredFolders={settings.ignored_folders} ignoredFileNames={settings.ignored_file_names} onChange={updateScope} />
      {!scopePreview && <button className="secondary" disabled={savingScope || settings.content_roots.length === 0} onClick={inspectScopeChange}><Save size={15} />{savingScope ? "正在计算…" : "预览内容范围变更"}</button>}
      {scopePreview && <div className="scope-confirm"><p>将新增 {scopePreview.added} 篇，移出 {scopePreview.removed} 篇。源文件不会被修改。</p><button className="primary" disabled={savingScope} onClick={persistScope}>{savingScope ? "正在保存…" : "确认并重扫"}</button><button className="secondary" onClick={() => setScopePreview(null)}>取消</button></div>}
      {scopeResult && <p className="inline-status" role="status">{scopeResult}</p>}
    </SettingsSection>
    <SettingsSection icon={<Bot />} title="AI 建议" description="只在主动请求时发送内容"><label className="switch-line">启用 AI 建议<input type="checkbox" defaultChecked={settings.ai_enabled} /></label><SecretField label="AI API Key" saved={settings.ai_secret_saved} onDelete={() => toast.show({ kind: "info", message: "AI API Key 删除功能尚未开放" })} /><button className="secondary" onClick={() => toast.show({ kind: "info", message: "AI 设置保存功能尚未开放" })}><Save size={15} />保存 AI 设置</button></SettingsSection>
    <SettingsSection icon={<Globe2 />} title="发布渠道" description="Hugo 与微信公众号"><h3>微信公众号模板</h3><TemplatePicker value={settings.default_template} templates={settings.templates} onChange={(id) => setSettings({ ...settings, default_template: id })} /><SecretField label="图片托管 Token" saved={settings.wechat_secret_saved} onDelete={() => toast.show({ kind: "info", message: "图片托管 Token 删除功能尚未开放" })} /><button className="secondary" onClick={() => toast.show({ kind: "info", message: "发布设置保存功能尚未开放" })}><Save size={15} />保存发布设置</button></SettingsSection>
    <SettingsSection icon={<Activity />} title="数据与诊断" description="检查本机依赖和工作区状态"><div className="diagnostic-list">{settings.diagnostics.map((item) => <div key={item.name}><span className={`doctor-${item.state}`}>{item.state}</span><b>{item.name}</b><p>{item.message}</p></div>)}</div><button className="secondary" disabled={diagnosing} onClick={refreshDiagnostics}><RefreshCw size={15} />{diagnosing ? "正在诊断…" : "重新诊断"}</button></SettingsSection>
  </div>;
}

function SettingsSection({ icon, title, description, children }: { icon: React.ReactNode; title: string; description: string; children: React.ReactNode }) {
  return <section className="settings-section"><header>{icon}<div><h2>{title}</h2><p>{description}</p></div></header><div className="settings-content">{children}</div></section>;
}
