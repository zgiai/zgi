'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { WORKFLOW_NOTE_COMPACT_CLASS } from './form-density';

interface CompactNoteProps {
  icon?: React.ReactNode;
  title: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}

export function CompactNote({ icon, title, children, className }: CompactNoteProps) {
  return (
    <div className={cn(WORKFLOW_NOTE_COMPACT_CLASS, className)}>
      <div className="flex items-center gap-2 text-foreground">
        {icon ? <span className="shrink-0">{icon}</span> : null}
        <span className="font-medium">{title}</span>
      </div>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}
