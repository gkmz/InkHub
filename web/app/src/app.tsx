import { useEffect, useState } from "react";
import { createWorkspace, getSession } from "./api/client";
import type { SessionResponse, WorkspaceDraft } from "./api/types";
import { AppShell } from "./components/AppShell";
import { LibraryPage } from "./pages/library/LibraryPage";
import { SetupPage } from "./pages/setup/SetupPage";
import { ScanPage } from "./pages/setup/ScanPage";
import { DashboardPage } from "./pages/workspace/DashboardPage";
import { ArticlePage } from "./pages/article/ArticlePage";
import { HugoPage } from "./pages/hugo/HugoPage";
import { WeChatPreviewPage } from "./pages/wechat-preview/WeChatPreviewPage";
import { XiaohongshuPage } from "./pages/xiaohongshu/XiaohongshuPage";
import { TaxonomyPage } from "./pages/taxonomy/TaxonomyPage";
import { SettingsPage } from "./pages/settings/SettingsPage";
import { ToastProvider } from "./components/ToastProvider";

/** App 根据最近工作区状态选择初始化或日常工作界面。 */
export function App() {
  return <ToastProvider><AppContent /></ToastProvider>;
}

function AppContent() {
  const [session, setSession] = useState<SessionResponse | null>(null);
  const [path, setPath] = useState(window.location.pathname);
  const [error, setError] = useState("");
  const [scanJob, setScanJob] = useState(() => sessionStorage.getItem("inkhub.scan-job") ?? "");
  useEffect(() => { getSession().then(setSession).catch((reason: Error) => setError(reason.message)); }, []);
  useEffect(() => { const listener = () => setPath(window.location.pathname); window.addEventListener("popstate", listener); return () => window.removeEventListener("popstate", listener); }, []);
  const navigate = (target: string) => { window.history.pushState({}, "", target); setPath(target); };
  if (error) return <main className="fatal-state"><h1>无法连接 InkHub</h1><p>{error}</p><button onClick={() => window.location.reload()}>重新连接</button></main>;
  if (!session) return <main className="boot-state"><span className="brand-mark">I</span><p>正在打开 InkHub…</p></main>;
  if (!session.has_workspace) return <SetupPage onComplete={async (draft: WorkspaceDraft) => { const result = await createWorkspace(draft, crypto.randomUUID()); sessionStorage.removeItem("inkhub.setup"); sessionStorage.setItem("inkhub.scan-job", result.job_id); setScanJob(result.job_id); setSession({ has_workspace: true, workspace: result.workspace }); navigate("/"); }} />;
  if (scanJob && session.workspace) return <ScanPage workspace={session.workspace} jobID={scanJob} onDone={() => { sessionStorage.removeItem("inkhub.scan-job"); setScanJob(""); }} />;
  const articleMatch = path.match(/^\/articles\/([^/]+)(\/hugo|\/wechat|\/xiaohongshu)?$/);
  if (articleMatch?.[2] === "/hugo") return <HugoPage articleID={articleMatch[1]} onNavigate={navigate} />;
  if (articleMatch?.[2] === "/wechat") return <WeChatPreviewPage articleID={articleMatch[1]} onNavigate={navigate} />;
  if (articleMatch?.[2] === "/xiaohongshu") return <XiaohongshuPage articleID={articleMatch[1]} onNavigate={navigate} />;
  const title = path === "/library" ? "内容库" : path === "/taxonomy" ? "类目管理" : path === "/settings" ? "设置" : "工作台";
  return <AppShell path={path} title={articleMatch ? "文章审核" : title} workspaceName={session.workspace?.name ?? "InkHub"} onNavigate={navigate} contentClassName={articleMatch ? "article-shell" : undefined}>{articleMatch ? <ArticlePage articleID={articleMatch[1]} onNavigate={navigate} /> : path === "/library" ? <LibraryPage onNavigate={navigate} /> : path === "/taxonomy" ? <TaxonomyPage /> : path === "/settings" ? <SettingsPage /> : <DashboardPage onNavigate={navigate} />}</AppShell>;
}
