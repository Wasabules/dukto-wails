// Shared presentation helpers consumed by the received list, the composer,
// and the progress toasts. Pure functions only so components can import
// without side effects.

import { mediaKindFromName } from './dukto';

export function humanBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

export function formatEta(seconds: number): string {
  if (!isFinite(seconds) || seconds > 86400) return '—';
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export function basename(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
  return i < 0 ? path : path.slice(i + 1);
}

// fileIcon picks a single emoji summary for a file name. It falls back to
// the generic folder glyph so the UI never has a blank cell.
export function fileIcon(name: string): string {
  const kind = mediaKindFromName(name);
  if (kind === 'image') return '🖼';
  if (kind === 'video') return '🎬';
  if (kind === 'audio') return '🎵';
  const i = name.lastIndexOf('.');
  const ext = i < 0 ? '' : name.slice(i + 1).toLowerCase();
  if (['zip', 'rar', '7z', 'tar', 'gz', 'xz', 'bz2'].includes(ext)) return '🗜';
  if (ext === 'pdf') return '📕';
  if (['doc', 'docx', 'odt', 'rtf'].includes(ext)) return '📄';
  if (['xls', 'xlsx', 'ods', 'csv'].includes(ext)) return '📊';
  if (['txt', 'md', 'log'].includes(ext)) return '📝';
  if (['exe', 'msi', 'dmg', 'deb', 'rpm', 'apk'].includes(ext)) return '⚙';
  if (['html', 'js', 'ts', 'css', 'json', 'xml', 'yaml', 'yml', 'go', 'py', 'rs', 'c', 'cpp', 'h', 'java', 'svelte'].includes(ext)) return '📝';
  return '📁';
}
