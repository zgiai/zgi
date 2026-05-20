'use client';

import React from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import type { CreateScheduledTaskActionData } from '../config';
import { scheduledTaskActionRegistry } from '../registry';

interface ActionListProps {
  actions: CreateScheduledTaskActionData[];
  selectedActionId: string | null;
  readOnly?: boolean;
  onOpen: (clientId: string) => void;
  onAdd: () => void;
  onDelete: (clientId: string) => void;
  onToggleEnabled: (clientId: string, enabled: boolean) => void;
}

/**
 * @component ActionList
 * @category Feature
 * @status Beta
 * @description List and selection panel for scheduled task actions.
 * @usage Render beside ActionEditor to choose which action draft is being edited.
 * @example
 * <ActionList actions={actions} selectedActionId={selectedId} onSelect={setSelectedId} onAdd={add} onDelete={remove} onToggleEnabled={toggle} />
 */
export function ActionList({
  actions,
  selectedActionId,
  readOnly = false,
  onOpen,
  onAdd,
  onDelete,
  onToggleEnabled,
}: ActionListProps) {
  const t = useT('nodes');
  const tCommon = useT('common');

  return (
    <div className="rounded-2xl border border-border bg-background p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-semibold text-foreground">
          {t('createScheduledTask.section.actions')}
        </h3>

        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onAdd}
          disabled={readOnly}
          className="rounded-xl"
        >
          <Plus className="size-4" />
          {t('createScheduledTask.actions.addAction')}
        </Button>
      </div>

      {actions.length > 0 ? (
        <div className="mt-4 space-y-3">
          {actions.map((action, index) => {
            const meta = scheduledTaskActionRegistry[action.action_type];
            const ActionIcon = meta.icon;
            const selected = action.client_id === selectedActionId;

            return (
              <div
                key={action.client_id}
                className={cn(
                  'flex items-center gap-2 rounded-2xl border px-2 py-1 transition-all',
                  selected
                    ? 'border-primary/40 bg-primary/5 shadow-sm'
                    : 'border-border/70 bg-background hover:border-primary/20'
                )}
              >
                <button
                  type="button"
                  onClick={() => onOpen(action.client_id)}
                  className="min-w-0 flex-1 rounded-xl px-1 py-1 text-left transition-all"
                >
                  <div className="flex items-center gap-2">
                    <div
                      className={cn(
                        'flex size-8 shrink-0 items-center justify-center rounded-xl border',
                        selected
                          ? 'border-primary/20 bg-primary/10 text-primary'
                          : 'border-border bg-muted text-muted-foreground'
                      )}
                    >
                      <ActionIcon className="size-4" />
                    </div>

                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-foreground">
                        {t(meta.labelKey as never)}
                      </p>
                    </div>
                  </div>
                </button>

                <div className="flex shrink-0 items-center gap-2">
                  <span className="text-xs font-medium text-muted-foreground">
                    {action.enabled ? tCommon('enabled') : tCommon('disabled')}
                  </span>
                  <Switch
                    checked={action.enabled}
                    onCheckedChange={enabled => onToggleEnabled(action.client_id, enabled)}
                    disabled={readOnly}
                    aria-label={t('createScheduledTask.fields.enabled')}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    className="size-8 rounded-full text-muted-foreground hover:text-destructive"
                    onClick={() => onDelete(action.client_id)}
                    disabled={readOnly}
                    aria-label={t('createScheduledTask.actions.deleteAction', { index: index + 1 })}
                    title={t('createScheduledTask.actions.deleteAction', { index: index + 1 })}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="mt-4 flex min-h-[220px] flex-col items-center justify-center rounded-2xl border border-dashed border-border bg-muted/10 px-6 py-10 text-center">
          <p className="text-sm font-medium text-foreground">
            {t('createScheduledTask.empty.noActionsTitle')}
          </p>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">
            {t('createScheduledTask.empty.noActionsDescription')}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onAdd}
            disabled={readOnly}
            className="mt-4 rounded-xl"
          >
            <Plus className="size-4" />
            {t('createScheduledTask.actions.addAction')}
          </Button>
        </div>
      )}
    </div>
  );
}

export default ActionList;
