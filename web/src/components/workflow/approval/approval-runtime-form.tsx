'use client';

import React from 'react';
import { Loader2 } from 'lucide-react';

import MarkdownViewer from '@/components/common/markdown-viewer';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import type {
  ApprovalRuntimeAction,
  ApprovalRuntimeForm as ApprovalRuntimeFormData,
} from '@/services/approval.service';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

export interface ApprovalRuntimeFormProps {
  form: ApprovalRuntimeFormData;
  isSubmitting?: boolean;
  submittedAction?: string | null;
  onSubmit: (payload: { inputs: Record<string, unknown>; action: string }) => void | Promise<void>;
  className?: string;
}

function stringifyDefault(value: unknown): string {
  if (value === null || typeof value === 'undefined') return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function getActionClassName(action: ApprovalRuntimeAction): string {
  switch (action.style) {
    case 'danger':
      return 'bg-destructive text-destructive-foreground hover:bg-destructive/90';
    case 'primary':
      return '';
    default:
      return '';
  }
}

/**
 * @component ApprovalRuntimeForm
 * @category Feature
 * @status Beta
 * @description Runtime form for submitting a paused human approval workflow node.
 * @usage Reuse in workflow run panel and public token approval page.
 * @example
 * <ApprovalRuntimeForm form={form} onSubmit={submit} />
 */
export function ApprovalRuntimeForm({
  form,
  isSubmitting = false,
  submittedAction,
  onSubmit,
  className,
}: ApprovalRuntimeFormProps) {
  const t = useT('nodes');
  const [inputs, setInputs] = React.useState<Record<string, string>>({});
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  React.useEffect(() => {
    const defaults = form.resolved_default_values || {};
    const next: Record<string, string> = {};
    for (const field of form.fields || []) {
      next[field.key] = stringifyDefault(defaults[field.key]);
    }
    setInputs(next);
    setErrors({});
  }, [form]);

  const submit = React.useCallback(
    async (action: ApprovalRuntimeAction) => {
      const nextErrors: Record<string, string> = {};
      for (const field of form.fields || []) {
        if (field.required && !String(inputs[field.key] ?? '').trim()) {
          nextErrors[field.key] = t('approval.runtime.required');
        }
      }
      setErrors(nextErrors);
      if (Object.keys(nextErrors).length > 0) return;
      await onSubmit({ inputs, action: action.id });
    },
    [form.fields, inputs, onSubmit, t]
  );

  return (
    <div className={cn('space-y-5', className)}>
      <div className="space-y-1">
        <h2 className="text-lg font-semibold">{form.node_title}</h2>
        {form.expiration_at ? (
          <p className="text-xs text-muted-foreground">
            {t('approval.runtime.expiresAt', {
              time: new Date(form.expiration_at * 1000).toLocaleString(),
            })}
          </p>
        ) : null}
      </div>

      <div className="rounded-lg border bg-background p-3">
        <MarkdownViewer content={form.content || ''} className="md-viewer break-words whitespace-pre-wrap" />
      </div>

      <div className="space-y-4">
        {(form.fields || []).map(field => (
          <div key={field.key} className="space-y-1.5">
            <Label htmlFor={`approval-${form.id}-${field.key}`} className="text-sm font-medium">
              {field.label || field.key}
              {field.required ? <span className="ml-1 text-destructive">*</span> : null}
            </Label>
            {field.type === 'textarea' ? (
              <Textarea
                id={`approval-${form.id}-${field.key}`}
                value={inputs[field.key] ?? ''}
                onChange={event =>
                  setInputs(current => ({ ...current, [field.key]: event.target.value }))
                }
                aria-invalid={Boolean(errors[field.key])}
                className="min-h-[120px]"
              />
            ) : (
              <Input
                id={`approval-${form.id}-${field.key}`}
                value={inputs[field.key] ?? ''}
                onChange={event =>
                  setInputs(current => ({ ...current, [field.key]: event.target.value }))
                }
                aria-invalid={Boolean(errors[field.key])}
              />
            )}
            {errors[field.key] ? (
              <p className="text-xs text-destructive">{errors[field.key]}</p>
            ) : null}
          </div>
        ))}
      </div>

      <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
        {(form.actions || []).map(action => (
          <Button
            key={action.id}
            type="button"
            variant={action.style === 'secondary' ? 'outline' : 'default'}
            className={getActionClassName(action)}
            disabled={isSubmitting}
            onClick={() => submit(action)}
          >
            {isSubmitting && submittedAction === action.id ? (
              <Loader2 className="size-4 animate-spin" />
            ) : null}
            {action.label || action.id}
          </Button>
        ))}
      </div>
    </div>
  );
}

export default ApprovalRuntimeForm;
