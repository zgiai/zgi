'use client';

import { useCallback, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import {
  MoreHorizontal,
  FileText,
  Download,
  Eye,
  Trash2,
  RotateCcw,
  ExternalLink,
  ArrowDown,
  ListFilter,
  Search,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { DocumentStatusBadge } from './document-status-badge';
import { ExtractionFallbackNotice } from './extraction-fallback-notice';
import { cn } from '@/lib/utils';
import type {
  Document,
  DocumentIndexingStatus,
  DocumentStatus,
  DocumentDisplayStatus,
} from '@/services/types/dataset';
import { formatDate, formatNumber } from '@/utils/format';

interface DocumentTableProps {
  documents: Document[];
  datasetId: string;
  // actions
  onDelete?: (document: Document) => void;
  onDownload?: (document: Document) => void;
  onReprocess?: (document: Document) => void;
  onToggleEnabled?: (documentId: string, enabled: boolean) => void;
  // selection
  selectedIds?: string[];
  onToggleSelect?: (id: string) => void;
  onToggleSelectAll?: (checked: boolean, idsOnPage: string[]) => void;
  // sorting
  sortField?: 'created_at' | 'updated_at' | '-created_at' | '-updated_at';
  onSortChange?: (field: 'created_at' | 'updated_at' | '-created_at' | '-updated_at') => void;
  // status filter
  statusFilter?: DocumentIndexingStatus[keyof DocumentIndexingStatus] | 'all';
  onStatusFilterChange?: (
    value: DocumentIndexingStatus[keyof DocumentIndexingStatus] | 'all'
  ) => void;
  // misc
  isMutatingEnabled?: string[];
  className?: string;
  /** Whether user has edit permission - hides delete/enable/disable actions when false */
  canEdit?: boolean;
}

export function DocumentTable({
  documents,
  datasetId,
  onDelete,
  onDownload,
  onReprocess,
  onToggleEnabled,
  selectedIds = [],
  onToggleSelect,
  onToggleSelectAll,
  sortField = '-created_at',
  onSortChange,
  statusFilter = 'all',
  onStatusFilterChange,
  isMutatingEnabled = [],
  className,
  canEdit = true,
}: DocumentTableProps) {
  const t = useT('datasets');
  const tCommon = useT('common');
  const router = useRouter();
  const [activeDropdown, setActiveDropdown] = useState<string | null>(null);

  // Formatting helpers now imported from util

  // Navigate to document detail page
  const handleViewDocument = useCallback(
    (document: Document) => {
      if (canEdit) router.push(`/console/dataset/${datasetId}/documents/${document.id}`);
    },
    [datasetId, canEdit, router]
  );

  // Get file type icon
  const getFileTypeIcon = (fileType?: string) => {
    if (!fileType) {
      return FileText;
    }

    switch (fileType.toLowerCase()) {
      case 'pdf':
        return FileText;
      case 'doc':
      case 'docx':
        return FileText;
      case 'txt':
        return FileText;
      default:
        return FileText;
    }
  };

  const mapDocForm = (form?: string) => {
    switch ((form || '').toLowerCase()) {
      case 'text_model':
        return t('docForm.textModel');
      case 'qa_model':
        return t('docForm.qaModel');
      case 'hierarchical_model':
        return t('docForm.hierarchicalModel');
      default:
        return form || '-';
    }
  };

  const renderSortArrow = useCallback(
    (field: Array<'created_at' | 'updated_at' | '-created_at' | '-updated_at'>) => {
      return (
        <ArrowDown
          size={16}
          className={cn(
            'inline transition-transform duration-200',
            field.includes(sortField) && !sortField.startsWith('-') ? 'rotate-180' : '',
            field.includes(sortField) ? 'text-primary' : 'text-muted-foreground'
          )}
        />
      );
    },
    [sortField]
  );
  return (
    <div className={cn('rounded-md border overflow-hidden ', className)}>
      <Table>
        <TableHeader>
          <TableRow>
            {/* Selection */}
            {canEdit && (
              <TableHead className="w-[40px]">
                <Checkbox
                  checked={documents.length > 0 && documents.every(d => selectedIds.includes(d.id))}
                  onCheckedChange={checked =>
                    onToggleSelectAll?.(
                      Boolean(checked),
                      documents.map(d => d.id)
                    )
                  }
                />
              </TableHead>
            )}
            {/* File Icon */}
            <TableHead className="w-8" />
            {/* Name */}
            <TableHead className="min-w-[220px]">{t('documentName')}</TableHead>
            {/* Word Count */}
            <TableHead className="w-[120px]">{t('wordCount')}</TableHead>
            {/* Hit Count */}
            <TableHead className="w-[120px]">{t('hitCount')}</TableHead>
            {/* Created At with sort */}
            <TableHead className="w-[180px] select-none">
              <span className="inline-flex items-center">
                {t('uploadTime')}
                <Button
                  variant="ghost"
                  isIcon
                  className="w-6 h-6 ml-1"
                  onClick={() => {
                    sortField === '-created_at'
                      ? onSortChange?.('created_at')
                      : onSortChange?.('-created_at');
                  }}
                >
                  {renderSortArrow(['created_at', '-created_at'])}
                </Button>
              </span>
            </TableHead>
            {/* Status with filter dropdown */}
            <TableHead className="w-[200px]">
              <span className="inline-flex items-center gap-2">
                {t('documentStatus')}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      isIcon
                      className={cn(
                        'h-6 w-6 ml-1',
                        statusFilter !== 'all'
                          ? 'text-primary bg-primary/10 hover:bg-primary/20'
                          : ''
                      )}
                    >
                      <ListFilter
                        size={16}
                        className={cn(
                          statusFilter !== 'all' ? 'text-primary' : 'text-muted-foreground'
                        )}
                      />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    <DropdownMenuRadioGroup
                      value={String(statusFilter || 'all')}
                      onValueChange={value =>
                        onStatusFilterChange?.(
                          value as unknown as
                            | DocumentIndexingStatus[keyof DocumentIndexingStatus]
                            | 'all'
                        )
                      }
                    >
                      <DropdownMenuRadioItem value="all">
                        <Badge variant="outline">{tCommon('all')}</Badge>
                      </DropdownMenuRadioItem>
                      <DropdownMenuSeparator />
                      {(['indexing', 'completed', 'failed'] as const).map(s => (
                        <DropdownMenuRadioItem key={s} value={s}>
                          <DocumentStatusBadge
                            status={
                              s as unknown as
                                | DocumentStatus
                                | DocumentDisplayStatus
                                | DocumentIndexingStatus
                            }
                          />
                        </DropdownMenuRadioItem>
                      ))}
                    </DropdownMenuRadioGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </span>
            </TableHead>
            {/* Enabled Switch */}
            <TableHead className="w-[100px]">{t('segments.enabled')}</TableHead>
            {/* Actions */}
            <TableHead className="w-[80px]" />
          </TableRow>
        </TableHeader>
        {documents.length > 0 && (
          <TableBody>
            {documents.map(document => {
              const FileIcon = getFileTypeIcon(document.metadata?.file_type);
              return (
                <TableRow
                  key={document.id}
                  className="hover:bg-muted/50 cursor-pointer"
                  onClick={() => handleViewDocument(document)}
                >
                  {/* Selection */}
                  {canEdit && (
                    <TableCell onClick={e => e.stopPropagation()}>
                      <Checkbox
                        checked={selectedIds.includes(document.id)}
                        onCheckedChange={() => onToggleSelect?.(document.id)}
                      />
                    </TableCell>
                  )}
                  <TableCell>
                    <FileIcon className="h-4 w-4 text-muted-foreground" />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <span
                        className="font-medium truncate hover:text-primary transition-colors"
                        title={document.name}
                      >
                        {document.name}
                      </span>
                      <ExtractionFallbackNotice extraction={document.doc_metadata?.extraction} />
                      <ExternalLink className="h-3 w-3 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                    </div>
                  </TableCell>

                  {/* Word Count */}
                  <TableCell className="text-sm text-muted-foreground">
                    {formatNumber(document?.word_count ?? 0)}
                  </TableCell>

                  {/* Hit Count */}
                  <TableCell className="text-sm text-muted-foreground">
                    {document.hit_count?.toLocaleString?.() ?? '-'}
                  </TableCell>

                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(document.created_at)}
                  </TableCell>

                  {/* Status */}
                  <TableCell>
                    <div className="flex items-center gap-3 min-w-[180px]">
                      <DocumentStatusBadge
                        status={
                          document.indexing_status ||
                          document.display_status ||
                          document.status ||
                          'pending'
                        }
                      />
                      {document.indexing_status &&
                        !['completed', 'error', 'paused'].includes(document.indexing_status) &&
                        typeof document.progress === 'number' &&
                        document.progress > 0 &&
                        document.progress < 100 && (
                          <div className="flex items-center gap-2 flex-1">
                            <Progress value={document.progress} className="h-1.5 w-16" />
                            <span className="text-xs text-muted-foreground whitespace-nowrap">
                              {document.progress}%
                            </span>
                          </div>
                        )}
                    </div>
                  </TableCell>

                  {/* Enabled switch - only interactive when user has edit permission */}
                  <TableCell onClick={e => e.stopPropagation()}>
                    {(() => {
                      const isEnabled = document.enabled ?? document.display_status === 'enabled';
                      return (
                        <Switch
                          checked={Boolean(isEnabled)}
                          disabled={!canEdit || isMutatingEnabled.includes(document.id)}
                          onCheckedChange={checked =>
                            canEdit && onToggleEnabled?.(document.id, Boolean(checked))
                          }
                        />
                      );
                    })()}
                  </TableCell>

                  <TableCell>
                    <DropdownMenu
                      open={activeDropdown === document.id}
                      onOpenChange={open => setActiveDropdown(open ? document.id : null)}
                    >
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          isIcon
                          className="h-8 w-8"
                          onClick={e => {
                            e.stopPropagation(); // Prevent row click
                          }}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        {canEdit && (
                          <DropdownMenuItem
                            onClick={e => {
                              e.stopPropagation();
                              handleViewDocument(document);
                            }}
                          >
                            <Eye className="h-4 w-4 mr-2" />
                            {t('documentActions.view')}
                          </DropdownMenuItem>
                        )}

                        {onDownload && (
                          <DropdownMenuItem
                            onClick={e => {
                              e.stopPropagation();
                              onDownload(document);
                            }}
                          >
                            <Download className="h-4 w-4 mr-2" />
                            {t('actions.download')}
                          </DropdownMenuItem>
                        )}

                        {onReprocess &&
                          (document.display_status === 'error' || document.status === 'error') && (
                            <DropdownMenuItem
                              onClick={e => {
                                e.stopPropagation();
                                onReprocess(document);
                              }}
                            >
                              <RotateCcw className="h-4 w-4 mr-2" />
                              {t('actions.reprocess')}
                            </DropdownMenuItem>
                          )}

                        {/* Only show delete when user has edit permission */}
                        {(onDownload || onReprocess) && onDelete && canEdit && (
                          <DropdownMenuSeparator />
                        )}

                        {onDelete && canEdit && (
                          <DropdownMenuItem
                            onClick={e => {
                              e.stopPropagation();
                              onDelete(document);
                            }}
                            className="text-destructive focus:text-destructive"
                          >
                            <Trash2 className="h-4 w-4 mr-2" />
                            {t('actions.delete')}
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        )}
        {documents.length === 0 && (
          <TableBody>
            <TableRow>
              <TableCell colSpan={11} className="text-center py-4">
                <div className="flex items-center flex-col gap-2">
                  <div className="mx-auto w-24 h-24 bg-muted rounded-full flex items-center justify-center mb-4">
                    <Search className="h-8 w-8 text-muted-foreground" />
                  </div>
                  <div className="text-lg">{t('noDocuments')}</div>
                  {statusFilter !== 'all' && (
                    <Button
                      variant="default"
                      size="sm"
                      onClick={() => onStatusFilterChange?.('all')}
                    >
                      {tCommon('resetFilters')}
                    </Button>
                  )}
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        )}
      </Table>
    </div>
  );
}
