import { BookOpenText, LayoutDashboard, Settings, Tags } from "lucide-react";
import type { ReactNode } from "react";

interface AppShellProps {
  path: string;
  title: string;
  workspaceName: string;
  children: ReactNode;
  onNavigate: (path: string) => void;
  contentClassName?: string;
}

const links = [
  { path: "/", label: "工作台", icon: LayoutDashboard },
  { path: "/library", label: "内容库", icon: BookOpenText },
  { path: "/taxonomy", label: "类目管理", icon: Tags },
  { path: "/settings", label: "设置", icon: Settings },
];

/** AppShell 提供桌面侧栏和移动端底栏共用的四项主导航。 */
export function AppShell({ path, title, workspaceName, children, onNavigate, contentClassName = "" }: AppShellProps) {
  return (
    <div className="app-shell">
      <aside className="side-rail">
        <div className="brand"><span className="brand-mark">I</span><strong>InkHub</strong></div>
        <p className="workspace-label">当前工作区</p>
        <p className="workspace-name">{workspaceName}</p>
        <Navigation path={path} onNavigate={onNavigate} />
        <p className="rail-status"><span />本地内容已连接</p>
      </aside>
      <div className="main-column">
        <header className="topbar">
          <div><p className="mobile-brand">InkHub</p><h1>{title}</h1></div>
          <span className="scan-state">扫描已完成</span>
        </header>
        <main id="main-content" className={contentClassName} tabIndex={-1}>{children}</main>
      </div>
      <div className="mobile-nav"><Navigation path={path} onNavigate={onNavigate} /></div>
    </div>
  );
}

function Navigation({ path, onNavigate }: { path: string; onNavigate: (path: string) => void }) {
  return (
    <nav aria-label="主导航">
      {links.map(({ path: target, label, icon: Icon }) => {
        const active = path === target;
        return <a key={target} href={target} aria-label={label} aria-current={active ? "page" : undefined} onClick={(event) => { event.preventDefault(); onNavigate(target); }}><Icon aria-hidden="true" size={19} /><span>{label}</span></a>;
      })}
    </nav>
  );
}
