import { Activity, Bot, Database, Globe2, RefreshCw, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { getSettings, previewContentScope, saveAISettings, saveContentScope, saveWeChatSettings } from "../../api/client";
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
  const [aiKey, setAIKey] = useState("");
  const [savingAI, setSavingAI] = useState(false);
  const [githubToken, setGitHubToken] = useState("");
  const [savingWeChat, setSavingWeChat] = useState(false);
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
  const persistAI = async () => {
    setSavingAI(true);
    try {
      const result = await saveAISettings({ enabled: settings.ai_enabled, base_url: settings.ai_base_url ?? "", model: settings.ai_model ?? "", api_key: aiKey });
      setSettings({ ...settings, ...result });
      setAIKey("");
      toast.show({ kind: "success", message: "AI 设置已保存" });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "AI 设置保存失败" });
    } finally {
      setSavingAI(false);
    }
  };
  const persistWeChat = async () => {
    setSavingWeChat(true);
    try {
      const result = await saveWeChatSettings({
        enabled: settings.wechat_enabled, template: settings.default_template,
        github_owner: settings.github_owner ?? "", github_repository: settings.github_repository ?? "",
        github_branch: settings.github_branch ?? "main", github_prefix: settings.github_prefix ?? "inkhub", github_token: githubToken,
      });
      setSettings({ ...settings, ...result });
      setGitHubToken("");
      toast.show({ kind: "success", message: "发布设置已保存" });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "发布设置保存失败" });
    } finally {
      setSavingWeChat(false);
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
      {settings.obsidian_settings && <div className="obsidian-settings-summary"><h3>Obsidian 资源设置</h3><p>附件位置：{settings.obsidian_settings.attachment_location}</p><p>链接类型：{settings.obsidian_settings.link_format}</p><p>图片语法：{settings.obsidian_settings.use_markdown_links ? "Markdown 图片" : "WikiLink"}</p></div>}
    </SettingsSection>
    <SettingsSection icon={<Bot />} title="AI 建议" description="只在主动请求时发送内容"><label className="switch-line">启用 AI 建议<input type="checkbox" checked={settings.ai_enabled} onChange={(event) => setSettings({ ...settings, ai_enabled: event.target.checked })} /></label><label>服务地址<input value={settings.ai_base_url ?? ""} onChange={(event) => setSettings({ ...settings, ai_base_url: event.target.value })} placeholder="https://api.openai.com/v1" /></label><label>模型<input value={settings.ai_model ?? ""} onChange={(event) => setSettings({ ...settings, ai_model: event.target.value })} placeholder="gpt-4.1-mini" /></label><SecretField label="AI API Key" saved={settings.ai_secret_saved} value={aiKey} onValueChange={setAIKey} onDelete={() => toast.show({ kind: "info", message: "请保存空配置前先关闭 AI" })} /><button className="secondary" disabled={savingAI} onClick={() => void persistAI()}><Save size={15} />{savingAI ? "正在保存…" : "保存 AI 设置"}</button></SettingsSection>
    <SettingsSection icon={<Globe2 />} title="发布渠道" description="Hugo 与微信公众号"><h3>微信公众号模板</h3><TemplatePicker value={settings.default_template} templates={settings.templates} onChange={(id) => setSettings({ ...settings, default_template: id })} /><h3>GitHub 图片仓库</h3><label>GitHub Owner<input value={settings.github_owner ?? ""} onChange={(event) => setSettings({ ...settings, github_owner: event.target.value })} /></label><label>Repository<input value={settings.github_repository ?? ""} onChange={(event) => setSettings({ ...settings, github_repository: event.target.value })} /></label><div className="field-pair"><label>Branch<input value={settings.github_branch ?? "main"} onChange={(event) => setSettings({ ...settings, github_branch: event.target.value })} /></label><label>路径前缀<input value={settings.github_prefix ?? "inkhub"} onChange={(event) => setSettings({ ...settings, github_prefix: event.target.value })} /></label></div><SecretField label="GitHub Token" saved={settings.github_token_saved ?? settings.wechat_secret_saved} value={githubToken} onValueChange={setGitHubToken} onDelete={() => toast.show({ kind: "info", message: "请清空仓库配置后保存" })} /><button className="secondary" disabled={savingWeChat} onClick={() => void persistWeChat()}><Save size={15} />{savingWeChat ? "正在保存…" : "保存发布设置"}</button></SettingsSection>
    <SettingsSection icon={<Activity />} title="数据与诊断" description="检查本机依赖和工作区状态"><div className="diagnostic-list">{settings.diagnostics.map((item) => <div key={item.name}><span className={`doctor-${item.state}`}>{item.state}</span><b>{item.name}</b><p>{item.message}</p></div>)}</div><button className="secondary" disabled={diagnosing} onClick={refreshDiagnostics}><RefreshCw size={15} />{diagnosing ? "正在诊断…" : "重新诊断"}</button></SettingsSection>
  </div>;
}

function SettingsSection({ icon, title, description, children }: { icon: React.ReactNode; title: string; description: string; children: React.ReactNode }) {
  return <section className="settings-section"><header>{icon}<div><h2>{title}</h2><p>{description}</p></div></header><div className="settings-content">{children}</div></section>;
}
