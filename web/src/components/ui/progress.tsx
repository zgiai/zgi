import * as React from 'react';

import { cn } from '@/lib/utils';

export interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Current progress 0-100 */
  value?: number;
}

/**
 * Progress
 * ---------------------------------------------------------------------------
 * Simple horizontal progress bar. Accepts an optional `value` (0-100). If no
 * value is provided, renders an indeterminate animation.
 * ---------------------------------------------------------------------------
 */
export const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
  ({ className, value, ...props }, ref) => {
    return (
      <div
        ref={ref}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={typeof value === 'number' ? value : undefined}
        className={cn('relative h-2 w-full overflow-hidden rounded-full bg-muted', className)}
        {...props}
      >
        {typeof value === 'number' ? (
          <span
            className="block h-full w-full rounded-full bg-primary transition-transform"
            style={{ transform: `translateX(-${100 - Math.min(100, Math.max(0, value))}%)` }}
          />
        ) : (
          // Indeterminate – animated bar
          <span className="absolute inset-0 flex -translate-x-full animate-[progress-indeterminate_2s_linear_infinite]">
            <span className="h-full w-full flex-1 rounded-full bg-primary" />
          </span>
        )}
      </div>
    );
  }
);
Progress.displayName = 'Progress';

/* -------------------------------------------------------------------------- */
/* Keyframes (global)                                                         */
/* -------------------------------------------------------------------------- */

// Add keyframes via inline style tag once to avoid tailwind config changes
if (typeof document !== 'undefined' && !document.getElementById('progress-keyframes')) {
  const style = document.createElement('style');
  style.id = 'progress-keyframes';
  style.innerHTML = `@keyframes progress-indeterminate { 0% { transform: translateX(-100%); } 100% { transform: translateX(0); } }`;
  document.head.appendChild(style);
}
