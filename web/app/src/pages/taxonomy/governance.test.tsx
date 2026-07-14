import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { TagApproval } from "../../components/TagApproval";
import { TemplatePicker } from "../../components/TemplatePicker";
import { SecretField } from "../../components/SecretField";

test("批准新 Tag 前展示 YAML diff 和受影响文章", async () => {
  render(<TagApproval term="local-first" similar={["本地优先"]} affected={["内容工作流.md", "架构设计.md"]} onApprove={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "批准新 Tag" }));
  expect(screen.getByText("+ local-first")).toBeInTheDocument();
  expect(screen.getByText("将更新 2 篇文章")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "批准并更新 2 篇文章" })).toBeInTheDocument();
});

test("默认模板不可用时回退 Default 并持续提示", () => {
  render(<TemplatePicker value="missing-template" templates={[{ id: "default", name: "InkHub Default", version: "1.0.0", compatible: true }, { id: "minimal", name: "InkHub Minimal", version: "1.0.0", compatible: true }]} onChange={vi.fn()} />);
  expect(screen.getByRole("status")).toHaveTextContent("已回退到 InkHub Default");
  expect(screen.getByRole("radio", { name: /InkHub Default/ })).toBeChecked();
});

test("Secret 字段只显示保存状态且不回显原值", () => {
  render(<SecretField label="API Key" saved />);
  expect(screen.getByText("已安全保存")).toBeInTheDocument();
  expect(screen.getByLabelText("API Key")).toHaveValue("");
  expect(screen.queryByDisplayValue(/sk-/)).not.toBeInTheDocument();
});
