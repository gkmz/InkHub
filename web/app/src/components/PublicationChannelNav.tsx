import { BookCheck, CloudUpload, MessageCircle, Sparkles } from "lucide-react";
import type { ArticleDetail, PublicationChannel, PublicationChannelSummary, PublicationDisplayState } from "../api/types";

type PublicationSection = "review" | PublicationChannel;
type ChannelArticle = Pick<ArticleDetail, "id" | "review_state" | "hugo_provider_id" | "wechat_provider_id" | "hugo_state" | "wechat_state" | "xiaohongshu_state">;

interface PublicationChannelNavProps {
  article: ChannelArticle;
  active: PublicationSection;
  onNavigate: (path: string) => void;
}

const channelDefinitions = [
  { channel: "hugo" as const, label: "Hugo", icon: CloudUpload },
  { channel: "wechat" as const, label: "微信", icon: MessageCircle },
  { channel: "xiaohongshu" as const, label: "小红书", icon: Sparkles },
];

// 将后端自然语言状态归一为页面展示状态，不改变持久化数据。
function normalizePublicationState(rawState: string, reviewState: string, configured = true): PublicationDisplayState {
  if (reviewState !== "已通过") return "blocked";
  if (!configured || rawState.includes("未配置")) return "not_configured";
  if (rawState.includes("失败")) return "failed";
  if (rawState.includes("更新") || rawState.includes("过期") || rawState.includes("重新")) return "stale";
  if (rawState.includes("处理中") || rawState.includes("正在") || rawState.includes("准备中")) return "running";
  if (rawState.includes("已同步") || rawState.includes("已准备") || rawState.includes("已复制") || rawState.includes("已确认") || rawState.includes("已发布")) return "completed";
  return "ready";
}

/** PublicationChannelNav 提供审核中心与三个独立发布渠道之间的一致导航。 */
export function PublicationChannelNav({ article, active, onNavigate }: PublicationChannelNavProps) {
  const summaries = channelDefinitions.map(({ channel, label }) => channelSummary(article, channel, label));
  return <nav className="publication-channel-nav" aria-label="文章工作流">
    <button type="button" className={`publication-channel-item state-${article.review_state === "已通过" ? "completed" : "ready"}`} aria-current={active === "review" ? "page" : undefined} onClick={() => onNavigate(`/articles/${article.id}`)}>
      <BookCheck size={17} /><span><b>审核</b><small>{article.review_state}</small></span>
    </button>
    {summaries.map((summary) => {
      const definition = channelDefinitions.find((item) => item.channel === summary.channel)!;
      const Icon = definition.icon;
      const blocked = summary.state === "blocked";
      const target = summary.state === "not_configured" ? "/settings" : `/articles/${article.id}/${summary.channel}`;
      return <button key={summary.channel} type="button" className={`publication-channel-item state-${summary.state}`} aria-current={active === summary.channel ? "page" : undefined} disabled={blocked} title={blocked ? "审核通过后可用" : summary.actionLabel} onClick={() => onNavigate(target)}>
        <Icon size={17} /><span><b>{summary.actionLabel}</b><small>{blocked ? "审核通过后可用" : summary.rawState}</small></span>
      </button>;
    })}
  </nav>;
}

function channelSummary(article: ChannelArticle, channel: PublicationChannel, label: string): PublicationChannelSummary {
  const rawState = channel === "hugo" ? article.hugo_state : channel === "wechat" ? article.wechat_state : article.xiaohongshu_state ?? "尚未准备";
  const configured = channel === "hugo" ? article.hugo_provider_id !== "" : channel === "wechat" ? article.wechat_provider_id !== "" : true;
  const state = normalizePublicationState(rawState, article.review_state, configured);
  const completed = state === "completed";
  const actionLabel = state === "not_configured" ? `配置${label}` : channel === "hugo" ? (completed ? "查看 Hugo" : "同步到 Hugo") : channel === "wechat" ? (completed ? "查看微信" : "发布到微信") : (completed ? "查看小红书" : "发布到小红书");
  return { channel, label, state, rawState: state === "not_configured" ? "未配置" : rawState, actionLabel };
}
