import { AlertTriangle, Check, LoaderCircle, RotateCcw } from "lucide-react";

/** JobStatus 展示可恢复后台任务，不向普通页面暴露 Job ID。 */
export function JobStatus({ state, progress, stage, message, onRetry }: { state: "queued" | "running" | "succeeded" | "failed"; progress: number; stage: string; message?: string; onRetry: () => void }) {
  const failed = state === "failed";
  const succeeded = state === "succeeded";
  return <section className={`job-status job-${state}`} aria-live="polite"><span>{failed ? <AlertTriangle /> : succeeded ? <Check /> : <LoaderCircle />}</span><div><b>{failed ? `${stage}失败` : succeeded ? "同步完成" : stage}</b><p>{message ?? (succeeded ? "渠道状态已更新" : `已完成 ${progress}%`)}</p>{!succeeded && <div className="progress-track" role="progressbar" aria-label={stage} aria-valuenow={progress}><i style={{ width: `${progress}%` }} /></div>}</div>{failed && <button className="secondary" type="button" onClick={onRetry}><RotateCcw size={14} />重试</button>}</section>;
}
