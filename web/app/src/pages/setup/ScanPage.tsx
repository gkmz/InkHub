import { Check, FileSearch, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { getJob } from "../../api/client";
import type { JobStatus, WorkspaceSummary } from "../../api/types";

/** ScanPage 按 Job ID 恢复首次扫描，单篇失败不会阻止进入工作台。 */
export function ScanPage({ workspace, jobID, onDone }: { workspace: WorkspaceSummary; jobID: string; onDone: () => void }) {
  const [job, setJob] = useState<JobStatus>({ id: jobID, state: "queued", progress: 0 });
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
  }, [jobID]);
  const complete = job.state === "succeeded";
  return <main className="scan-page"><div className="brand"><span className="brand-mark">I</span><strong>InkHub</strong></div><section><span className={`scan-icon ${job.state}`}>{complete ? <Check /> : job.state === "failed" ? <TriangleAlert /> : <FileSearch />}</span><p className="eyebrow">{workspace.name}</p><h1>{complete ? "内容库已准备好" : job.state === "failed" ? "扫描未能完成" : "正在整理你的文章"}</h1><p>{complete ? `已索引 ${job.indexed ?? 0} 篇文章${job.failed ? `，${job.failed} 篇需要稍后处理` : ""}。` : "你可以离开这个页面，重新打开后会从当前任务继续。"}</p><div className="progress-track" aria-label="扫描进度" aria-valuenow={job.progress} role="progressbar"><span style={{ width: `${job.progress}%` }} /></div>{complete && <button className="primary" onClick={onDone}>进入工作台</button>}{job.state === "failed" && <button className="secondary" onClick={onDone}>先进入工作台</button>}</section></main>;
}
