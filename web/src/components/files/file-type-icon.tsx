import type { LucideIcon } from 'lucide-react';
import {
  FileArchive,
  Code2 as FileCode,
  FileCog,
  FileIcon,
  FileImage,
  FileSpreadsheet,
  FileText,
  FileType2,
  Music,
  Video,
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface FileTypeIconProps {
  extension?: string | null;
  filename?: string | null;
  className?: string;
}

interface FileTypeConfig {
  Icon: LucideIcon;
  color: string;
}

function normalizeFileExtension(extension?: string | null, filename?: string | null) {
  const explicitExtension = extension?.replace(/^\./, '').trim();
  if (explicitExtension) return explicitExtension.toLowerCase();

  const name = filename?.trim() ?? '';
  const index = name.lastIndexOf('.');
  return index > -1 && index < name.length - 1 ? name.slice(index + 1).toLowerCase() : '';
}

export function getFileTypeConfig(extension?: string | null, filename?: string | null): FileTypeConfig {
  const ext = normalizeFileExtension(extension, filename);

  const configs: Record<string, FileTypeConfig> = {
    pdf: { Icon: FileText, color: 'text-rose-600' },
    doc: { Icon: FileText, color: 'text-blue-600' },
    docx: { Icon: FileText, color: 'text-blue-600' },
    txt: { Icon: FileText, color: 'text-slate-600' },
    md: { Icon: FileText, color: 'text-slate-600' },
    markdown: { Icon: FileText, color: 'text-slate-600' },
    mdx: { Icon: FileText, color: 'text-slate-600' },
    xls: { Icon: FileSpreadsheet, color: 'text-emerald-600' },
    xlsx: { Icon: FileSpreadsheet, color: 'text-emerald-600' },
    csv: { Icon: FileSpreadsheet, color: 'text-emerald-600' },
    ppt: { Icon: FileType2, color: 'text-orange-600' },
    pptx: { Icon: FileType2, color: 'text-orange-600' },
    jpg: { Icon: FileImage, color: 'text-pink-600' },
    jpeg: { Icon: FileImage, color: 'text-pink-600' },
    png: { Icon: FileImage, color: 'text-pink-600' },
    gif: { Icon: FileImage, color: 'text-pink-600' },
    webp: { Icon: FileImage, color: 'text-pink-600' },
    svg: { Icon: FileImage, color: 'text-pink-600' },
    bmp: { Icon: FileImage, color: 'text-pink-600' },
    tif: { Icon: FileImage, color: 'text-pink-600' },
    tiff: { Icon: FileImage, color: 'text-pink-600' },
    mp4: { Icon: Video, color: 'text-violet-600' },
    avi: { Icon: Video, color: 'text-violet-600' },
    mov: { Icon: Video, color: 'text-violet-600' },
    wmv: { Icon: Video, color: 'text-violet-600' },
    mkv: { Icon: Video, color: 'text-violet-600' },
    mp3: { Icon: Music, color: 'text-amber-600' },
    wav: { Icon: Music, color: 'text-amber-600' },
    flac: { Icon: Music, color: 'text-amber-600' },
    zip: { Icon: FileArchive, color: 'text-amber-600' },
    rar: { Icon: FileArchive, color: 'text-amber-600' },
    '7z': { Icon: FileArchive, color: 'text-amber-600' },
    tar: { Icon: FileArchive, color: 'text-amber-600' },
    gz: { Icon: FileArchive, color: 'text-amber-600' },
    js: { Icon: FileCode, color: 'text-indigo-600' },
    ts: { Icon: FileCode, color: 'text-indigo-600' },
    jsx: { Icon: FileCode, color: 'text-indigo-600' },
    tsx: { Icon: FileCode, color: 'text-indigo-600' },
    html: { Icon: FileCode, color: 'text-indigo-600' },
    css: { Icon: FileCode, color: 'text-indigo-600' },
    json: { Icon: FileCode, color: 'text-indigo-600' },
    xml: { Icon: FileCode, color: 'text-indigo-600' },
  };

  return configs[ext] || { Icon: ext ? FileCog : FileIcon, color: 'text-muted-foreground' };
}

export function FileTypeIcon({ extension, filename, className }: FileTypeIconProps) {
  const { Icon, color } = getFileTypeConfig(extension, filename);

  return <Icon className={cn(color, className)} />;
}
