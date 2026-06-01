'use client';

import * as React from 'react';
import { Checkbox } from '@/components/ui/checkbox';
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
import { Badge } from '@/components/ui/badge';
import { useCreateWorkflowTestBatch } from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import type { WorkflowTestCase } from '@/services/types/workflow-test';
import { formatQuestionTypeLabel } from './question-type';

interface CreateBatchDialogProps {
  agentId: string;
  cases: WorkflowTestCase[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function defaultBatchName(template: (values: { date: string; time: string }) => string) {
  const now = new Date();
  const yyyy = now.getFullYear();
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  const dd = String(now.getDate()).padStart(2, '0');
  const hh = String(now.getHours()).padStart(2, '0');
  const min = String(now.getMinutes()).padStart(2, '0');
  return template({ date: `${yyyy}-${mm}-${dd}`, time: `${hh}:${min}` });
}

export function CreateBatchDialog({ agentId, cases, open, onOpenChange }: CreateBatchDialogProps) {
  const t = useT('agents.workflowTest.dialogs.createBatch');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const enabledCases = React.useMemo(() => cases.filter(item => item.status === 'enabled'), [cases]);
  const createBatch = useCreateWorkflowTestBatch(agentId);
  const [name, setName] = React.useState('');
  const [selectedIds, setSelectedIds] = React.useState<string[]>([]);

  React.useEffect(() => {
    if (open) {
      setName(defaultBatchName(values => t('defaultName', values)));
      setSelectedIds(enabledCases.map(item => item.id));
    }
  }, [enabledCases, open, t]);

  const allSelected = enabledCases.length > 0 && selectedIds.length === enabledCases.length;
  const canSubmit = name.trim() && selectedIds.length > 0 && !createBatch.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" onInteractOutside={event => event.preventDefault()}>
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="workflow-test-batch-name">{t('nameLabel')}</Label>
            <Input
              id="workflow-test-batch-name"
              value={name}
              onChange={event => setName(event.target.value)}
              placeholder={t('namePlaceholder')}
            />
          </div>

          <div className="rounded-xl border border-slate-200">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div>
                <div className="font-medium text-slate-950">{t('selectCasesTitle')}</div>
                <div className="text-sm text-slate-500">
                  {t('selectedProgress', { selected: selectedIds.length, total: enabledCases.length })}
                </div>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={allSelected}
                  onCheckedChange={checked =>
                    setSelectedIds(checked ? enabledCases.map(item => item.id) : [])
                  }
                />
                {t('selectAll')}
              </label>
            </div>
            <div className="max-h-[360px] overflow-y-auto">
              {enabledCases.length === 0 ? (
                <div className="px-4 py-10 text-center text-sm text-slate-500">
                  {t('emptyEnabled')}
                </div>
              ) : (
                enabledCases.map(item => {
                  const selected = selectedIds.includes(item.id);
                  const hasAttachments = item.turns?.some(turn => turn.attachments?.length);
                  return (
                    <label
                      key={item.id}
                      className="flex cursor-pointer items-start gap-3 border-b border-slate-100 px-4 py-3 last:border-b-0 hover:bg-slate-50"
                    >
                      <Checkbox
                        checked={selected}
                        onCheckedChange={checked => {
                          setSelectedIds(prev =>
                            checked
                              ? Array.from(new Set([...prev, item.id]))
                              : prev.filter(id => id !== item.id)
                          );
                        }}
                      />
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium text-slate-950">{item.content}</div>
                        <div className="mt-2 flex items-center gap-2">
                          <Badge variant="outline">
                            {formatQuestionTypeLabel(item.question_type, typeT)}
                          </Badge>
                          {hasAttachments ? (
                            <Badge className="bg-blue-50 text-blue-700">
                              {commonT('attachmentsIncluded')}
                            </Badge>
                          ) : null}
                        </div>
                      </div>
                    </label>
                  );
                })
              )}
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {commonT('cancel')}
          </Button>
          <Button
            disabled={!canSubmit}
            onClick={() => {
              createBatch.mutate(
                { name, case_ids: selectedIds },
                { onSuccess: () => onOpenChange(false) }
              );
            }}
          >
            {t('submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
