'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import DefaultValueEditor from './default-value-editor';
import type { ConversationVariable } from '../../store/type';

function parseValueByType(t: ConversationVariable['type'], raw: string): unknown {
  try {
    switch (t) {
      case 'string':
        return raw;
      case 'number':
        return raw === '' ? '' : Number.isNaN(Number(raw)) ? '' : Number(raw);
      case 'boolean':
        return raw === 'true' || raw === '1';
      case 'object':
        return raw.trim() ? JSON.parse(raw) : {};
      case 'array[string]':
      case 'array[number]':
      case 'array[boolean]':
      case 'array[object]':
        return raw.trim() ? JSON.parse(raw) : [];
      default:
        return raw;
    }
  } catch {
    if (t === 'object') return {};
    if (t.startsWith('array')) return [];
    return raw;
  }
}

function getIdentifierError(name: string) {
  const raw = (name || '').trim();
  if (raw.length === 0) {
    return 'workflow.conversationVariables.validation.required';
  }
  if (!/^[A-Za-z]/.test(raw)) {
    return 'workflow.conversationVariables.validation.mustStartWithLetter';
  }
  if (!/^[A-Za-z][A-Za-z0-9_]*$/.test(raw)) {
    return 'workflow.conversationVariables.validation.allowedChars';
  }
  return '';
}

export interface ConversationVariableEditorDialogProps {
  open: boolean;
  editing: boolean;
  value: ConversationVariable;
  onChange: (next: ConversationVariable) => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: (finalValue: ConversationVariable) => void;
  existingNames: string[];
  typeOptions: Array<ConversationVariable['type']>;
}

const ConversationVariableEditorDialog: React.FC<ConversationVariableEditorDialogProps> = ({
  open,
  editing,
  value,
  onChange,
  onOpenChange,
  onSubmit,
  existingNames,
  typeOptions,
}) => {
  const t = useT('agents');
  const tCommon = useT('common');
  const [nameError, setNameError] = React.useState<string>('');

  React.useEffect(() => {
    if (!open) {
      setNameError('');
    }
  }, [open]);

  const handleSave = () => {
    const raw = (value.name || '').trim();
    const reason = getIdentifierError(raw);
    if (reason) {
      setNameError(t(reason));
      return;
    }
    const dup = existingNames.includes(raw);
    if (dup) {
      setNameError(t('workflow.conversationVariables.validation.duplicateName'));
      return;
    }
    setNameError('');
    onSubmit({ ...value, name: raw });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[560px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {editing
              ? t('workflow.conversationVariables.dialog.editTitle')
              : t('workflow.conversationVariables.dialog.addTitle')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6 font-medium">
          <div className="space-y-5">
            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">
                {t('workflow.conversationVariables.fields.name')}
              </Label>
              <Input
                value={value.name}
                className={cn(
                  'h-10 shadow-sm font-medium',
                  nameError
                    ? 'border-destructive focus-visible:ring-destructive bg-destructive/5'
                    : undefined
                )}
                onChange={e => {
                  const next = e.target.value;
                  onChange({ ...value, name: next });
                  const reason = getIdentifierError(next);
                  if (reason) {
                    setNameError(t(reason));
                    return;
                  }
                  const isDup = existingNames.includes(next.trim());
                  if (isDup) {
                    setNameError(t('workflow.conversationVariables.validation.duplicateName'));
                  } else {
                    setNameError('');
                  }
                }}
                placeholder={t('workflow.conversationVariables.fields.name')}
              />
              {nameError ? (
                <div className="text-xs text-destructive font-bold px-1">{nameError}</div>
              ) : null}
            </div>

            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">
                {t('workflow.conversationVariables.fields.type')}
              </Label>
              <Select
                value={value.type}
                onValueChange={val => {
                  const nextType = val as ConversationVariable['type'];
                  const nextVal =
                    nextType === 'string'
                      ? ''
                      : nextType === 'boolean'
                        ? false
                        : nextType === 'number'
                          ? ''
                          : nextType.startsWith('array')
                            ? '[]'
                            : '{}';
                  onChange({
                    ...value,
                    type: nextType,
                    value: parseValueByType(nextType, String(nextVal)),
                  });
                }}
              >
                <SelectTrigger className="h-10 shadow-sm font-medium">
                  <SelectValue placeholder={t('workflow.conversationVariables.fields.type')} />
                </SelectTrigger>
                <SelectContent>
                  {typeOptions.map(t => (
                    <SelectItem key={t} value={t} className="font-medium">
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">
                {t('workflow.conversationVariables.fields.description')}
              </Label>
              <Textarea
                value={value.description ?? ''}
                onChange={e => onChange({ ...value, description: e.target.value })}
                placeholder={t('workflow.conversationVariables.fields.description')}
                className="max-h-32 shadow-sm font-medium resize-none"
              />
            </div>

            <div className="flex flex-col gap-2 pt-2 border-t border-neutral-100">
              <Label className="text-sm font-bold px-0.5">
                {t('workflow.conversationVariables.placeholders.defaultValue')}
              </Label>
              <div className="bg-neutral-50/50 rounded-xl border border-neutral-100 p-1">
                <DefaultValueEditor
                  type={value.type}
                  value={value.value}
                  onChange={v => onChange({ ...value, value: v })}
                />
              </div>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="font-semibold">
            {tCommon('cancel')}
          </Button>
          <Button onClick={handleSave} size="lg" className="px-10 font-bold shadow-sm">
            {tCommon('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default ConversationVariableEditorDialog;
