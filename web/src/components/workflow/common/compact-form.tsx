'use client';

import React from 'react';
import type * as ToggleGroupPrimitive from '@radix-ui/react-toggle-group';
import { type VariantProps } from 'class-variance-authority';
import { Label } from '@/components/ui/label';
import { SelectTrigger } from '@/components/ui/select';
import { TabsList, TabsTrigger } from '@/components/ui/tabs';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { toggleVariants } from '@/components/ui/toggle';
import { cn } from '@/lib/utils';
import {
  WORKFLOW_CONTROL_COMPACT_CLASS,
  WORKFLOW_FIELD_HINT_COMPACT_CLASS,
  WORKFLOW_FIELD_LABEL_COMPACT_CLASS,
  WORKFLOW_TABS_LIST_COMPACT_CLASS,
  WORKFLOW_TABS_TRIGGER_COMPACT_CLASS,
} from './form-density';

interface WorkflowCompactFieldLabelProps extends React.ComponentProps<typeof Label> {}

export function WorkflowCompactFieldLabel({
  className,
  ...props
}: WorkflowCompactFieldLabelProps) {
  return <Label className={cn(WORKFLOW_FIELD_LABEL_COMPACT_CLASS, className)} {...props} />;
}

interface WorkflowCompactFieldHintProps extends React.HTMLAttributes<HTMLParagraphElement> {}

export function WorkflowCompactFieldHint({
  className,
  ...props
}: WorkflowCompactFieldHintProps) {
  return <p className={cn(WORKFLOW_FIELD_HINT_COMPACT_CLASS, className)} {...props} />;
}

interface WorkflowCompactSelectTriggerProps
  extends React.ComponentPropsWithoutRef<typeof SelectTrigger> {}

export function WorkflowCompactSelectTrigger({
  className,
  ...props
}: WorkflowCompactSelectTriggerProps) {
  return <SelectTrigger className={cn(WORKFLOW_CONTROL_COMPACT_CLASS, className)} {...props} />;
}

interface WorkflowCompactTabsListProps extends React.ComponentPropsWithoutRef<typeof TabsList> {}

export function WorkflowCompactTabsList({
  className,
  ...props
}: WorkflowCompactTabsListProps) {
  return <TabsList className={cn(WORKFLOW_TABS_LIST_COMPACT_CLASS, className)} {...props} />;
}

interface WorkflowCompactTabsTriggerProps
  extends React.ComponentPropsWithoutRef<typeof TabsTrigger> {}

export function WorkflowCompactTabsTrigger({
  className,
  ...props
}: WorkflowCompactTabsTriggerProps) {
  return <TabsTrigger className={cn(WORKFLOW_TABS_TRIGGER_COMPACT_CLASS, className)} {...props} />;
}

type WorkflowCompactToggleGroupProps =
  | (ToggleGroupPrimitive.ToggleGroupSingleProps & VariantProps<typeof toggleVariants>)
  | (ToggleGroupPrimitive.ToggleGroupMultipleProps & VariantProps<typeof toggleVariants>);

export function WorkflowCompactToggleGroup({
  className,
  ...props
}: WorkflowCompactToggleGroupProps) {
  const compactClassName = cn(
    'inline-flex gap-1 rounded-xl border border-border bg-muted/35 p-1 shadow-xs',
    className
  );

  if (props.type === 'multiple') {
    return (
      <ToggleGroup
        {...props}
        className={compactClassName}
        variant={props.variant ?? 'default'}
        size={props.size ?? 'sm'}
      />
    );
  }

  return (
    <ToggleGroup
      {...props}
      className={compactClassName}
      variant={props.variant ?? 'default'}
      size={props.size ?? 'sm'}
    />
  );
}

interface WorkflowCompactToggleGroupItemProps
  extends React.ComponentPropsWithoutRef<typeof ToggleGroupItem> {
  active?: boolean;
}

export function WorkflowCompactToggleGroupItem({
  active = false,
  className,
  ...props
}: WorkflowCompactToggleGroupItemProps) {
  return (
    <ToggleGroupItem
      className={cn(
        'h-8 min-w-[72px] gap-1.5 rounded-lg border border-transparent px-3 text-[12px] font-medium text-muted-foreground transition-all hover:text-foreground',
        active &&
          '!border-primary/45 !bg-background !text-primary shadow-[0_1px_2px_rgba(15,23,42,0.08)]',
        className
      )}
      {...props}
    />
  );
}
