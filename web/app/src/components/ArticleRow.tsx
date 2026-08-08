import { ArrowRight, CircleAlert, FilePenLine, RotateCcw } from "lucide-react";
import type { ArticleState, ArticleSummary } from "../api/types";

const stateText: Record<ArticleState, string> = { draft: "草稿", blocked: "处理失败", changed: "内容已更新", incomplete: "信息不完整", pending_review: "等待审核", approved: "已通过" };
const actionText: Record<ArticleState, string> = { draft: "继续编辑", blocked: "重试", changed: "查看更新", incomplete: "补充信息", pending_review: "继续审核", approved: "查看" };

/** ArticleRow 用自然语言展示文章状态，不泄露内部 hash 或任务标识。 */
export function ArticleRow({ article, dashboard = false, onOpen, selected = false, onSelectedChange }: { article: ArticleSummary; dashboard?: boolean; onOpen?: (id: string) => void; selected?: boolean; onSelectedChange?: (id: string, selected: boolean) => void }) {
  const ActionIcon = article.state === "blocked" ? RotateCcw : article.state === "incomplete" ? FilePenLine : ArrowRight;
  const statusText = article.content_stage === "draft" ? "草稿" : article.disposition === "published" ? "已发表" : article.disposition === "ignored" ? "已忽略" : stateText[article.state];
  const actionLabel = article.content_stage === "draft" ? "继续编辑" : article.next_action === "retry" ? "重试" : article.next_action === "review" ? "审核" : article.next_action === "publish" ? "发布" : article.next_action === "view" ? "查看" : actionText[article.state];
  return (
    <article className={`article-row state-${article.state}${onSelectedChange ? " selectable" : ""}`} data-testid={dashboard ? "dashboard-row" : "library-row"}>
      {onSelectedChange && <input className="row-select" type="checkbox" aria-label={`选择文章 ${article.title || "未命名文章"}`} checked={selected} onChange={(event) => onSelectedChange(article.id, event.currentTarget.checked)} />}
      <div className="article-primary">
        <div className="article-title-line">{article.state === "blocked" && <CircleAlert size={16} aria-hidden="true" />}<h3>{article.title || "未命名文章"}</h3></div>
        <p><b className="article-filename" title={article.filename}>{article.filename || "未命名文章"}</b> · {article.directory || "根目录"} · {article.category || "未分类"}{article.content_stage_issue && <span className="stage-warning"> · {article.content_stage_issue}</span>}</p>
      </div>
      <time dateTime={article.modified_at}>{new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric" }).format(new Date(article.modified_at))}</time>
      <span className="status-label">{statusText}</span>
      <span className="channel-state"><b>H</b>{article.hugo_state}</span>
      <span className="channel-state"><b>微</b>{article.wechat_state}</span>
      <span className="channel-state"><b>小</b>{article.xiaohongshu_state ?? "尚未准备"}</span>
      <button className="row-action" type="button" onClick={() => onOpen?.(article.id)}>{actionLabel}<ActionIcon size={16} aria-hidden="true" /></button>
    </article>
  );
}
