'use client';

import React from 'react';
import type { LucideIcon } from 'lucide-react';
import {
  FileText,
  FileImage,
  Music,
  Video,
  Code2 as Code,
  FileArchive,
  FileSpreadsheet,
  FileType2,
  FileCog,
} from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * Map common file extensions to Lucide icons.
 * Extend this map as needed to support more file types.
 */
const extensionIconMap: Record<string, LucideIcon> = {
  // Documents
  doc: FileText,
  docx: FileText,
  txt: FileText,
  md: FileText,
  pdf: FileText,
  // Spreadsheets
  xls: FileSpreadsheet,
  xlsx: FileSpreadsheet,
  csv: FileSpreadsheet,
  // Presentations
  ppt: FileType2,
  pptx: FileType2,
  // Archives
  zip: FileArchive,
  rar: FileArchive,
  '7z': FileArchive,
  tar: FileArchive,
  gz: FileArchive,
  // Images
  jpg: FileImage,
  jpeg: FileImage,
  png: FileImage,
  gif: FileImage,
  bmp: FileImage,
  svg: FileImage,
  webp: FileImage,
  // Audio
  mp3: Music,
  wav: Music,
  flac: Music,
  // Video
  mp4: Video,
  mov: Video,
  avi: Video,
  mkv: Video,
  // Code
  js: Code,
  ts: Code,
  jsx: Code,
  tsx: Code,
  html: Code,
  css: Code,
  json: Code,
};

/**
 * Map extension to suggested text color classes.
 */
const extensionColorMap: Record<string, string> = {
  pdf: 'text-red-600',
  doc: 'text-blue-600',
  docx: 'text-blue-600',
  txt: 'text-gray-600',
  md: 'text-gray-600',
  xls: 'text-green-600',
  xlsx: 'text-green-600',
  csv: 'text-green-600',
  ppt: 'text-orange-600',
  pptx: 'text-orange-600',
  zip: 'text-yellow-600',
  rar: 'text-yellow-600',
  '7z': 'text-yellow-600',
  tar: 'text-yellow-600',
  gz: 'text-yellow-600',
  jpg: 'text-pink-600',
  jpeg: 'text-pink-600',
  png: 'text-pink-600',
  gif: 'text-pink-600',
  bmp: 'text-pink-600',
  svg: 'text-pink-600',
  webp: 'text-pink-600',
  mp3: 'text-amber-600',
  wav: 'text-amber-600',
  flac: 'text-amber-600',
  mp4: 'text-purple-600',
  mov: 'text-purple-600',
  avi: 'text-purple-600',
  mkv: 'text-purple-600',
  js: 'text-indigo-600',
  ts: 'text-indigo-600',
  jsx: 'text-indigo-600',
  tsx: 'text-indigo-600',
  html: 'text-indigo-600',
  css: 'text-indigo-600',
  json: 'text-indigo-600',
};

export interface FileIconProps extends React.HTMLAttributes<SVGSVGElement> {
  /** Filename including extension, e.g., "report.pdf" */
  filename?: string;
  /** File extension without dot, e.g., "pdf". Takes precedence over filename. */
  extension?: string;
  /** Icon size. Accepts preset keywords or explicit number (pixels). */
  size?: 'sm' | 'md' | 'lg' | number;
}

/**
 * FileIcon – shows an icon that represents the file type based on extension.
 *
 * Example usage:
 * ```tsx
 * <FileIcon filename="report.pdf" />
 * ```
 */
export function FileIcon({ filename, extension, size = 'md', className, ...props }: FileIconProps) {
  const ext = (extension || filename?.split('.').pop() || '').toLowerCase();

  // Determine icon component
  const Icon: LucideIcon = extensionIconMap[ext] || FileCog;

  // Determine size in pixels
  const pixelSize = typeof size === 'number' ? size : size === 'sm' ? 16 : size === 'lg' ? 28 : 20;

  // Determine color class
  const colorClass = extensionColorMap[ext] || 'text-muted-foreground';

  return (
    <Icon
      className={cn(colorClass, className)}
      width={pixelSize}
      height={pixelSize}
      {...props}
    />
  );
} 