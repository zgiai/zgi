'use client';

import React from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { WorkflowValueEditor } from '@/components/workflow/ui';

interface WorkflowValueListEditorLabels {
  title: string;
  add: string;
  placeholder: (index: number) => string;
  remove: (index: number) => string;
}

interface WorkflowValueListEditorProps {
  nodeId: string;
  value: string[];
  onChange: (next: string[]) => void;
  labels: WorkflowValueListEditorLabels;
  readOnly?: boolean;
  className?: string;
  addButtonPlacement?: 'bottom' | 'header';
  portalRoot?: React.ComponentProps<typeof WorkflowValueEditor>['portalRoot'];
}

/**
 * @component WorkflowValueListEditor
 * @category Feature
 * @status Beta
 * @description Variable-aware list editor for arrays of workflow string values.
 * @usage Use for fields like email recipients that should support plain text and upstream variable tokens.
 * @example
 * <WorkflowValueListEditor nodeId={nodeId} value={recipients} onChange={setRecipients} labels={labels} />
 */
export function WorkflowValueListEditor({
  nodeId,
  value,
  onChange,
  labels,
  readOnly = false,
  className,
  addButtonPlacement = 'bottom',
  portalRoot,
}: WorkflowValueListEditorProps) {
  const items = value;

  const updateItem = React.useCallback(
    (index: number, nextValue: string) => {
      onChange(items.map((item, itemIndex) => (itemIndex === index ? nextValue : item)));
    },
    [items, onChange]
  );

  const handleAdd = React.useCallback(() => {
    onChange([...items, '']);
  }, [items, onChange]);

  const handleRemove = React.useCallback(
    (index: number) => {
      onChange(items.filter((_, itemIndex) => itemIndex !== index));
    },
    [items, onChange]
  );

  return (
    <div className={cn('space-y-2.5', className)}>
      <div className="flex items-center justify-between gap-3">
        <p className="text-[13px] font-medium text-foreground">{labels.title}</p>
        {addButtonPlacement === 'header' ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAdd}
            disabled={readOnly}
            className="h-8 rounded-xl border-dashed px-3 text-sm font-medium hover:bg-muted"
          >
            <Plus className="size-4" />
            {labels.add}
          </Button>
        ) : null}
      </div>

      {items.length > 0 ? (
        <div className="space-y-2">
          {items.map((item, index) => (
            <div
              key={`workflow-value-list-item-${index}`}
              className="flex items-start gap-2 rounded-xl border border-border bg-muted/20 p-2"
            >
              <WorkflowValueEditor
                nodeId={nodeId}
                value={item}
                onChange={nextValue => updateItem(index, nextValue)}
                readOnly={readOnly}
                portalRoot={portalRoot}
                placeholder={labels.placeholder(index)}
                className="flex-1"
                editorClassName="min-h-[40px] rounded-lg border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
              />

              <Button
                type="button"
                variant="ghost"
                size="sm"
                isIcon
                className="mt-0.5 size-8 rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                onClick={() => handleRemove(index)}
                disabled={readOnly}
                aria-label={labels.remove(index)}
                title={labels.remove(index)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      ) : null}

      {addButtonPlacement === 'bottom' ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleAdd}
          disabled={readOnly}
          className="h-9 rounded-xl border-dashed px-3.5 text-sm font-medium hover:bg-muted"
        >
          <Plus className="size-4" />
          {labels.add}
        </Button>
      ) : null}
    </div>
  );
}

export default WorkflowValueListEditor;
