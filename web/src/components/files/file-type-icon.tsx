import type { ComponentType } from 'react';
import {
  FileArchive,
  FileAudio,
  FileCode,
  FileIcon,
  FileMusic,
  FileSpreadsheet,
  FileText,
  Image as ImageIcon,
  Video,
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface FileTypeIconProps {
  extension?: string | null;
  filename?: string | null;
  className?: string;
}

interface FileTypeConfig {
  Icon: ComponentType<{ className?: string }>;
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
    pdf: { Icon: FileText, color: 'text-muted-foreground' },
    doc: { Icon: FileText, color: 'text-muted-foreground' },
    docx: { Icon: FileText, color: 'text-muted-foreground' },
    txt: { Icon: FileText, color: 'text-muted-foreground' },
    md: { Icon: FileText, color: 'text-muted-foreground' },
    xls: { Icon: FileSpreadsheet, color: 'text-muted-foreground' },
    xlsx: { Icon: FileSpreadsheet, color: 'text-muted-foreground' },
    csv: { Icon: FileSpreadsheet, color: 'text-muted-foreground' },
    jpg: { Icon: ImageIcon, color: 'text-warning' },
    jpeg: { Icon: ImageIcon, color: 'text-warning' },
    png: { Icon: ImageIcon, color: 'text-warning' },
    gif: { Icon: ImageIcon, color: 'text-warning' },
    webp: { Icon: ImageIcon, color: 'text-warning' },
    svg: { Icon: ImageIcon, color: 'text-warning' },
    mp4: { Icon: Video, color: 'text-muted-foreground' },
    avi: { Icon: Video, color: 'text-muted-foreground' },
    mov: { Icon: Video, color: 'text-muted-foreground' },
    wmv: { Icon: Video, color: 'text-muted-foreground' },
    mp3: { Icon: FileMusic, color: 'text-muted-foreground' },
    wav: { Icon: FileAudio, color: 'text-muted-foreground' },
    zip: { Icon: FileArchive, color: 'text-muted-foreground' },
    rar: { Icon: FileArchive, color: 'text-muted-foreground' },
    '7z': { Icon: FileArchive, color: 'text-muted-foreground' },
    js: { Icon: FileCode, color: 'text-muted-foreground' },
    ts: { Icon: FileCode, color: 'text-muted-foreground' },
    jsx: { Icon: FileCode, color: 'text-muted-foreground' },
    tsx: { Icon: FileCode, color: 'text-muted-foreground' },
    json: { Icon: FileCode, color: 'text-muted-foreground' },
  };

  return configs[ext] || { Icon: FileIcon, color: 'text-muted-foreground' };
}

export function FileTypeIcon({ extension, filename, className }: FileTypeIconProps) {
  const { Icon, color } = getFileTypeConfig(extension, filename);

  return <Icon className={cn(color, className)} />;
}
