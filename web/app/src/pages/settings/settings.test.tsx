import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { SettingsPage } from "./SettingsPage";
import { ToastProvider } from "../../components/ToastProvider";

function renderSettings() {
  return render(<ToastProvider><SettingsPage /></ToastProvider>);
}

test("已有工作区可以配置内容目录并触发重扫", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes("/settings/content-scope/preview")) return Response.json({ added: 12, removed: 1 });
    if (url.includes("/settings/content-scope")) {
      expect(init?.method).toBe("PUT");
      expect(init?.body).toBe(JSON.stringify({ content_roots: ["Areas"], ignored_folders: [], ignored_file_names: ["index.md", "_index.md"] }));
      return Response.json({ indexed: 12, failed: 0 });
    }
    return Response.json({
      workspace_name: "极客老墨",
      vault_path: "/Users/me/Vault",
      content_roots: [],
      ignored_folders: [],
      ignored_file_names: ["index.md", "_index.md"],
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
  renderSettings();

  expect(await screen.findByDisplayValue("极客老墨")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("checkbox", { name: "Areas（12 篇）" }));
  await userEvent.click(screen.getByRole("button", { name: "预览内容范围变更" }));
  expect(await screen.findByText("将新增 12 篇，移出 1 篇。源文件不会被修改。")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "确认并重扫" }));

  expect(await screen.findByText("已索引 12 篇，失败 0 篇")).toBeInTheDocument();
});

test("重新诊断会再次请求设置并刷新诊断结果", async () => {
  let settingsRequests = 0;
  vi.spyOn(globalThis, "fetch").mockImplementation(async () => {
    settingsRequests += 1;
    return Response.json({
      workspace_name: "极客老墨",
      vault_path: "/Users/me/Vault",
      content_roots: ["Areas"],
      ignored_folders: [],
      ignored_file_names: ["index.md", "_index.md"],
      directories: [],
      ai_enabled: false,
      ai_secret_saved: false,
      hugo_enabled: true,
      wechat_enabled: true,
      wechat_secret_saved: false,
      default_template: "default",
      templates: [],
      diagnostics: settingsRequests === 1
        ? [{ name: "内容目录", state: "需要处理", message: "暂时无法读取" }]
        : [{ name: "内容目录", state: "正常", message: "路径可读" }],
    });
  });
  renderSettings();

  expect(await screen.findByText("暂时无法读取")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "重新诊断" }));

  expect(await screen.findByText("路径可读")).toBeInTheDocument();
  expect(await screen.findByRole("status")).toHaveTextContent("诊断已更新");
  expect(settingsRequests).toBe(2);
});

test("重新诊断失败时显示错误且保留原诊断", async () => {
  let settingsRequests = 0;
  vi.spyOn(globalThis, "fetch").mockImplementation(async () => {
    settingsRequests += 1;
    if (settingsRequests > 1) throw new Error("网络不可用");
    return Response.json({
      workspace_name: "极客老墨",
      vault_path: "/Users/me/Vault",
      content_roots: ["Areas"],
      ignored_folders: [],
      ignored_file_names: ["index.md"],
      directories: [],
      ai_enabled: false,
      ai_secret_saved: false,
      hugo_enabled: false,
      wechat_enabled: false,
      wechat_secret_saved: false,
      default_template: "default",
      templates: [],
      diagnostics: [{ name: "内容目录", state: "正常", message: "原诊断仍有效" }],
    });
  });
  renderSettings();

  expect(await screen.findByText("原诊断仍有效")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "重新诊断" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("网络不可用");
  expect(screen.getByText("原诊断仍有效")).toBeInTheDocument();
});

test("AI 设置保存到 Provider 且不要求重新输入已有 Secret", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "PUT") {
      expect(init.body).toBe(JSON.stringify({ enabled: true, base_url: "https://ai.example.com/v1", model: "model-1", api_key: "" }));
      return Response.json({ ai_enabled: true, ai_secret_saved: true });
    }
    return Response.json({
    workspace_name: "极客老墨",
    vault_path: "/Users/me/Vault",
    content_roots: ["Areas"],
    ignored_folders: [],
    ignored_file_names: ["index.md"],
    directories: [],
    ai_enabled: false,
    ai_secret_saved: true,
    ai_base_url: "https://ai.example.com/v1",
    ai_model: "model-1",
    hugo_enabled: true,
    wechat_enabled: true,
    wechat_secret_saved: true,
    default_template: "default",
    templates: [],
    diagnostics: [],
    });
  });
  renderSettings();

  await screen.findByDisplayValue("极客老墨");
  await userEvent.click(screen.getByRole("checkbox", { name: "启用 AI 建议" }));
  await userEvent.click(screen.getByRole("button", { name: "保存 AI 设置" }));
  expect(await screen.findByRole("status")).toHaveTextContent("AI 设置已保存");
  expect(fetchMock).toHaveBeenCalledTimes(2);
});

test("微信图片仓库保存到 Provider 且 Token 不留在表单", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "PUT") {
      expect(init.body).toBe(JSON.stringify({ enabled: true, template: "default", github_owner: "gkmz", github_repository: "images", github_branch: "main", github_prefix: "inkhub", github_token: "secret-token" }));
      return Response.json({ wechat_enabled: true, default_template: "default", github_owner: "gkmz", github_repository: "images", github_branch: "main", github_prefix: "inkhub", github_token_saved: true });
    }
    return Response.json({
      workspace_name: "极客老墨", vault_path: "/Users/me/Vault", content_roots: ["Areas"], ignored_folders: [], ignored_file_names: ["index.md"], directories: [], ai_enabled: false, ai_secret_saved: false, hugo_enabled: true, wechat_enabled: true, wechat_secret_saved: false, github_token_saved: false, github_owner: "", github_repository: "", github_branch: "main", github_prefix: "inkhub", default_template: "default", templates: [], diagnostics: [],
    });
  });
  renderSettings();

  await screen.findByDisplayValue("极客老墨");
  await userEvent.type(screen.getByRole("textbox", { name: "GitHub Owner" }), "gkmz");
  await userEvent.type(screen.getByRole("textbox", { name: "Repository" }), "images");
  await userEvent.type(screen.getByLabelText("GitHub Token"), "secret-token");
  await userEvent.click(screen.getByRole("button", { name: "保存发布设置" }));
  expect(await screen.findByRole("status")).toHaveTextContent("发布设置已保存");
  expect(screen.getByLabelText("GitHub Token")).toHaveValue("");
  expect(fetchMock).toHaveBeenCalledTimes(2);
});
