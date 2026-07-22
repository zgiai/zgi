'use client';

import { Button } from '@/components/ui/button';
import { SheetClose, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { RunStatusBadge } from '@/components/workflow/ui/run-status-badge';
import { RuntimeLogSourceBadge } from './runtime-log-source';

interface RuntimeLogDetailHeaderProps {
  title: string;
  description: string;
  runId?: string | null;
  status?: string | null;
  sourceLabel?: string | null;
  closeLabel: string;
  onClose?: () => void;
}

export function RuntimeLogDetailHeader({
  title,
  description,
  runId,
  status,
  sourceLabel,
  closeLabel,
  onClose,
}: RuntimeLogDetailHeaderProps) {
  return (
    <SheetHeader className="shrink-0 border-b px-5 py-4 text-left">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <SheetTitle className="text-base">{title}</SheetTitle>
            {status ? <RunStatusBadge status={status} /> : null}
            {sourceLabel ? <RuntimeLogSourceBadge label={sourceLabel} /> : null}
          </div>
          <SheetDescription className="mt-1 truncate" title={runId ?? ''}>
            {description}
          </SheetDescription>
        </div>
        <SheetClose asChild>
          <Button type="button" variant="outline" size="xs" className="shrink-0" onClick={onClose}>
            {closeLabel}
          </Button>
        </SheetClose>
      </div>
    </SheetHeader>
  );
}
