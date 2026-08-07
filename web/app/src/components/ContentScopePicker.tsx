import { Folder, MinusCircle } from "lucide-react";
import { useMemo, useState } from "react";
import type { DirectoryCandidate } from "../api/types";

interface ContentScopePickerProps {
  directories: DirectoryCandidate[];
  contentRoots: string[];
  ignoredFolders: string[];
  ignoredFileNames: string[];
  showHeading?: boolean;
  onChange: (contentRoots: string[], ignoredFolders: string[], ignoredFileNames: string[]) => void;
}

/** ContentScopePicker 用两个固定层级表达管理目录和内部忽略目录。 */
export function ContentScopePicker({ directories, contentRoots, ignoredFolders, ignoredFileNames, showHeading = true, onChange }: ContentScopePickerProps) {
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

  return <section className="content-scope-picker" aria-labelledby={showHeading ? "content-scope-heading" : undefined} aria-label={showHeading ? undefined : "内容范围"}>
    {showHeading && <header><Folder size={18} /><div><h2 id="content-scope-heading">管理这些目录</h2><p>目录中的 Markdown 会递归加入内容库</p></div></header>}
    <div className="scope-current-grid">
      <section className="included" aria-label="当前管理目录"><header><h3>已纳入内容库</h3><span>{roots.length}</span></header>{roots.length === 0 ? <p>尚未选择目录</p> : <ul>{roots.map((root) => <li key={root}><span>{root}</span><small>{directoryCount(directories, root)}</small><button type="button" aria-label={`移除管理目录 ${root}`} onClick={() => {
        const selected = roots.filter((item) => item !== root);
        onChange(selected, ignored.filter((item) => selected.some((selectedRoot) => isWithin(item, selectedRoot))), fileNames);
      }}><MinusCircle size={15} /></button></li>)}</ul>}</section>
      <section className="excluded" aria-label="当前排除目录"><header><h3>已排除子目录</h3><span>{ignored.length}</span></header>{ignored.length === 0 ? <p>没有排除目录</p> : <ul>{ignored.map((path) => <li key={path}><span>{path}</span><small>{directoryCount(directories, path)}</small><button type="button" aria-label={`移除排除目录 ${path}`} onClick={() => onChange(roots, ignored.filter((item) => item !== path), fileNames)}><MinusCircle size={15} /></button></li>)}</ul>}</section>
    </div>
    <div className="scope-control-section"><header><div><h3>选择管理目录</h3><p>勾选顶层目录后，其中的 Markdown 会递归进入内容库</p></div><span>{directories.length} 个目录</span></header>
      <input className="scope-search" aria-label="搜索管理目录" placeholder="搜索目录" value={rootSearch} onChange={(event) => setRootSearch(event.target.value)} />
      <div className="scope-directory-list">{visibleDirectories.map((directory) => <label key={directory.path}>
        <input type="checkbox" aria-label={`${directory.path}（${directory.markdown_count} 篇）`} checked={roots.includes(directory.path)} onChange={() => toggleRoot(directory.path)} />
        <span>{directory.path}</span><small>{directory.markdown_count} 篇</small>
      </label>)}</div>
    </div>
    {roots.length === 0
      ? <p className="scope-empty">尚未选择内容目录，InkHub 不会扫描任何笔记。</p>
      : <div className="scope-ignore scope-control-section"><header><div><h3>排除子目录</h3><p>只影响已纳入目录内部，不会删除源文件</p></div><span>{ignoredCandidates.length} 个可选</span></header>
        <input className="scope-search" aria-label="搜索忽略目录" placeholder="搜索子目录" value={ignoredSearch} onChange={(event) => setIgnoredSearch(event.target.value)} />
        <div className="scope-directory-list">{ignoredCandidates.map((directory) => <label key={directory.path}><input type="checkbox" aria-label={`忽略 ${directory.path}（${directory.markdown_count} 篇）`} checked={ignored.includes(directory.path)} onChange={() => toggleIgnored(directory.path)} /><span>{directory.path}</span><small>{directory.markdown_count} 篇</small></label>)}</div>
        <div className="scope-file-names"><header><div><h3>忽略文件名</h3><p>对所有已管理目录精确匹配，大小写不敏感</p></div><span>{fileNames.length} 条规则</span></header><div className="scope-ignore-add"><input aria-label="添加忽略文件名" placeholder="例如 index.md" value={fileNameDraft} onChange={(event) => setFileNameDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") addFileName(); }} /><button type="button" className="secondary" onClick={addFileName}>添加文件名</button></div><ul>{fileNames.map((name) => <li key={name}><span>{name}</span><button type="button" aria-label={`删除忽略文件名 ${name}`} onClick={() => onChange(roots, ignored, fileNames.filter((item) => item !== name))}><MinusCircle size={16} /></button></li>)}</ul></div>
      </div>}
  </section>;
}

function isWithin(candidate: string, root: string) {
  return candidate === root || candidate.startsWith(`${root}/`);
}

function directoryCount(directories: DirectoryCandidate[], path: string) {
  const directory = directories.find((candidate) => candidate.path === path);
  return directory ? `${directory.markdown_count} 篇` : "数量待扫描";
}
