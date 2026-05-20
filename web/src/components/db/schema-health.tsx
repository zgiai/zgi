'use client';

import type { FC } from 'react';
import React from 'react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Sparkles, Wand2 } from 'lucide-react';
import type { DbTableColumn } from '@/services/types/db';
import { Type } from '@/services/types/db';
import { useT } from '@/i18n';

export interface SemanticColumnSuggestion {
  column: DbTableColumn;
  suggestedName: string;
  suggestedType: Type;
  sourceLabel: string;
  confidence: 'high' | 'medium';
}

const SOURCE_LABEL_MAP: ReadonlyArray<{
  patterns: readonly string[];
  name: string;
  type: Type;
}> = [
  { patterns: ['住院号', '住院号码', 'inpatient'], name: 'inpatient_no', type: Type.Text },
  { patterns: ['入院时间', '入院日期', 'admission'], name: 'admission_time', type: Type.Timestamp },
  {
    patterns: ['上级医师查房记录', '上级医生查房记录', '查房记录', 'round'],
    name: 'superior_doctor_round_record',
    type: Type.Text,
  },
  { patterns: ['操作时间', '记录时间', 'operation'], name: 'operation_time', type: Type.Timestamp },
  { patterns: ['标识', '医院', '机构', '院区', 'hospital'], name: 'hospital_name', type: Type.Text },
] as const;

function getSourceLabel(column: DbTableColumn): string {
  const raw =
    column.source_column_name ||
    column.display_name ||
    column.description?.replace(/^Imported from\s+/i, '') ||
    '';
  return raw.trim();
}

function toSnakeCase(input: string, fallback: string): string {
  const normalized = input
    .trim()
    .replace(/^Imported from\s+/i, '')
    .replace(/[^\p{L}\p{N}]+/gu, '_')
    .replace(/^_+|_+$/g, '')
    .toLowerCase();
  if (/^[a-z][a-z0-9_]*$/.test(normalized)) return normalized;
  return fallback;
}

function inferSuggestion(column: DbTableColumn): SemanticColumnSuggestion | null {
  if (column.is_system_field || !/^column_\d+$/i.test(column.name)) return null;

  const sourceLabel = getSourceLabel(column);
  const haystack = `${sourceLabel} ${column.description || ''} ${column.name}`.toLowerCase();
  const matched = SOURCE_LABEL_MAP.find(item =>
    item.patterns.some(pattern => haystack.includes(pattern.toLowerCase()))
  );

  if (matched) {
    return {
      column,
      suggestedName: matched.name,
      suggestedType: matched.type,
      sourceLabel,
      confidence: 'high',
    };
  }

  return {
    column,
    suggestedName: toSnakeCase(sourceLabel, column.name),
    suggestedType: column.type,
    sourceLabel,
    confidence: 'medium',
  };
}

export function getSemanticColumnSuggestions(
  columns: readonly DbTableColumn[]
): SemanticColumnSuggestion[] {
  return columns.map(inferSuggestion).filter((item): item is SemanticColumnSuggestion => !!item);
}

export function isGenericImportedSchema(columns: readonly DbTableColumn[]): boolean {
  return getSemanticColumnSuggestions(columns).length > 0;
}

export function isBusinessTimeColumn(column: DbTableColumn, names: readonly string[]): boolean {
  const source = `${column.name} ${getSourceLabel(column)} ${column.description || ''}`.toLowerCase();
  return names.some(name => source.includes(name.toLowerCase()));
}

interface SchemaHealthNoticeProps {
  columns: readonly DbTableColumn[];
  manageStructureHref?: string;
  compact?: boolean;
}

export const SchemaHealthNotice: FC<SchemaHealthNoticeProps> = ({
  columns,
  manageStructureHref,
  compact = false,
}) => {
  const t = useT('dbs');
  const [open, setOpen] = React.useState(false);
  const suggestions = React.useMemo(() => getSemanticColumnSuggestions(columns), [columns]);

  if (suggestions.length === 0) return null;

  return (
    <>
      <Alert className={compact ? 'py-2' : undefined}>
        <Sparkles className="h-4 w-4" />
        <AlertTitle className="text-sm">{t('schemaHealth.title')}</AlertTitle>
        <AlertDescription className="flex flex-wrap items-center justify-between gap-3 text-sm">
          <span>
            {t('schemaHealth.description', {
              count: suggestions.length,
            })}
          </span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
              <Wand2 className="h-4 w-4" />
              {t('schemaHealth.previewAction')}
            </Button>
            {manageStructureHref && (
              <Button asChild size="sm">
                <a href={manageStructureHref}>{t('actions.manageStructure')}</a>
              </Button>
            )}
          </div>
        </AlertDescription>
      </Alert>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent size="xl">
          <DialogHeader>
            <DialogTitle>{t('schemaHealth.previewTitle')}</DialogTitle>
          </DialogHeader>
          <DialogBody className="px-6 pb-4">
            <div className="mb-3 rounded-md bg-muted/40 p-3 text-sm text-muted-foreground">
              {t('schemaHealth.previewDesc')}
            </div>
            <div className="max-h-[420px] overflow-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('schemaHealth.currentField')}</TableHead>
                    <TableHead>{t('schemaHealth.sourceField')}</TableHead>
                    <TableHead>{t('schemaHealth.suggestedField')}</TableHead>
                    <TableHead>{t('columns.type')}</TableHead>
                    <TableHead>{t('schemaHealth.confidence')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {suggestions.map(item => (
                    <TableRow key={item.column.id}>
                      <TableCell className="font-mono text-xs">{item.column.name}</TableCell>
                      <TableCell>{item.sourceLabel || '-'}</TableCell>
                      <TableCell className="font-mono text-xs">{item.suggestedName}</TableCell>
                      <TableCell className="uppercase text-xs text-muted-foreground">
                        {item.suggestedType}
                      </TableCell>
                      <TableCell>
                        <Badge variant={item.confidence === 'high' ? 'secondary' : 'outline'}>
                          {item.confidence === 'high'
                            ? t('schemaHealth.high')
                            : t('schemaHealth.medium')}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t('schemaHealth.later')}
            </Button>
            {manageStructureHref && (
              <Button asChild>
                <a href={manageStructureHref}>{t('schemaHealth.applyInStructure')}</a>
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};
