import { Activity, Bot, Database, Globe2, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { getSettings, saveContentScope } from "../../api/client";
import type { SettingsView } from "../../api/types";
import { ContentScopePicker } from "../../components/ContentScopePicker";
import { SecretField } from "../../components/SecretField";
import { TemplatePicker } from "../../components/TemplatePicker";

/** SettingsPage 按工作区、AI、发布渠道和诊断分组保存设置。 */
export function SettingsPage() {
  const [settings, setSettings] = useState<SettingsView | null>(null);
  const [scopeResult, setScopeResult] = useState("");
  const [savingScope, setSavingScope] = useState(false);
  useEffect(() => { const controller = new AbortController(); void getSettings(controller.signal).then(setSettings); return () => controller.abort(); }, []);
  if (!settings) return <div className="page-state">正在读取设置…</div>;
  const updateScope = (content_roots: string[], ignored_folders: string[]) => setSettings({ ...settings, content_roots, ignored_folders });
  const persistScope = async () => {
    setSavingScope(true);
    setScopeResult("");
    try {
      const result = await saveContentScope(settings.content_roots, settings.ignored_folders);
      setScopeResult(`已索引 ${result.indexed} 篇，失败 ${result.failed} 篇`);
    } catch (reason) {
      setScopeResult(reason instanceof Error ? reason.message : "保存失败");
    } finally {
      setSavingScope(false);
    }
  };
  return <div className="settings-page">
    <SettingsSection icon={<Database />} title="工作区" description="内容位置和本地索引">
      <label>工作区名称<input value={settings.workspace_name} readOnly /></label>
      <label>Obsidian Vault<input value={settings.vault_path} readOnly /></label>
      <ContentScopePicker directories={settings.directories} contentRoots={settings.content_roots} ignoredFolders={settings.ignored_folders} onChange={updateScope} />
      <button className="secondary" disabled={savingScope || settings.content_roots.length === 0} onClick={persistScope}><Save size={15} />{savingScope ? "正在保存…" : "保存内容范围"}</button>
      {scopeResult && <p className="inline-status" role="status">{scopeResult}</p>}
    </SettingsSection>
    <SettingsSection icon={<Bot />} title="AI 建议" description="只在主动请求时发送内容"><label className="switch-line">启用 AI 建议<input type="checkbox" defaultChecked={settings.ai_enabled} /></label><SecretField label="AI API Key" saved={settings.ai_secret_saved} /><button className="secondary"><Save size={15} />保存 AI 设置</button></SettingsSection>
    <SettingsSection icon={<Globe2 />} title="发布渠道" description="Hugo 与微信公众号"><h3>微信公众号模板</h3><TemplatePicker value={settings.default_template} templates={settings.templates} onChange={(id) => setSettings({ ...settings, default_template: id })} /><SecretField label="图片托管 Token" saved={settings.wechat_secret_saved} /><button className="secondary"><Save size={15} />保存发布设置</button></SettingsSection>
    <SettingsSection icon={<Activity />} title="数据与诊断" description="检查本机依赖和工作区状态"><div className="diagnostic-list">{settings.diagnostics.map((item) => <div key={item.name}><span className={`doctor-${item.state}`}>{item.state}</span><b>{item.name}</b><p>{item.message}</p></div>)}</div><button className="secondary">重新诊断</button></SettingsSection>
  </div>;
}

function SettingsSection({ icon, title, description, children }: { icon: React.ReactNode; title: string; description: string; children: React.ReactNode }) {
  return <section className="settings-section"><header>{icon}<div><h2>{title}</h2><p>{description}</p></div></header><div className="settings-content">{children}</div></section>;
}
