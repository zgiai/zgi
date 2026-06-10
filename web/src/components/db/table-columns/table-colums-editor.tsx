'use client';

// Editable Db table columns rows for reuse in multiple pages.
// Renders only the <TableBody/> with editable cells, leaving headers and container to parent.
// Strict types, no any; i18n via next-intl 'dbs' namespace.

import { useEffect, useMemo } from 'react';
import { useT } from '@/i18n';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { TableBody, TableCell, TableRow } from '@/components/ui/table';
import { TrashIcon, AlertCircle } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { DbTableColumn } from '@/services/types/db';
import { Type } from '@/services/types/db';
import {
  getDuplicateDbColumnNames,
  isInvalidDbColumnName,
  isReservedDbColumnName,
} from '@/utils/validation';

interface TableColumnsEditorProps {
  columns: readonly DbTableColumn[];
  onChange: (next: DbTableColumn[]) => void;
  disableSystemFields?: boolean;
  showActions?: boolean;
  typeOptions?: ReadonlyArray<{ label: string; value: Type }>;
  onValidationChange?: (state: { hasDuplicateNames: boolean; hasInvalidNames: boolean }) => void;
}

export default function TableColumnsEditor({
  columns,
  onChange,
  disableSystemFields = true,
  showActions = true,
  typeOptions = [
    { label: 'boolean', value: Type.Boolean },
    { label: 'integer', value: Type.Integer },
    { label: 'numeric', value: Type.Numeric },
    { label: 'text', value: Type.Text },
    { label: 'timestamp', value: Type.Timestamp },
  ],
  onValidationChange,
}: TableColumnsEditorProps) {
  const t = useT('dbs');

  const validationColumns = useMemo(
    () => columns.filter(c => !(disableSystemFields && c.is_system_field)),
    [columns, disableSystemFields]
  );

  // Duplicate names detection (case-insensitive, ignore empty while editing)
  const duplicateNameSet = useMemo(
    () => getDuplicateDbColumnNames(validationColumns),
    [validationColumns]
  );

  const hasDuplicateNames = useMemo(
    () => Array.from(duplicateNameSet.values()).length > 0,
    [duplicateNameSet]
  );

  const hasInvalidNames = useMemo(
    () =>
      validationColumns.some(
        c => isInvalidDbColumnName(c.name || '') || isReservedDbColumnName(c.name || '')
      ),
    [validationColumns]
  );

  // Report validation state to parent if requested
  useEffect(() => {
    if (onValidationChange) {
      // Only report when booleans change to avoid update loops
      onValidationChange({
        hasDuplicateNames,
        hasInvalidNames,
      });
    }
    // Depend only on booleans so the effect does not run every render.
  }, [hasDuplicateNames, hasInvalidNames]);

  const updateColumn = (id: string, patch: Partial<DbTableColumn>): void => {
    const next = columns.map(c => (c.id === id ? { ...c, ...patch } : c));
    onChange(next);
  };

  const removeColumn = (id: string): void => {
    const next = columns.filter(c => c.id !== id);
    onChange(next);
  };

  return (
    <TableBody>
      {columns.map((col, index) => {
        const isSystem = disableSystemFields && Boolean(col.is_system_field);
        const lowerName = (col.name || '').trim().toLowerCase();
        const isDup = !isSystem && lowerName.length > 0 && duplicateNameSet.has(lowerName);
        const invalidFormat = !isSystem && isInvalidDbColumnName(col.name || '');
        const isReserved = !isSystem && isReservedDbColumnName(col.name || '');
        const isInvalid = invalidFormat || isReserved;
        return (
          <TableRow key={col.id || `row-${index}`}>
            {/* name */}
            <TableCell>
              {isSystem ? (
                <span className="text-sm text-muted-foreground" title={col.name}>
                  {col.name}
                </span>
              ) : (
                <div className="relative">
                  <Input
                    value={col.name}
                    onChange={e => {
                      // Allow free typing to avoid confusing auto-modification; validate separately
                      const raw = e.target.value;
                      updateColumn(col.id, { name: raw });
                    }}
                    className={
                      isDup || isInvalid
                        ? 'border-destructive focus-visible:ring-destructive pr-8'
                        : undefined
                    }
                    aria-invalid={isDup || isInvalid}
                  />
                  {/* Inline error icon with tooltip to prevent overflow and layout shift */}
                  {isInvalid || isDup ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <AlertCircle className="absolute right-2 top-1/2 -translate-y-1/2 h-4 w-4 text-destructive" />
                      </TooltipTrigger>
                      <TooltipContent side="top" className="max-w-[320px]">
                        <div className="text-xs">
                          {isInvalid
                            ? isReserved
                              ? t('columns.reservedNameTip')
                              : t('columns.invalidNameTip')
                            : t('columns.duplicateNameTip')}
                        </div>
                      </TooltipContent>
                    </Tooltip>
                  ) : null}
                </div>
              )}
            </TableCell>

            {/* description */}
            <TableCell>
              {isSystem ? (
                <span className="text-sm text-muted-foreground" title={col.description}>
                  {col.description}
                </span>
              ) : (
                <Input
                  value={col.description}
                  onChange={e => updateColumn(col.id, { description: e.target.value })}
                />
              )}
            </TableCell>

            {/* type */}
            <TableCell>
              {isSystem ? (
                <span className="uppercase text-xs text-muted-foreground">{col.type}</span>
              ) : (
                <Select
                  value={col.type}
                  onValueChange={v => updateColumn(col.id, { type: v as Type })}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder={t('columns.selectTypePlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {typeOptions.map(opt => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </TableCell>

            {/* required */}
            <TableCell>
              <div className="flex items-center">
                <Switch
                  checked={col.is_required}
                  onCheckedChange={checked => updateColumn(col.id, { is_required: !!checked })}
                  disabled={isSystem}
                />
              </div>
            </TableCell>

            {/* actions */}
            {showActions && (
              <TableCell>
                {isSystem ? (
                  <span className="text-xs text-muted-foreground">{t('columns.system')}</span>
                ) : (
                  <Button
                    variant="ghost"
                    className="hover:bg-destructive/30 hover:text-destructive"
                    size="sm"
                    onClick={() => removeColumn(col.id)}
                  >
                    <TrashIcon className="h-4 w-4" />
                  </Button>
                )}
              </TableCell>
            )}
          </TableRow>
        );
      })}
    </TableBody>
  );
}
