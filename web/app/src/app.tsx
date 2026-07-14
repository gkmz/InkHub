import { useEffect, useState } from "react";
import { createWorkspace, getSession } from "./api/client";
import type { SessionResponse, WorkspaceDraft } from "./api/types";
import { AppShell } from "./components/AppShell";
import { LibraryPage } from "./pages/library/LibraryPage";
import { SetupPage } from "./pages/setup/SetupPage";
import { ScanPage } from "./pages/setup/ScanPage";
import { DashboardPage } from "./pages/workspace/DashboardPage";

/** App 根据最近工作区状态选择初始化或日常工作界面。 */
export function App() {
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
  const title = path === "/library" ? "内容库" : path === "/taxonomy" ? "标签治理" : path === "/settings" ? "设置" : "工作台";
  return <AppShell path={path} title={title} workspaceName={session.workspace?.name ?? "InkHub"} onNavigate={navigate}>{path === "/library" ? <LibraryPage /> : path === "/taxonomy" || path === "/settings" ? <div className="empty-state"><h2>{title}将在下一阶段开放</h2><p>主导航位置已保留，当前阶段专注于初始化和内容浏览。</p></div> : <DashboardPage onNavigate={navigate} />}</AppShell>;
}
