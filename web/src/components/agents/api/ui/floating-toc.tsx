'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import Toc from './toc';
import { cn } from '@/lib/utils';
import { List, X } from 'lucide-react';

export function FloatingToc({ rootRef }: { rootRef: React.RefObject<HTMLElement> }) {
  const [open, setOpen] = React.useState(true);

  return (
    <div className={cn('sticky top-4 z-10 flex w-[280px] flex-col items-end')}>
      <Button
        variant="outline"
        isIcon
        aria-label={open ? 'close toc' : 'open toc'}
        aria-expanded={open}
        className={cn(
          'h-9 w-9 rounded-full shadow-md transition-all duration-300 ease-out transform-gpu',
          open ? 'bg-primary/5 text-primary' : 'bg-background text-foreground',
          'hover:bg-muted active:scale-95'
        )}
        onClick={() => setOpen(v => !v)}
      >
        <span
          className={cn(
            'transition-transform duration-300 ease-out',
            open ? 'rotate-180' : 'rotate-0'
          )}
        >
          {open ? <X className="h-4 w-4" /> : <List className="h-4 w-4" />}
        </span>
      </Button>

      {/* Always mounted panel to avoid flicker on close */}
      <div
        id="floating-toc-panel"
        data-state={open ? 'open' : 'closed'}
        aria-hidden={!open}
        className={cn(
          'mt-2 w-[280px] rounded-lg border bg-popover text-popover-foreground shadow-lg p-3',
          'transition-[opacity,transform,max-height] duration-250 ease-out transform-gpu will-change-[opacity,transform]',
          'overflow-hidden',
          open
            ? 'opacity-100 translate-y-0 scale-100 max-h-[60vh]'
            : 'opacity-0 -translate-y-1 scale-95 max-h-0 pointer-events-none'
        )}
      >
        <Toc rootRef={rootRef} variant="floating" />
      </div>
    </div>
  );
}
