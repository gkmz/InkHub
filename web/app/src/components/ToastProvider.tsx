import { CheckCircle2, Info, X, XCircle } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ToastContext } from "./toast";
import type { ToastAPI, ToastInput } from "./toast";

interface ToastMessage extends ToastInput {
  id: number;
}

// ToastProvider 统一承载页面操作反馈，避免各业务页面重复维护提示状态。
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [messages, setMessages] = useState<ToastMessage[]>([]);
  const nextID = useRef(1);
  const api = useMemo<ToastAPI>(() => ({
    show: (input) => setMessages((current) => [...current, { ...input, id: nextID.current++ }]),
  }), []);
  const dismiss = useCallback((id: number) => setMessages((current) => current.filter((message) => message.id !== id)), []);

  return <ToastContext.Provider value={api}>
    {children}
    <div className="toast-region" role="region" aria-label="操作提示">
      {messages.map((message) => <ToastItem key={message.id} message={message} onDismiss={dismiss} />)}
    </div>
  </ToastContext.Provider>;
}

function ToastItem({ message, onDismiss }: { message: ToastMessage; onDismiss: (id: number) => void }) {
  useEffect(() => {
    const timer = window.setTimeout(() => onDismiss(message.id), 4500);
    return () => window.clearTimeout(timer);
  }, [message.id, onDismiss]);
  const Icon = message.kind === "success" ? CheckCircle2 : message.kind === "error" ? XCircle : Info;
  return <div className={`toast toast-${message.kind}`} role={message.kind === "error" ? "alert" : "status"}>
    <Icon size={18} />
    <span>{message.message}</span>
    <button type="button" aria-label="关闭提示" onClick={() => onDismiss(message.id)}><X size={16} /></button>
  </div>;
}
