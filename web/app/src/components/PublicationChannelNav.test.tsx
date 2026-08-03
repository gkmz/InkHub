import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, test, vi } from "vitest";
import { PublicationChannelNav } from "./PublicationChannelNav";

const approvedArticle = {
  id: "a1",
  review_state: "已通过",
  hugo_provider_id: "h1",
  wechat_provider_id: "w1",
  hugo_state: "需要同步",
  wechat_state: "尚未准备",
  xiaohongshu_state: "尚未准备",
};

describe("PublicationChannelNav", () => {
  test("审核通过后三个发布渠道可以独立进入", async () => {
    const navigate = vi.fn();
    render(<PublicationChannelNav article={approvedArticle} active="hugo" onNavigate={navigate} />);

    expect(screen.getByRole("button", { name: /审核/ })).toBeEnabled();
    expect(screen.getByRole("button", { name: /同步到 Hugo/ })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: /发布到微信/ })).toBeEnabled();
    expect(screen.getByRole("button", { name: /发布到小红书/ })).toBeEnabled();

    await userEvent.click(screen.getByRole("button", { name: /发布到小红书/ }));
    expect(navigate).toHaveBeenCalledWith("/articles/a1/xiaohongshu");
  });

  test("审核未通过时保留渠道状态但禁止开始发布", () => {
    render(<PublicationChannelNav article={{ ...approvedArticle, review_state: "待审核" }} active="review" onNavigate={vi.fn()} />);

    expect(screen.getByRole("button", { name: /同步到 Hugo.*审核通过后可用/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /发布到微信.*审核通过后可用/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /发布到小红书.*审核通过后可用/ })).toBeDisabled();
  });

  test("渠道未配置时进入设置而不是无效的发布页面", async () => {
    const navigate = vi.fn();
    render(<PublicationChannelNav article={{ ...approvedArticle, hugo_provider_id: "", wechat_provider_id: "" }} active="review" onNavigate={navigate} />);

    expect(screen.getByRole("button", { name: /配置Hugo.*未配置/ })).toBeEnabled();
    expect(screen.getByRole("button", { name: /配置微信.*未配置/ })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: /配置Hugo/ }));
    expect(navigate).toHaveBeenCalledWith("/settings");
  });
});
