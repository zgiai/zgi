import * as React from 'react';
import { cn } from '@/lib/utils';

export type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement>;

const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => {
    return (
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
          className
        )}
        ref={ref}
        {...props}
      />
    );
  }
);
Textarea.displayName = 'Textarea';

export { Textarea };
