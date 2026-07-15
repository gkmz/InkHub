import { Folder, MinusCircle } from "lucide-react";
import { useMemo, useState } from "react";
import type { DirectoryCandidate } from "../api/types";

interface ContentScopePickerProps {
  directories: DirectoryCandidate[];
  contentRoots: string[];
  ignoredFolders: string[];
  ignoredFileNames: string[];
  onChange: (contentRoots: string[], ignoredFolders: string[], ignoredFileNames: string[]) => void;
}

/** ContentScopePicker 用两个固定层级表达管理目录和内部忽略目录。 */
export function ContentScopePicker({ directories, contentRoots, ignoredFolders, ignoredFileNames, onChange }: ContentScopePickerProps) {
  const roots = useMemo(() => contentRoots ?? [], [contentRoots]);
  const ignored = useMemo(() => ignoredFolders ?? [], [ignoredFolders]);
  const fileNames = useMemo(() => ignoredFileNames ?? [], [ignoredFileNames]);
  const [rootSearch, setRootSearch] = useState("");
  const [ignoredSearch, setIgnoredSearch] = useState("");
  const [fileNameDraft, setFileNameDraft] = useState("");
  const visibleDirectories = useMemo(() => directories.filter((directory) =>
    roots.includes(directory.path)
    || !roots.some((root) => directory.path !== root && isWithin(directory.path, root))
    && directory.path.toLowerCase().includes(rootSearch.trim().toLowerCase())), [roots, directories, rootSearch]);
  const ignoredCandidates = useMemo(() => directories.filter((directory) =>
    roots.some((root) => directory.path !== root && isWithin(directory.path, root))
    && directory.path.toLowerCase().includes(ignoredSearch.trim().toLowerCase())), [roots, directories, ignoredSearch]);

  const toggleRoot = (path: string) => {
    const selected = roots.includes(path)
      ? roots.filter((root) => root !== path)
      : roots.some((root) => isWithin(path, root))
        ? roots
        : [...roots.filter((root) => !isWithin(root, path)), path].sort();
    const retainedIgnored = ignored.filter((item) => selected.some((root) => isWithin(item, root)));
    onChange(selected, retainedIgnored, fileNames);
  };

  const toggleIgnored = (path: string) => onChange(roots, ignored.includes(path) ? ignored.filter((item) => item !== path) : [...ignored, path].sort(), fileNames);
  const addFileName = () => {
    const value = fileNameDraft.trim().toLowerCase();
    if (!value || value.includes("/") || !value.endsWith(".md") || fileNames.includes(value)) return;
    onChange(roots, ignored, [...fileNames, value].sort());
    setFileNameDraft("");
  };

  return <section className="content-scope-picker" aria-labelledby="content-scope-heading">
    <header><Folder size={18} /><div><h2 id="content-scope-heading">管理这些目录</h2><p>目录中的 Markdown 会递归加入内容库</p></div></header>
    <input className="scope-search" aria-label="搜索管理目录" placeholder="搜索目录" value={rootSearch} onChange={(event) => setRootSearch(event.target.value)} />
    <div className="scope-directory-list">{visibleDirectories.map((directory) => <label key={directory.path}>
      <input type="checkbox" aria-label={`${directory.path}（${directory.markdown_count} 篇）`} checked={roots.includes(directory.path)} onChange={() => toggleRoot(directory.path)} />
      <span>{directory.path}</span><small>{directory.markdown_count} 篇</small>
    </label>)}</div>
    {roots.length === 0
      ? <p className="scope-empty">尚未选择内容目录，InkHub 不会扫描任何笔记。</p>
      : <div className="scope-ignore"><h3>其中不管理这些目录</h3>
        <input className="scope-search" aria-label="搜索忽略目录" placeholder="搜索子目录" value={ignoredSearch} onChange={(event) => setIgnoredSearch(event.target.value)} />
        <div className="scope-directory-list">{ignoredCandidates.map((directory) => <label key={directory.path}><input type="checkbox" aria-label={`忽略 ${directory.path}（${directory.markdown_count} 篇）`} checked={ignored.includes(directory.path)} onChange={() => toggleIgnored(directory.path)} /><span>{directory.path}</span><small>{directory.markdown_count} 篇</small></label>)}</div>
        <div className="scope-file-names"><h3>忽略文件名</h3><p>对所有已管理目录精确匹配，大小写不敏感</p><div className="scope-ignore-add"><input aria-label="添加忽略文件名" placeholder="例如 index.md" value={fileNameDraft} onChange={(event) => setFileNameDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") addFileName(); }} /><button type="button" className="secondary" onClick={addFileName}>添加文件名</button></div><ul>{fileNames.map((name) => <li key={name}><span>{name}</span><button type="button" aria-label={`删除忽略文件名 ${name}`} onClick={() => onChange(roots, ignored, fileNames.filter((item) => item !== name))}><MinusCircle size={16} /></button></li>)}</ul></div>
      </div>}
  </section>;
}

function isWithin(candidate: string, root: string) {
  return candidate === root || candidate.startsWith(`${root}/`);
}
