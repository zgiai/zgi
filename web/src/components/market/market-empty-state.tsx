'use client';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface MarketEmptyStateProps {
  title: string;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export default function MarketEmptyState({
  title,
  actionLabel,
  onAction,
  className,
}: MarketEmptyStateProps) {
  return (
    <div
      className={cn(
        'flex min-h-[420px] flex-col items-center justify-center px-4 py-16 text-center',
        className
      )}
    >
      <div className="relative mb-10 flex h-56 w-full max-w-xl items-end justify-center overflow-hidden">
        <div className="absolute inset-x-0 bottom-0 mx-auto h-48 max-w-md bg-[linear-gradient(to_right,hsl(var(--border)/0.35)_1px,transparent_1px),linear-gradient(to_bottom,hsl(var(--border)/0.35)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_10%,transparent_72%)]" />
        <div className="relative mb-8 w-[280px] rounded-lg border bg-background/90 p-4 shadow-sm">
          <div className="mb-3 h-3 w-28 rounded-full bg-muted" />
          <div className="mb-2 h-3 w-full rounded-full bg-muted" />
          <div className="mb-4 h-3 w-[88%] rounded-full bg-muted" />
          <div className="h-3 w-32 rounded-full bg-muted" />
          <div className="mt-4 flex">
            <span className="h-4 w-4 rounded-sm bg-amber-400 shadow-sm" />
            <span className="-ml-1 h-4 w-4 rounded-sm bg-rose-500 shadow-sm" />
            <span className="-ml-1 h-4 w-4 rounded-sm bg-emerald-500 shadow-sm" />
            <span className="-ml-1 h-4 w-4 rounded-sm bg-blue-500 shadow-sm" />
          </div>
        </div>
      </div>

      <h3 className="text-xl font-semibold text-foreground">{title}</h3>
      {actionLabel && onAction && (
        <Button className="mt-6 rounded-lg px-5" onClick={onAction}>
          {actionLabel}
        </Button>
      )}
    </div>
  );
}
