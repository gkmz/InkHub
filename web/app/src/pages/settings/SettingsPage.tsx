import { Activity, Bot, Check, Database, FileOutput, FolderOpen, Globe2, MessageCircle, NotebookPen, Plus, RefreshCw, Save, ScanSearch, X } from "lucide-react";
import { useEffect, useState } from "react";
import { APIError, confirmHugoTakeover, getSettings, pickDirectory, previewContentScope, previewHugoTakeover, saveAISettings, saveContentScope, saveHugoSettings, savePublicationContentSettings, saveWeChatSettings, saveXiaohongshuSettings } from "../../api/client";
import type { HugoTakeoverReport, SettingsView } from "../../api/types";
import { ContentScopePicker } from "../../components/ContentScopePicker";
import { SecretField } from "../../components/SecretField";
import { useToast } from "../../components/toast";

const settingsTabs = [
  { id: "workspace", label: "工作区", icon: Database },
  { id: "content", label: "内容处理", icon: FileOutput },
  { id: "hugo", label: "Hugo", icon: Globe2 },
  { id: "wechat", label: "微信", icon: MessageCircle },
  { id: "xiaohongshu", label: "小红书", icon: NotebookPen },
  { id: "ai", label: "AI", icon: Bot },
  { id: "diagnostics", label: "诊断", icon: Activity },
] as const;

type SettingsTabID = typeof settingsTabs[number]["id"];

function initialSettingsTab(): SettingsTabID {
  const saved = sessionStorage.getItem("inkhub.settings.tab");
  return settingsTabs.some((tab) => tab.id === saved) ? saved as SettingsTabID : "workspace";
}

/** SettingsPage 按工作区、AI、发布渠道和诊断分组保存设置。 */
export function SettingsPage() {
  const toast = useToast();
  const [activeTab, setActiveTab] = useState<SettingsTabID>(initialSettingsTab);
  const [settings, setSettings] = useState<SettingsView | null>(null);
  const [scopeResult, setScopeResult] = useState("");
  const [savingScope, setSavingScope] = useState(false);
  const [savingPublicationContent, setSavingPublicationContent] = useState(false);
  const [scopePreview, setScopePreview] = useState<{ added: number; removed: number } | null>(null);
  const [diagnosing, setDiagnosing] = useState(false);
  const [aiKey, setAIKey] = useState("");
  const [savingAI, setSavingAI] = useState(false);
  const [githubToken, setGitHubToken] = useState("");
  const [savingWeChat, setSavingWeChat] = useState(false);
  const [savingXiaohongshu, setSavingXiaohongshu] = useState(false);
  const [savingHugo, setSavingHugo] = useState(false);
  const [takeoverReport, setTakeoverReport] = useState<HugoTakeoverReport | null>(null);
  const [takingOver, setTakingOver] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void getSettings(controller.signal).then(async (value) => {
      setSettings(value);
      if (!sessionStorage.getItem("inkhub.hugo-takeover-pending") || !value.hugo_enabled || !value.hugo_valid) return;
      try {
        setActiveTab("hugo");
        sessionStorage.setItem("inkhub.settings.tab", "hugo");
        setTakeoverReport(await previewHugoTakeover());
        sessionStorage.removeItem("inkhub.hugo-takeover-pending");
      } catch (reason) {
        toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "历史内容扫描失败" });
      }
    });
    return () => controller.abort();
  }, [toast]);
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
      setScopeResult(`已索引 ${result.indexed} 篇，补充 ${result.assigned_ids} 个 Stable ID`);
      setScopePreview(null);
      toast.show({ kind: result.failed > 0 ? "info" : "success", message: "内容范围已保存并完成重扫" });
    } catch (reason) {
		const message = reason instanceof Error ? reason.message : "保存失败";
		if (reason instanceof APIError && reason.details?.initialization && typeof reason.details.initialization === "object") {
			const initialization = reason.details.initialization as { issues?: Array<{ article_path: string; message: string }> };
			const issues = initialization.issues ?? [];
			setScopeResult(issues.length > 0 ? `${message}：${issues.map((issue) => `${issue.article_path}（${issue.message}）`).join("；")}` : message);
		} else {
			setScopeResult(message);
		}
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
  const persistPublicationContent = async () => {
    setSavingPublicationContent(true);
    try {
      const result = await savePublicationContentSettings({ excluded_sections: settings.excluded_sections ?? [] });
      setSettings({ ...settings, ...result });
      toast.show({ kind: "success", message: "发布内容规则已保存" });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "发布内容规则保存失败" });
    } finally {
      setSavingPublicationContent(false);
    }
  };
  const persistWeChat = async () => {
    setSavingWeChat(true);
    try {
      const result = await saveWeChatSettings({
        enabled: settings.wechat_enabled, template: "default",
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
  const persistXiaohongshu = async () => {
    setSavingXiaohongshu(true);
    try {
      const result = await saveXiaohongshuSettings({ enabled: settings.xiaohongshu_enabled, template: settings.xiaohongshu_template });
      setSettings({ ...settings, ...result });
      toast.show({ kind: "success", message: settings.xiaohongshu_enabled ? "小红书设置已保存" : "小红书发布已关闭" });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "小红书设置保存失败" });
    } finally {
      setSavingXiaohongshu(false);
    }
  };
  const selectHugoDirectory = async () => {
    try {
      const { path } = await pickDirectory("hugo");
      setTakeoverReport(null);
      setSettings({ ...settings, hugo_path: path, hugo_enabled: true, hugo_valid: false });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "无法选择 Hugo 目录" });
    }
  };
  const inspectHugoTakeover = async () => {
    setTakingOver(true);
    try {
      const report = await previewHugoTakeover();
      setTakeoverReport(report);
      toast.show({ kind: report.conflict_count > 0 ? "info" : "success", message: "历史内容扫描完成" });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "历史内容扫描失败" });
    } finally {
      setTakingOver(false);
    }
  };
  const persistHugo = async () => {
    setSavingHugo(true);
    setTakeoverReport(null);
    try {
      const result = await saveHugoSettings({ enabled: settings.hugo_enabled, path: settings.hugo_path ?? "", base_url: settings.hugo_base_url ?? "" });
      setSettings({ ...settings, ...result });
      toast.show({ kind: "success", message: settings.hugo_enabled ? "Hugo 目录已保存" : "Hugo 发布已关闭" });
      if (settings.hugo_enabled) {
        const report = await previewHugoTakeover();
        setTakeoverReport(report);
      }
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "Hugo 设置保存失败" });
    } finally {
      setSavingHugo(false);
    }
  };
  const applyHugoTakeover = async () => {
    setTakingOver(true);
    try {
      const result = await confirmHugoTakeover();
      const refreshed = await getSettings();
      setSettings(refreshed);
      setTakeoverReport(await previewHugoTakeover());
      toast.show({ kind: result.remaining_source_issues > 0 ? "info" : "success", message: `已补齐 ${result.assigned_ids} 篇文章身份，恢复 ${result.recovered_articles} 篇历史文章，关联 ${result.linked_bundles} 个 Bundle` });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "Hugo 历史内容接管失败" });
    } finally {
      setTakingOver(false);
    }
  };
  const selectTab = (tab: SettingsTabID) => {
    setActiveTab(tab);
    sessionStorage.setItem("inkhub.settings.tab", tab);
  };
  const moveTab = (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    let target = index;
    if (event.key === "ArrowRight") target = (index + 1) % settingsTabs.length;
    else if (event.key === "ArrowLeft") target = (index - 1 + settingsTabs.length) % settingsTabs.length;
    else if (event.key === "Home") target = 0;
    else if (event.key === "End") target = settingsTabs.length - 1;
    else return;
    event.preventDefault();
    selectTab(settingsTabs[target].id);
    requestAnimationFrame(() => document.getElementById(`settings-tab-${settingsTabs[target].id}`)?.focus());
  };
  return <div className="settings-page">
    <nav className="settings-tabs" role="tablist" aria-label="设置分类">
      {settingsTabs.map((tab, index) => {
        const Icon = tab.icon;
        return <button key={tab.id} id={`settings-tab-${tab.id}`} role="tab" aria-selected={activeTab === tab.id} aria-controls={`settings-panel-${tab.id}`} tabIndex={activeTab === tab.id ? 0 : -1} onKeyDown={(event) => moveTab(event, index)} onClick={() => selectTab(tab.id)}><Icon size={16} /><span>{tab.label}</span></button>;
      })}
    </nav>
    <div className="settings-tab-panel" id={`settings-panel-${activeTab}`} role="tabpanel" aria-labelledby={`settings-tab-${activeTab}`}>
      {activeTab === "workspace" && <SettingsSection icon={<Database />} title="工作区" description="确定内容来源、索引范围和 Obsidian 解析方式">
        <SettingsGroup title="工作区信息" description="当前连接的知识库。名称和位置在初始化后保持只读。">
          <dl className="workspace-summary"><div><dt>工作区</dt><dd>{settings.workspace_name}</dd></div><div><dt>Obsidian Vault</dt><dd>{settings.vault_path}</dd></div></dl>
        </SettingsGroup>
        <SettingsGroup title="内容范围" description="控制哪些 Markdown 进入内容库，以及哪些子目录和文件被排除。">
          <ContentScopePicker directories={settings.directories} contentRoots={settings.content_roots} ignoredFolders={settings.ignored_folders} ignoredFileNames={settings.ignored_file_names} showHeading={false} onChange={updateScope} />
          <div className="settings-actions">{!scopePreview && <button className="secondary" disabled={savingScope || settings.content_roots.length === 0} onClick={inspectScopeChange}><Save size={15} />{savingScope ? "正在计算…" : "预览内容范围变更"}</button>}</div>
          {scopePreview && <div className="scope-confirm"><p>将新增 {scopePreview.added} 篇，移出 {scopePreview.removed} 篇。新纳入且缺少身份的文章会补充 frontmatter 和 Stable ID。</p><button className="primary" disabled={savingScope} onClick={persistScope}>{savingScope ? "正在初始化…" : "确认并初始化"}</button><button className="secondary" onClick={() => setScopePreview(null)}>取消</button></div>}
          {scopeResult && <p className="inline-status" role="status">{scopeResult}</p>}
        </SettingsGroup>
        {settings.obsidian_settings && <SettingsGroup title="Obsidian 解析" description="InkHub 按这些 Vault 设置解析附件和内部链接。"><dl className="obsidian-settings-summary"><div><dt>附件位置</dt><dd>{settings.obsidian_settings.attachment_location}</dd></div><div><dt>链接类型</dt><dd>{settings.obsidian_settings.link_format}</dd></div><div><dt>图片语法</dt><dd>{settings.obsidian_settings.use_markdown_links ? "Markdown 图片" : "WikiLink"}</dd></div></dl></SettingsGroup>}
      </SettingsSection>}
      {activeTab === "content" && <SettingsSection icon={<FileOutput />} title="内容处理" description="控制源文章进入各发布渠道前的统一转换规则">
        <SettingsGroup title="发布时排除的章节" description="审核页仍展示完整原文；Hugo、微信和小红书会移除匹配标题及其全部子内容。">
          <ExcludedSectionEditor value={settings.excluded_sections ?? []} onChange={(excluded_sections) => setSettings({ ...settings, excluded_sections })} />
          <button className="secondary" disabled={savingPublicationContent} onClick={() => void persistPublicationContent()}><Save size={15} />{savingPublicationContent ? "正在保存…" : "保存内容处理规则"}</button>
        </SettingsGroup>
      </SettingsSection>}
      {activeTab === "hugo" && <SettingsSection icon={<Globe2 />} title="Hugo" description="博客目录和历史内容接管">
        <SettingsGroup title="发布状态" description="关闭后文章仍保留在内容库，但不会显示 Hugo 发布入口。"><label className="switch-line">启用 Hugo<input type="checkbox" checked={settings.hugo_enabled} onChange={(event) => { setTakeoverReport(null); setSettings({ ...settings, hugo_enabled: event.target.checked }); }} /></label></SettingsGroup>
        <SettingsGroup title="站点连接" description="选择 Hugo 项目根目录，并设置发布后使用的站点地址。">
          <label>Hugo 根目录<div className="settings-path-field"><input value={settings.hugo_path ?? ""} onChange={(event) => { setTakeoverReport(null); setSettings({ ...settings, hugo_path: event.target.value, hugo_valid: false }); }} placeholder="选择包含 hugo.yaml 或 hugo.toml 的目录" /><button type="button" title="选择 Hugo 目录" aria-label="选择 Hugo 目录" onClick={() => void selectHugoDirectory()}><FolderOpen size={17} /></button></div></label>
          <label>站点地址<input value={settings.hugo_base_url ?? ""} onChange={(event) => setSettings({ ...settings, hugo_base_url: event.target.value })} placeholder="可选，如 https://example.com" /></label>
          <div className="settings-actions"><button className="secondary" disabled={savingHugo || (settings.hugo_enabled && !settings.hugo_path?.trim())} onClick={() => void persistHugo()}><Save size={15} />{savingHugo ? "正在校验…" : "保存并扫描"}</button>{settings.hugo_enabled && settings.hugo_valid && <button className="secondary" disabled={takingOver} onClick={() => void inspectHugoTakeover()}><ScanSearch size={15} />{takingOver ? "正在扫描…" : "重新扫描历史内容"}</button>}</div>
          {takeoverReport && <HugoTakeover report={takeoverReport} busy={takingOver} onConfirm={() => void applyHugoTakeover()} />}
        </SettingsGroup>
      </SettingsSection>}
      {activeTab === "wechat" && <SettingsSection icon={<MessageCircle />} title="微信" description="公众号模板和图片仓库">
        <SettingsGroup title="发布状态" description="控制文章审核页是否显示微信公众号发布入口。"><label className="switch-line">启用微信发布<input type="checkbox" checked={settings.wechat_enabled} onChange={(event) => setSettings({ ...settings, wechat_enabled: event.target.checked })} /></label></SettingsGroup>
        <SettingsGroup title="公众号模板" description="所有微信文章默认使用同一套正文视觉。"><p className="fixed-template"><span>InkHub 墨绿</span><small>文章发布页可选择 Mermaid 样式</small></p></SettingsGroup>
        <SettingsGroup title="图片仓库" description="将本地图片上传到公开 GitHub 仓库，生成微信可访问的地址。"><label>GitHub Owner<input value={settings.github_owner ?? ""} onChange={(event) => setSettings({ ...settings, github_owner: event.target.value })} /></label><label>Repository<input value={settings.github_repository ?? ""} onChange={(event) => setSettings({ ...settings, github_repository: event.target.value })} /></label><div className="field-pair"><label>Branch<input value={settings.github_branch ?? "main"} onChange={(event) => setSettings({ ...settings, github_branch: event.target.value })} /></label><label>路径前缀<input value={settings.github_prefix ?? "inkhub"} onChange={(event) => setSettings({ ...settings, github_prefix: event.target.value })} /></label></div><SecretField label="GitHub Token" saved={settings.github_token_saved ?? settings.wechat_secret_saved} value={githubToken} onValueChange={setGitHubToken} onDelete={() => toast.show({ kind: "info", message: "请清空仓库配置后保存" })} /><button className="secondary" disabled={savingWeChat} onClick={() => void persistWeChat()}><Save size={15} />{savingWeChat ? "正在保存…" : "保存微信设置"}</button></SettingsGroup>
      </SettingsSection>}
      {activeTab === "xiaohongshu" && <SettingsSection icon={<NotebookPen />} title="小红书" description="图片笔记发布和默认模板">
        <SettingsGroup title="发布状态" description="控制文章审核页是否显示小红书笔记发布入口。"><label className="switch-line">启用小红书发布<input type="checkbox" checked={settings.xiaohongshu_enabled} onChange={(event) => setSettings({ ...settings, xiaohongshu_enabled: event.target.checked })} /></label></SettingsGroup>
        <SettingsGroup title="默认模板" description="用于笔记分页预览和最终图片导出。"><div className="template-picker"><div className="template-grid">{settings.xiaohongshu_templates.map((template) => {
          const selected = settings.xiaohongshu_template === template.id;
          return <label key={template.id} className={selected ? "selected" : ""}><input type="radio" name="xiaohongshu-template" value={template.id} checked={selected} disabled={!template.compatible} onChange={() => setSettings({ ...settings, xiaohongshu_template: template.id })} /><span className="real-template-preview xiaohongshu-template-preview"><NotebookPen size={18} /><i /><i /><i /></span><b>{template.name}</b><small>{template.compatible ? "中文排版 · 护眼配色 · 375 × 667" : "当前版本不可用"}</small>{selected && <Check className="selected-check" aria-hidden="true" />}</label>;
        })}</div></div>
          <button className="secondary" disabled={savingXiaohongshu} onClick={() => void persistXiaohongshu()}><Save size={15} />{savingXiaohongshu ? "正在保存…" : "保存小红书设置"}</button>
        </SettingsGroup>
      </SettingsSection>}
      {activeTab === "ai" && <SettingsSection icon={<Bot />} title="AI" description="审核建议和小红书改写服务"><SettingsGroup title="服务状态" description="关闭后仍可手工审核和编辑各发布渠道内容。"><label className="switch-line">启用 AI<input type="checkbox" checked={settings.ai_enabled} onChange={(event) => setSettings({ ...settings, ai_enabled: event.target.checked })} /></label></SettingsGroup><SettingsGroup title="模型连接" description="配置兼容 OpenAI API 的服务地址、模型和访问密钥。"><label>服务地址<input value={settings.ai_base_url ?? ""} onChange={(event) => setSettings({ ...settings, ai_base_url: event.target.value })} placeholder="https://api.openai.com/v1" /></label><label>模型<input value={settings.ai_model ?? ""} onChange={(event) => setSettings({ ...settings, ai_model: event.target.value })} placeholder="gpt-4.1-mini" /></label><SecretField label="AI API Key" saved={settings.ai_secret_saved} value={aiKey} onValueChange={setAIKey} onDelete={() => toast.show({ kind: "info", message: "请保存空配置前先关闭 AI" })} /><button className="secondary" disabled={savingAI} onClick={() => void persistAI()}><Save size={15} />{savingAI ? "正在保存…" : "保存 AI 设置"}</button></SettingsGroup></SettingsSection>}
      {activeTab === "diagnostics" && <SettingsSection icon={<Activity />} title="诊断" description="本机依赖和工作区状态"><SettingsGroup title="运行检查" description="确认内容目录、发布渠道和本机数据库是否可用。"><div className="diagnostic-list">{settings.diagnostics.map((item) => <div key={item.name}><span className={`doctor-${item.state}`}>{item.state}</span><b>{item.name}</b><p>{item.message}</p></div>)}</div><button className="secondary" disabled={diagnosing} onClick={refreshDiagnostics}><RefreshCw size={15} />{diagnosing ? "正在诊断…" : "重新诊断"}</button></SettingsGroup></SettingsSection>}
    </div>
  </div>;
}

function HugoTakeover({ report, busy, onConfirm }: { report: HugoTakeoverReport; busy: boolean; onConfirm: () => void }) {
  const visible = report.candidates.filter((item) => item.status !== "matched").concat(report.candidates.filter((item) => item.status === "matched")).slice(0, 12);
  const completed = report.articles_missing_id === 0 && report.source_issue_count === 0 && report.conflict_count === 0;
  return <section className="takeover-report" aria-label="Hugo 历史内容接管报告">
    <header><div><h3>{completed ? "历史内容已接管" : "历史内容接管报告"}</h3><p>只自动处理唯一匹配；未匹配内容保持原样。</p></div><span className={report.conflict_count > 0 ? "takeover-warning" : "takeover-ready"}>{report.conflict_count > 0 ? `${report.conflict_count} 个冲突` : "可以接管"}</span></header>
    <div className="takeover-summary"><span><b>{report.bundle_count}</b>Hugo Bundle</span><span><b>{report.matched_count}</b>确定匹配</span><span><b>{report.articles_missing_id}</b>待补文章 ID</span><span><b>{report.source_issue_count}</b>待恢复旧文章</span><span><b>{report.unmatched_count}</b>未匹配</span></div>
    {report.source_issues.length > 0 && <details className="takeover-source-issues"><summary>查看 {report.source_issue_count} 篇历史格式异常文章</summary><div>{report.source_issues.map((issue) => <p key={issue.article_path}><b>{issue.article_path}</b><span>{issue.message}</span></p>)}</div></details>}
    {visible.length > 0 && <div className="takeover-list">{visible.map((item) => <div key={item.bundle_path}><span className={`takeover-status ${item.status}`}>{item.status === "matched" ? "匹配" : item.status === "conflict" ? "冲突" : "未匹配"}</span><span><b>{item.title || item.bundle_path}</b><small>{item.article_path ? `${item.bundle_path} → ${item.article_path}` : item.bundle_path}</small></span><small>{item.match_reason ?? "没有足够的匹配信息"}</small></div>)}</div>}
    {!completed && <div className="takeover-confirm"><p>确认后会规范化 {report.source_issue_count} 篇旧格式文章，为缺失身份的内容写入 Stable ID，再关联 {report.matched_count} 个历史 Bundle。该操作会修改 Obsidian 和 Hugo 文件。</p><button className="primary" disabled={busy || report.conflict_count > 0} onClick={onConfirm}>{busy ? "正在接管…" : "确认接管并补齐身份"}</button></div>}
  </section>;
}

function SettingsSection({ icon, title, description, children }: { icon: React.ReactNode; title: string; description: string; children: React.ReactNode }) {
  return <section className="settings-section"><header>{icon}<div><h2>{title}</h2><p>{description}</p></div></header><div className="settings-content">{children}</div></section>;
}

/** SettingsGroup 为同一类设置提供稳定的标题、说明和操作区域。 */
function SettingsGroup({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section className="settings-group"><header><div><h3>{title}</h3><p>{description}</p></div></header><div className="settings-group-body">{children}</div></section>;
}

/** ExcludedSectionEditor 以精确标题标签维护发布时需要裁剪的章节。 */
function ExcludedSectionEditor({ value, onChange }: { value: string[]; onChange: (value: string[]) => void }) {
  const [input, setInput] = useState("");
  const add = () => {
    const title = input.trim();
    if (!title || value.includes(title)) return;
    onChange([...value, title]);
    setInput("");
  };
  return <div className="excluded-section-editor">
    <div className="excluded-section-control">
      {value.map((title) => <span className="excluded-section-chip" key={title}><span>{title}</span><button type="button" title={`删除 ${title}`} aria-label={`删除排除章节 ${title}`} onClick={() => onChange(value.filter((item) => item !== title))}><X size={12} /></button></span>)}
      <input aria-label="新增排除章节标题" value={input} placeholder={value.length === 0 ? "输入标题，如：相关链接" : "继续添加标题"} onChange={(event) => setInput(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); add(); } }} />
      <button type="button" className="add-section-title" title="添加章节标题" aria-label="添加排除章节" disabled={!input.trim() || value.includes(input.trim())} onClick={add}><Plus size={15} /></button>
    </div>
    <small>精确匹配标题文本，不区分标题级别；同名章节出现多次时会全部排除。</small>
  </div>;
}
