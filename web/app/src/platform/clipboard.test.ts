import { afterEach, expect, test, vi } from "vitest";
import { copyFormattedHTML } from "./clipboard";

const originalClipboard = Object.getOwnPropertyDescriptor(Navigator.prototype, "clipboard");
const originalExecCommand = Object.getOwnPropertyDescriptor(document, "execCommand");

afterEach(() => {
  vi.unstubAllGlobals();
  if (originalClipboard) Object.defineProperty(Navigator.prototype, "clipboard", originalClipboard);
  else Reflect.deleteProperty(Navigator.prototype, "clipboard");
  if (originalExecCommand) Object.defineProperty(document, "execCommand", originalExecCommand);
  else Reflect.deleteProperty(document, "execCommand");
});

test("优先把微信内容作为 HTML 富文本写入剪贴板", async () => {
  const write = vi.fn().mockResolvedValue(undefined);
  class TestClipboardItem {
    constructor(readonly data: Record<string, Blob>) {}
  }
  Object.defineProperty(Navigator.prototype, "clipboard", { configurable: true, value: { write } });
  vi.stubGlobal("ClipboardItem", TestClipboardItem);

  await copyFormattedHTML("<p>正文<strong>重点</strong></p>");

  expect(write).toHaveBeenCalledOnce();
  const item = write.mock.calls[0][0][0] as TestClipboardItem;
  expect(item.data["text/html"].type).toBe("text/html");
  expect(item.data["text/plain"].type).toBe("text/plain");
});

test("剪贴板权限被拒绝时降级为浏览器选区复制", async () => {
  const write = vi.fn().mockRejectedValue(new DOMException("denied", "NotAllowedError"));
  const execCommand = vi.fn().mockReturnValue(true);
  Object.defineProperty(Navigator.prototype, "clipboard", { configurable: true, value: { write } });
  Object.defineProperty(document, "execCommand", { configurable: true, value: execCommand });
  vi.stubGlobal("ClipboardItem", class { constructor(readonly data: Record<string, Blob>) {} });

  await copyFormattedHTML("<p><strong>格式化正文</strong></p>");

  expect(write).toHaveBeenCalledOnce();
  expect(execCommand).toHaveBeenCalledWith("copy");
  expect(document.querySelector("[data-inkhub-clipboard]")).not.toBeInTheDocument();
});

test("现代和降级复制都失败时返回明确错误", async () => {
  Object.defineProperty(Navigator.prototype, "clipboard", { configurable: true, value: { write: vi.fn().mockRejectedValue(new Error("denied")) } });
  Object.defineProperty(document, "execCommand", { configurable: true, value: vi.fn().mockReturnValue(false) });
  vi.stubGlobal("ClipboardItem", class { constructor(readonly data: Record<string, Blob>) {} });

  await expect(copyFormattedHTML("<p>正文</p>")).rejects.toThrow("无法写入格式化内容");
});
