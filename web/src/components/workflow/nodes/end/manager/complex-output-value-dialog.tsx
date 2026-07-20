'use client';

import React from 'react';
import { AlertCircle, Eye, PencilLine } from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import DefaultValueEditor from '@/components/workflow/ui/conversation-variables-panel/default-value-editor';
import { useT } from '@/i18n';

import {
  copyComplexOutputValue,
  countComplexOutputValue,
  defaultComplexOutputMode,
  serializeComplexOutputValue,
  validateComplexOutputValue,
  type ComplexOutputType,
  type ComplexOutputValidationError,
} from './complex-output-value';

interface ComplexOutputValueDialogProps {
  variableName: string;
  type: ComplexOutputType;
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

export function ComplexOutputValueDialog({
  variableName,
  type,
  value,
  onChange,
  readOnly = false,
}: ComplexOutputValueDialogProps) {
  const t = useT('nodes');
  const tCommon = useT('common');
  const [open, setOpen] = React.useState(false);
  const [draftValue, setDraftValue] = React.useState<unknown>(() => copyComplexOutputValue(value));
  const [jsonInput, setJsonInput] = React.useState(() => serializeComplexOutputValue(value));
  const [jsonError, setJsonError] = React.useState<ComplexOutputValidationError | undefined>();
  const [arrayMode, setArrayMode] = React.useState<'list' | 'json'>(() =>
    defaultComplexOutputMode(type)
  );
  const count = countComplexOutputValue(type, value);
  const summary =
    type === 'object'
      ? t('end.outputs.objectSummary', { count })
      : t('end.outputs.arraySummary', { count });

  const handleOpenChange = React.useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) {
        setDraftValue(copyComplexOutputValue(value));
        setJsonInput(serializeComplexOutputValue(value));
        setJsonError(undefined);
        setArrayMode(defaultComplexOutputMode(type));
      }
      setOpen(nextOpen);
    },
    [type, value]
  );

  const handleTypedChange = React.useCallback((nextValue: unknown) => {
    setDraftValue(nextValue);
    setJsonInput(serializeComplexOutputValue(nextValue));
    setJsonError(undefined);
  }, []);

  const handleJsonChange = React.useCallback(
    (raw: string) => {
      setJsonInput(raw);
      const result = validateComplexOutputValue(type, raw);
      setJsonError(result.error);
      if (!result.error) setDraftValue(result.value);
    },
    [type]
  );

  const handleArrayModeChange = React.useCallback(
    (nextMode: 'list' | 'json') => {
      if (nextMode === 'list' && jsonError) return;
      setArrayMode(nextMode);
    },
    [jsonError]
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <div className="flex h-8 min-w-0 items-center justify-between gap-2 rounded-md border bg-muted/20 pl-2.5 pr-1">
        <span className="truncate text-xs text-muted-foreground">{summary}</span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 shrink-0 gap-1.5 px-2 text-xs"
          onClick={() => handleOpenChange(true)}
        >
          {readOnly ? <Eye className="size-3.5" /> : <PencilLine className="size-3.5" />}
          {readOnly ? t('end.outputs.viewConstant') : t('end.outputs.editConstant')}
        </Button>
      </div>

      <DialogContent size="lg" className="gap-0 overflow-hidden p-0">
        <DialogHeader className="border-b">
          <DialogTitle>
            {t('end.outputs.editConstantTitle', {
              name: variableName.trim() || t('end.unnamed'),
            })}
          </DialogTitle>
        </DialogHeader>
        <DialogBody className="py-5">
          <DefaultValueEditor
            type={type}
            value={draftValue}
            onChange={handleTypedChange}
            arrayMode={arrayMode}
            onArrayModeChange={handleArrayModeChange}
            jsonValue={jsonInput}
            onJsonValueChange={handleJsonChange}
            readOnly={readOnly}
          />
          {jsonError ? (
            <div className="mt-2 flex items-start gap-1.5 text-xs text-destructive">
              <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
              <span>{t(`end.outputs.${jsonError}`)}</span>
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter className="border-t bg-muted/10">
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            {tCommon('cancel')}
          </Button>
          {!readOnly ? (
            <Button
              type="button"
              onClick={() => {
                const result = validateComplexOutputValue(type, jsonInput);
                if (result.error) {
                  setJsonError(result.error);
                  return;
                }
                onChange(copyComplexOutputValue(result.value));
                setOpen(false);
              }}
              disabled={!jsonInput.trim() || !!jsonError}
            >
              {tCommon('confirm')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default ComplexOutputValueDialog;
