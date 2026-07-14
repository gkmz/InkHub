import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { AISuggestions } from "../../components/AISuggestions";
import { MetadataForm } from "../../components/MetadataForm";
import { PublicationTrack } from "../../components/PublicationTrack";
import { WeChatActions } from "../../components/WeChatActions";
import { JobStatus } from "../../components/JobStatus";

const metadata = {
  title: "本地优先的内容工作流",
  description: "旧摘要",
  category: "工程实践",
  series: "InkHub",
  tags: ["Go", "React"],
  keywords: ["本地优先"],
  slug: "local-first-content",
  cover: "",
};

test("源文件变化后元数据表单禁止覆盖并提供重新加载", async () => {
  const save = vi.fn();
  render(<MetadataForm value={metadata} sourceChanged onSave={save} />);
  await userEvent.clear(screen.getByLabelText("标题"));
  await userEvent.type(screen.getByLabelText("标题"), "新标题");
  expect(screen.getByText("文章已在写作工具中更新")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存到文章" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "重新加载" })).toBeInTheDocument();
  expect(save).not.toHaveBeenCalled();
});

test("保存元数据前展示字段级变更摘要", async () => {
  render(<MetadataForm value={metadata} sourceChanged={false} onSave={vi.fn()} />);
  await userEvent.clear(screen.getByLabelText("Description"));
  await userEvent.type(screen.getByLabelText("Description"), "新摘要");
  expect(screen.getByText("Description：旧摘要 → 新摘要")).toBeInTheDocument();
});

test("AI 建议只能逐字段采用并写入表单草稿", async () => {
  const accept = vi.fn();
  render(<AISuggestions stale={false} suggestions={[{ field: "description", original: "旧摘要", suggested: "更清晰的新摘要", reason: "补充文章收益" }]} onAccept={accept} />);
  expect(screen.queryByRole("button", { name: /全部/ })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "采用 Description 建议" }));
  expect(accept).toHaveBeenCalledWith("description", "更清晰的新摘要");
});

test("文章更新后 AI 建议过期且不能继续采用", () => {
  render(<AISuggestions stale suggestions={[{ field: "title", original: "旧标题", suggested: "新标题", reason: "更具体" }]} onAccept={vi.fn()} />);
  expect(screen.getByText("文章已更新，请重新分析")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "采用 Title 建议" })).toBeDisabled();
});

test("发布轨道不暴露内部 hash 和任务 ID", () => {
  render(<PublicationTrack review="已通过" hugo="需要同步" wechat="尚未准备" />);
  expect(screen.getByText("审核")).toBeInTheDocument();
  expect(screen.getByText("需要同步")).toBeInTheDocument();
  expect(screen.queryByText(/hash|job_/i)).not.toBeInTheDocument();
});

test("Hugo 任务失败时显示失败步骤和重试且不宣称成功", () => {
  render(<JobStatus state="failed" progress={68} stage="构建预览" message="Hugo build 未通过" onRetry={vi.fn()} />);
  expect(screen.getByText("构建预览失败")).toBeInTheDocument();
  expect(screen.getByText("Hugo build 未通过")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
  expect(screen.queryByText("同步完成")).not.toBeInTheDocument();
});

test("微信必须先复制当前内容才能人工确认草稿", async () => {
  const copy = vi.fn().mockResolvedValue(undefined);
  const confirm = vi.fn();
  render(<WeChatActions copied={false} onCopy={copy} onConfirm={confirm} />);
  expect(screen.queryByRole("button", { name: "草稿已保存" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "复制格式化内容" }));
  expect(copy).toHaveBeenCalledOnce();
});

test("剪贴板失败后恢复复制按钮并提供 HTML 兜底", async () => {
  render(<WeChatActions copied={false} onCopy={vi.fn().mockRejectedValue(new Error("denied"))} onConfirm={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "复制格式化内容" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("无法写入剪贴板");
  expect(screen.getByRole("button", { name: "复制格式化内容" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "查看 HTML" })).toBeInTheDocument();
});
