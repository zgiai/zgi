'use client';

import React, { useCallback } from 'react';

import { Input } from '@/components/ui/input';
import { useDebouncedCommit } from '@/components/workflow/hooks/use-debounced-commit';
import { logWorkflowEditDebug } from '@/components/workflow/utils/edit-debug';
import { cn } from '@/lib/utils';
import { sanitizeIdentifier } from '@/utils/validation';

function isDeleteShortcut(event: React.KeyboardEvent<HTMLElement>): boolean {
  return event.key === 'Delete' || event.key === 'Backspace';
}

function isClipboardShortcut(event: React.KeyboardEvent<HTMLElement>): boolean {
  if (!event.ctrlKey && !event.metaKey) return false;
  const key = event.key.toLowerCase();
  return key === 'c' || key === 'v' || key === 'x';
}

/**
 * @component WorkflowIdentifierInput
 * @category Common
 * @status Beta
 * @description Debounced identifier input that keeps edits local until commit and blocks workflow-level delete and clipboard shortcuts while focused.
 * @usage Use for variable names and output keys that should remain stable during rapid typing inside workflow side panels.
 * @example
 * <WorkflowIdentifierInput initial="result" onCommit={setName} placeholder="Variable name" invalid={false} />
 */
export function WorkflowIdentifierInput({
  initial,
  onCommit,
  onBlurNormalize,
  placeholder,
  invalid,
  disabled = false,
  className,
  debugLabel,
}: {
  initial: string;
  onCommit: (value: string) => void;
  onBlurNormalize?: () => void;
  placeholder: string;
  invalid: boolean;
  disabled?: boolean;
  className?: string;
  debugLabel?: string;
}) {
  const { value, setValue, flush } = useDebouncedCommit<string>(initial || '', {
    delay: 350,
    onCommit,
    debugLabel,
    isEqual: (a, b) => a === b,
    flushOnUnmount: true,
  });

  const debug = useCallback(
    (message: string, data?: Record<string, unknown>) => {
      logWorkflowEditDebug(debugLabel ? `${debugLabel}:identifier` : undefined, message, data);
    },
    [debugLabel]
  );

  const stopShortcutPropagation = useCallback((event: React.KeyboardEvent<HTMLInputElement>) => {
    if (isDeleteShortcut(event) || isClipboardShortcut(event)) {
      event.stopPropagation();
    }
  }, []);

  return (
    <Input
      value={value}
      onChange={event => {
        const raw = event.target.value;
        const sanitized = sanitizeIdentifier(raw);
        debug('input change', { raw, sanitized, previousValue: value });
        setValue(sanitized);
      }}
      onBlur={() => {
        debug('input blur; flush and normalize', { value });
        flush();
        onBlurNormalize?.();
      }}
      onKeyDown={stopShortcutPropagation}
      placeholder={placeholder}
      className={cn(invalid ? 'border-destructive focus-visible:ring-destructive' : '', className)}
      disabled={disabled}
      aria-invalid={invalid || undefined}
    />
  );
}
