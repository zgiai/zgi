'use client';

import { useState, type ReactNode } from 'react';
import { InfoHint } from '@/components/memory/info-hint';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';

/**
 * One memory switch of an Agent or DigitalHuman: a title, one line saying
 * what it does, and the longer explanation behind a hint.
 */
export function MemorySettingRow({
  id,
  title,
  description,
  hint,
  badge,
  checked,
  disabled,
  onCheckedChange,
}: {
  id: string;
  title: string;
  description: ReactNode;
  hint?: string;
  badge?: ReactNode;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-1.5">
          <span id={id} className="text-sm font-medium">
            {title}
          </span>
          {hint ? <InfoHint text={hint} /> : null}
          {badge}
        </div>
        <div className="mt-0.5 text-xs leading-5 text-muted-foreground">{description}</div>
      </div>
      <Switch
        aria-labelledby={id}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

export interface MemoryInstructionsTemplate {
  id: string;
  name: string;
  description: string;
  instructions: string;
}

/**
 * The memory instructions of an Agent or DigitalHuman, with the templates
 * offered in place. Picking a template replaces what is written, after
 * confirming when that would lose text.
 */
export function MemoryInstructionsField({
  value,
  onChange,
  maxLength,
  templates,
  placeholder,
  disabled,
  onTemplateApplied,
}: {
  value: string;
  onChange: (value: string) => void;
  maxLength: number;
  templates: MemoryInstructionsTemplate[];
  placeholder: string;
  disabled?: boolean;
  onTemplateApplied?: (template: MemoryInstructionsTemplate) => void;
}) {
  const t = useT('common.memorySettings');
  const [templateId, setTemplateId] = useState('');
  const [pending, setPending] = useState<MemoryInstructionsTemplate | null>(null);
  const apply = (template: MemoryInstructionsTemplate) => {
    setTemplateId(template.id);
    onChange(template.instructions.slice(0, maxLength));
    setPending(null);
    onTemplateApplied?.(template);
  };
  const choose = (id: string) => {
    const template = templates.find(item => item.id === id);
    if (!template) return;
    const current = value.trim();
    if (current && current !== template.instructions.trim()) {
      setPending(template);
      return;
    }
    apply(template);
  };

  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1.5">
          <span className="text-sm font-medium">{t('instructionsLabel')}</span>
          <span className="text-xs text-muted-foreground">{t('instructionsOptional')}</span>
          <InfoHint text={t('instructionsHint', { max: maxLength })} />
        </div>
        {templates.length > 0 ? (
          <Select value={templateId} disabled={disabled} onValueChange={choose}>
            <SelectTrigger
              className="h-7 w-auto min-w-36 gap-1.5 text-xs"
              aria-label={t('templateLabel')}
            >
              <SelectValue placeholder={t('templatePlaceholder')} />
            </SelectTrigger>
            <SelectContent align="end" className="max-w-80">
              {templates.map(template => (
                <SelectItem key={template.id} value={template.id} textValue={template.name}>
                  <div className="min-w-0">
                    <div className="text-sm">{template.name}</div>
                    <div className="whitespace-normal text-xs text-muted-foreground">
                      {template.description}
                    </div>
                  </div>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
      </div>
      <Textarea
        value={value}
        maxLength={maxLength}
        showCharacterCount
        className="min-h-28"
        aria-label={t('instructionsLabel')}
        placeholder={placeholder}
        disabled={disabled}
        onChange={event => onChange(event.target.value.slice(0, maxLength))}
      />
      {!value.trim() ? (
        <div className="text-xs text-muted-foreground">{t('instructionsEmpty')}</div>
      ) : null}
      <ConfirmDialog
        open={Boolean(pending)}
        onOpenChange={open => !open && setPending(null)}
        title={t('replaceTitle')}
        description={t('replaceDescription')}
        confirmText={t('replace')}
        cancelText={t('cancel')}
        variant="warning"
        onConfirm={() => pending && apply(pending)}
      />
    </div>
  );
}
