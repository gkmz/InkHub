import { createContext, useContext } from "react";

export type ToastKind = "info" | "success" | "error";

export interface ToastInput {
  kind: ToastKind;
  message: string;
}

export interface ToastAPI {
  show: (input: ToastInput) => void;
}

export const ToastContext = createContext<ToastAPI | null>(null);

// useToast 返回全局操作提示入口，只能在 ToastProvider 内调用。
export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast 必须在 ToastProvider 内使用");
  return context;
}
