import { Check, FileSearch, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { getJob } from "../../api/client";
import type { JobStatus, WorkspaceSummary } from "../../api/types";

/** ScanPage 在文章身份全部就绪前阻止进入主界面，并支持原地重试。 */
export function ScanPage({ workspace, jobID, onDone, onRetry }: { workspace: WorkspaceSummary; jobID: string; onDone: () => void; onRetry: () => Promise<void> }) {
  const [job, setJob] = useState<JobStatus>({ id: jobID, state: "queued", progress: 0 });
  const [retryVersion, setRetryVersion] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let timer = 0;
    const poll = async () => {
      try {
        const next = await getJob(jobID, controller.signal);
        setJob(next);
        if (next.state === "queued" || next.state === "running") timer = window.setTimeout(poll, 800);
      } catch (reason) {
        if ((reason as Error).name !== "AbortError") setJob((current) => ({ ...current, state: "failed" }));
      }
    };
    void poll();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [jobID, retryVersion]);
  const complete = job.state === "succeeded";
  return <main className="scan-page"><div className="brand"><span className="brand-mark">I</span><strong>InkHub</strong></div><section><span className={`scan-icon ${job.state}`}>{complete ? <Check /> : job.state === "failed" ? <TriangleAlert /> : <FileSearch />}</span><p className="eyebrow">{workspace.name}</p><h1>{complete ? "内容库已准备好" : job.state === "failed" ? "初始化未能完成" : "正在初始化文章"}</h1><p>{complete ? `已索引 ${job.indexed ?? 0} 篇文章，补充 ${job.assigned_ids ?? 0} 个 Stable ID。` : job.state === "failed" ? (job.error_message || "请修复下列源文件问题后重试。") : "正在检查 frontmatter 并建立稳定文章身份。"}</p>{job.state === "failed" && job.issues && job.issues.length > 0 && <ul className="initialization-issues">{job.issues.map((issue) => <li key={`${issue.article_path}-${issue.code}`}><strong>{issue.article_path}</strong><span>{issue.message}</span></li>)}</ul>}<div className="progress-track" aria-label="初始化进度" aria-valuenow={job.progress} role="progressbar"><span style={{ width: `${job.progress}%` }} /></div>{complete && <button className="primary" onClick={onDone}>进入 InkHub</button>}{job.state === "failed" && <button className="secondary" onClick={async () => { setJob((current) => ({ ...current, state: "running", progress: 5 })); await onRetry(); setRetryVersion((current) => current + 1); }}>重新检查并初始化</button>}</section></main>;
}
