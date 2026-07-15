import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";
import { ToastProvider } from "./ToastProvider";
import { useToast } from "./toast";

function Trigger() {
  const toast = useToast();
  return <button onClick={() => toast.show({ kind: "info", message: "该功能尚未开放" })}>触发提示</button>;
}

test("业务操作可以显示并关闭统一提示", async () => {
  render(<ToastProvider><Trigger /></ToastProvider>);

  await userEvent.click(screen.getByRole("button", { name: "触发提示" }));
  expect(screen.getByRole("status")).toHaveTextContent("该功能尚未开放");

  await userEvent.click(screen.getByRole("button", { name: "关闭提示" }));
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});
