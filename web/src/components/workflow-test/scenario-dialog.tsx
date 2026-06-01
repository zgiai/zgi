'use client';

import * as React from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useSaveWorkflowTestScenarios } from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { WorkflowTestScenario } from '@/services/types/workflow-test';

interface ScenarioDialogProps {
  agentId: string;
  scenarios: WorkflowTestScenario[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface EditableScenario {
  clientId: string;
  id?: string;
  name: string;
  description: string;
  caseCount: number;
}

function createEditableScenario(): EditableScenario {
  return {
    clientId: `new-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    name: '',
    description: '',
    caseCount: 0,
  };
}

export function ScenarioDialog({ agentId, scenarios, open, onOpenChange }: ScenarioDialogProps) {
  const t = useT('agents.workflowTest.dialogs.scenario');
  const commonT = useT('agents.workflowTest.common');
  const saveScenarios = useSaveWorkflowTestScenarios(agentId);
  const [items, setItems] = React.useState<EditableScenario[]>([]);
  const [invalidNameClientId, setInvalidNameClientId] = React.useState<string | null>(null);
  const nameInputRefs = React.useRef(new Map<string, HTMLInputElement>());

  React.useEffect(() => {
    if (open) {
      setInvalidNameClientId(null);
      setItems(
        scenarios.length > 0
          ? scenarios.map(scenario => ({
              clientId: scenario.id,
              id: scenario.id,
              name: scenario.name,
              description: scenario.description,
              caseCount: scenario.case_count,
            }))
          : [createEditableScenario()]
      );
    }
  }, [open, scenarios]);

  const updateItem = (clientId: string, patch: Partial<EditableScenario>) => {
    setItems(prev => prev.map(item => (item.clientId === clientId ? { ...item, ...patch } : item)));
    if (patch.name !== undefined && patch.name.trim() && invalidNameClientId === clientId) {
      setInvalidNameClientId(null);
    }
  };

  const removeItem = (clientId: string) => {
    setItems(prev => prev.filter(item => item.clientId !== clientId));
    if (invalidNameClientId === clientId) {
      setInvalidNameClientId(null);
    }
  };

  const hasClearedAllScenarios = scenarios.length > 0 && items.length === 0;
  const canSubmit = (items.length > 0 || hasClearedAllScenarios) && !saveScenarios.isPending;

  const setNameInputRef = React.useCallback(
    (clientId: string) => (node: HTMLInputElement | null) => {
      if (node) {
        nameInputRefs.current.set(clientId, node);
      } else {
        nameInputRefs.current.delete(clientId);
      }
    },
    []
  );

  const handleSubmit = () => {
    const invalidItem = items.find(item => !item.name.trim());
    if (invalidItem) {
      setInvalidNameClientId(invalidItem.clientId);
      requestAnimationFrame(() => {
        nameInputRefs.current.get(invalidItem.clientId)?.focus();
      });
      return;
    }
    saveScenarios.mutate(
      {
        scenarios: items.map(item => ({
          id: item.id,
          name: item.name.trim(),
          description: item.description.trim(),
        })),
      },
      { onSuccess: () => onOpenChange(false) }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="xl"
        className="max-h-[88vh] max-w-[960px] overflow-hidden rounded-2xl"
        onInteractOutside={event => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[62vh] space-y-4 overflow-y-auto pr-1">
          {items.map((item, index) => (
            <div key={item.clientId} className="rounded-xl border border-slate-200 bg-slate-50 p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1 text-sm text-slate-500">
                  <div className="font-semibold">{t('itemTitle', { index: index + 1 })}</div>
                  {item.id ? <div>{t('caseCount', { count: item.caseCount })}</div> : null}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={item.caseCount > 0}
                  className={cn(
                    item.caseCount > 0 ? 'text-slate-400' : 'text-red-600 hover:text-red-700'
                  )}
                  onClick={() => removeItem(item.clientId)}
                >
                  <Trash2 className="mr-1 size-4" />
                  {commonT('delete')}
                </Button>
              </div>
              <div className="mt-4 grid gap-4 md:grid-cols-[0.8fr_1.2fr]">
                <div className="space-y-2">
                  <Label>
                    {t('nameLabel')}
                    <span className="ml-0.5 text-red-500">*</span>
                  </Label>
                  <Input
                    ref={setNameInputRef(item.clientId)}
                    value={item.name}
                    onChange={event => updateItem(item.clientId, { name: event.target.value })}
                    placeholder={t('namePlaceholder')}
                    aria-invalid={invalidNameClientId === item.clientId ? 'true' : 'false'}
                    className={cn(
                      invalidNameClientId === item.clientId &&
                        'border-red-300 focus-visible:ring-red-200'
                    )}
                  />
                  {invalidNameClientId === item.clientId ? (
                    <p className="text-xs font-medium text-red-600">{t('nameRequired')}</p>
                  ) : null}
                </div>
                <div className="space-y-2">
                  <Label>{t('descriptionLabel')}</Label>
                  <Input
                    value={item.description}
                    onChange={event =>
                      updateItem(item.clientId, { description: event.target.value })
                    }
                    placeholder={t('descriptionPlaceholder')}
                  />
                </div>
              </div>
            </div>
          ))}
          <Button
            variant="outline"
            onClick={() => setItems(prev => [...prev, createEditableScenario()])}
          >
            <Plus className="mr-2 size-4" />
            {t('add')}
          </Button>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {commonT('cancel')}
          </Button>
          <Button
            disabled={!canSubmit}
            onClick={handleSubmit}
          >
            {t('submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
