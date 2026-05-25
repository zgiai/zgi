'use client';

import type { ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AgentConfigSection } from './types';

interface RuntimeSectionProps {
  title: string;
  section: AgentConfigSection;
  open: boolean;
  onToggle: (section: AgentConfigSection) => void;
  action?: ReactNode;
  children: ReactNode;
}

export function RuntimeSection({
  title,
  section,
  open,
  onToggle,
  action,
  children,
}: RuntimeSectionProps) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          className="flex min-w-0 items-center gap-2 text-sm font-medium"
          onClick={() => onToggle(section)}
        >
          <ChevronDown
            className={cn(
              'size-4 shrink-0 text-muted-foreground transition-transform',
              !open ? '-rotate-90' : ''
            )}
          />
          <span className="truncate">{title}</span>
        </button>
        {action}
      </div>
      {open ? children : null}
    </section>
  );
}
