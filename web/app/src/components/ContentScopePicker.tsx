import { Folder, MinusCircle } from "lucide-react";
import { useMemo, useState } from "react";
import type { DirectoryCandidate } from "../api/types";

interface ContentScopePickerProps {
  directories: DirectoryCandidate[];
  contentRoots: string[];
  ignoredFolders: string[];
  onChange: (contentRoots: string[], ignoredFolders: string[]) => void;
}

/** ContentScopePicker 用两个固定层级表达管理目录和内部忽略目录。 */
export function ContentScopePicker({ directories, contentRoots, ignoredFolders, onChange }: ContentScopePickerProps) {
  const [ignoreCandidate, setIgnoreCandidate] = useState("");
  const visibleDirectories = useMemo(() => directories.filter((directory) =>
    contentRoots.includes(directory.path)
    || !contentRoots.some((root) => directory.path !== root && isWithin(directory.path, root))), [contentRoots, directories]);
  const availableIgnored = useMemo(() => directories.filter((directory) =>
    contentRoots.some((root) => directory.path !== root && isWithin(directory.path, root))
    && !ignoredFolders.includes(directory.path)), [contentRoots, directories, ignoredFolders]);

  const toggleRoot = (path: string) => {
    const selected = contentRoots.includes(path)
      ? contentRoots.filter((root) => root !== path)
      : contentRoots.some((root) => isWithin(path, root))
        ? contentRoots
        : [...contentRoots.filter((root) => !isWithin(root, path)), path].sort();
    const retainedIgnored = ignoredFolders.filter((ignored) => selected.some((root) => isWithin(ignored, root)));
    onChange(selected, retainedIgnored);
  };

  const addIgnored = () => {
    if (!ignoreCandidate) return;
    onChange(contentRoots, [...ignoredFolders, ignoreCandidate].sort());
    setIgnoreCandidate("");
  };

  return <section className="content-scope-picker" aria-labelledby="content-scope-heading">
    <header><Folder size={18} /><div><h2 id="content-scope-heading">管理这些目录</h2><p>目录中的 Markdown 会递归加入内容库</p></div></header>
    <div className="scope-directory-list">{visibleDirectories.map((directory) => <label key={directory.path}>
      <input type="checkbox" aria-label={`${directory.path}（${directory.markdown_count} 篇）`} checked={contentRoots.includes(directory.path)} onChange={() => toggleRoot(directory.path)} />
      <span>{directory.path}</span><small>{directory.markdown_count} 篇</small>
    </label>)}</div>
    {contentRoots.length === 0
      ? <p className="scope-empty">尚未选择内容目录，InkHub 不会扫描任何笔记。</p>
      : <div className="scope-ignore"><h3>其中不管理这些目录</h3>
        <div className="scope-ignore-add"><select aria-label="要忽略的子目录" value={ignoreCandidate} onChange={(event) => setIgnoreCandidate(event.target.value)}><option value="">选择子目录</option>{availableIgnored.map((directory) => <option key={directory.path} value={directory.path}>{directory.path}（{directory.markdown_count} 篇）</option>)}</select><button type="button" className="secondary" disabled={!ignoreCandidate} onClick={addIgnored}>添加忽略目录</button></div>
        <ul>{ignoredFolders.map((path) => <li key={path}><span>{path}</span><button type="button" aria-label={`恢复管理 ${path}`} onClick={() => onChange(contentRoots, ignoredFolders.filter((item) => item !== path))}><MinusCircle size={16} /></button></li>)}</ul>
      </div>}
  </section>;
}

function isWithin(candidate: string, root: string) {
  return candidate === root || candidate.startsWith(`${root}/`);
}
