'use client';

import type { ReactNode } from 'react';
import { Plus } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { RuntimeSection } from './runtime-section';
import type { AgentConfigSection } from './types';

interface AgentRuntimeResourceSectionProps {
  title: string;
  section: AgentConfigSection;
  open: boolean;
  count: number;
  addLabel: string;
  helpText: string;
  emptyText: string;
  isLoading: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onAdd: () => void;
  addTooltip?: string;
  readOnly?: boolean;
  children: ReactNode;
}

interface AgentRuntimeResourceCardProps {
  icon: ReactNode;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  children?: ReactNode;
  error?: boolean;
}

export function AgentRuntimeResourceSection({
  title,
  section,
  open,
  count,
  addLabel,
  helpText,
  emptyText,
  isLoading,
  onToggleSection,
  onAdd,
  addTooltip,
  readOnly = false,
  children,
}: AgentRuntimeResourceSectionProps) {
  const action = (
    <div className="flex items-center gap-2">
      <Badge variant="subtle">{count}</Badge>
      <ResourceAddButton label={addLabel} tooltip={addTooltip} onAdd={onAdd} disabled={readOnly} />
    </div>
  );

  return (
    <RuntimeSection
      title={title}
      section={section}
      open={open}
      onToggle={onToggleSection}
      action={action}
    >
      <div className="space-y-3">
        <div className="rounded-md border bg-muted/25 p-3 text-xs leading-5 text-muted-foreground">
          {helpText}
        </div>

        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : count === 0 ? (
          <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
            {emptyText}
          </div>
        ) : (
          children
        )}
      </div>
    </RuntimeSection>
  );
}

export function AgentRuntimeResourceCard({
  icon,
  title,
  description,
  action,
  children,
  error,
}: AgentRuntimeResourceCardProps) {
  return (
    <div className="rounded-md border bg-background p-3">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
          <span className={error ? 'text-destructive' : undefined}>{icon}</span>
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{title}</div>
          {description ? (
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
              {description}
            </div>
          ) : null}
        </div>
        {action}
      </div>
      {children ? <div className="mt-3">{children}</div> : null}
    </div>
  );
}

function ResourceAddButton({
  label,
  tooltip,
  onAdd,
  disabled = false,
}: {
  label: string;
  tooltip?: string;
  onAdd: () => void;
  disabled?: boolean;
}) {
  const button = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      isIcon
      className="size-8"
      aria-label={label}
      disabled={disabled}
      onClick={event => {
        event.stopPropagation();
        if (disabled) return;
        onAdd();
      }}
    >
      <Plus className="size-4" />
    </Button>
  );

  if (!tooltip) return button;

  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}
