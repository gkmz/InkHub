import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { SettingsPage } from "./SettingsPage";

test("已有工作区可以配置内容目录并触发重扫", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes("/settings/content-scope")) {
      expect(init?.method).toBe("PUT");
      expect(init?.body).toBe(JSON.stringify({ content_roots: ["Areas"], ignored_folders: [] }));
      return Response.json({ indexed: 12, failed: 0 });
    }
    return Response.json({
      workspace_name: "极客老墨",
      vault_path: "/Users/me/Vault",
      content_roots: [],
      ignored_folders: [],
      directories: [{ path: "Areas", markdown_count: 12 }],
      ai_enabled: false,
      ai_secret_saved: false,
      hugo_enabled: true,
      wechat_enabled: true,
      wechat_secret_saved: false,
      default_template: "default",
      templates: [],
      diagnostics: [],
    });
  });
  render(<SettingsPage />);

  expect(await screen.findByDisplayValue("极客老墨")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("checkbox", { name: "Areas（12 篇）" }));
  await userEvent.click(screen.getByRole("button", { name: "保存内容范围" }));

  expect(await screen.findByText("已索引 12 篇，失败 0 篇")).toBeInTheDocument();
});
