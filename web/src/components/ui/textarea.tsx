import * as React from 'react';
import { cn } from '@/lib/utils';

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  showCharacterCount?: boolean;
  characterCountClassName?: string;
}

const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, showCharacterCount, characterCountClassName, maxLength, value, defaultValue, ...props }, ref) => {
    const hasCharacterCount = showCharacterCount && typeof maxLength === 'number';
    const countValue = value ?? defaultValue ?? '';
    const count = Array.from(String(countValue)).length;
    const textarea = (
      <textarea
        className={cn(
          'flex min-h-[80px] max-h-[200px] w-full rounded-lg border bg-white dark:bg-zinc-900 px-3 py-2.5 text-sm text-foreground',
          'border-border shadow-sm',
          'transition-colors duration-200 ease-out',
          'placeholder:text-muted-foreground/80 placeholder:text-[13px]',
          'selection:bg-primary/20 selection:text-foreground',
          'hover:border-highlight',
          'focus-visible:outline-none focus-visible:border-primary/70',
          'aria-invalid:border-destructive',
          'disabled:cursor-not-allowed disabled:opacity-50 disabled:bg-muted/50',
          hasCharacterCount && 'pb-7',
          className
        )}
        ref={ref}
        maxLength={maxLength}
        value={value}
        defaultValue={defaultValue}
        {...props}
      />
    );

    if (!hasCharacterCount) return textarea;

    return (
      <div className="relative w-full">
        {textarea}
        <span
          className={cn(
            'pointer-events-none absolute bottom-2 right-3 rounded bg-background/80 px-1 text-[11px] text-muted-foreground',
            count > maxLength && 'text-destructive',
            characterCountClassName
          )}
        >
          {count}/{maxLength}
        </span>
      </div>
    );
  }
);
Textarea.displayName = 'Textarea';

export { Textarea };
