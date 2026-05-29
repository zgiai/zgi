'use client';

import * as React from 'react';
import { FileText, ImageIcon, Paperclip, Plus, Trash2, X } from 'lucide-react';
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
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import {
  useCreateWorkflowTestCase,
  useUpdateWorkflowTestCase,
} from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import type { FileItem } from '@/services/types/file';
import type { WorkflowTestAttachment, WorkflowTestCase } from '@/services/types/workflow-test';
import { QUESTION_TYPE_OPTIONS } from './question-type';

interface CaseDialogProps {
  agentId: string;
  scenarios: Array<{ id: string; name: string }>;
  caseItem?: WorkflowTestCase | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  supportsAttachments?: boolean;
}

interface EditableTurn {
  id: string;
  content: string;
  attachments: WorkflowTestAttachment[];
  selectedFiles: FileItem[];
}

function createTurn(content = '', attachments: WorkflowTestAttachment[] = []): EditableTurn {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`,
    content,
    attachments,
    selectedFiles: [],
  };
}

function inferAttachmentType(file: FileItem): string {
  if (file.mime_type?.startsWith('image/')) return 'image';
  return 'document';
}

function attachmentFromFile(file: FileItem): WorkflowTestAttachment {
  return {
    type: inferAttachmentType(file),
    transfer_method: 'local_file',
    upload_file_id: file.id,
    name: file.name,
  };
}

function attachmentKey(file: WorkflowTestAttachment): string {
  return file.upload_file_id || file.url || file.name || '';
}

export function CaseDialog({
  agentId,
  scenarios,
  caseItem,
  open,
  onOpenChange,
  supportsAttachments = true,
}: CaseDialogProps) {
  const t = useT('agents.workflowTest.dialogs.case');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const createCase = useCreateWorkflowTestCase(agentId);
  const updateCase = useUpdateWorkflowTestCase(agentId);
  const [fileSelectorTurnId, setFileSelectorTurnId] = React.useState<string | null>(null);
  const [turns, setTurns] = React.useState<EditableTurn[]>(() => [createTurn()]);
  const [scenarioId, setScenarioId] = React.useState('');
  const [expectedResult, setExpectedResult] = React.useState('');
  const [questionType, setQuestionType] = React.useState('core');
  const [status, setStatus] = React.useState('enabled');

  React.useEffect(() => {
    if (open && caseItem) {
      const existingTurns = caseItem.turns?.length
        ? caseItem.turns.map(turn => createTurn(turn.content, turn.attachments ?? []))
        : [createTurn(caseItem.content)];
      setTurns(existingTurns.length ? existingTurns : [createTurn(caseItem.content)]);
      setScenarioId(caseItem.scenario_id || '');
      setExpectedResult(caseItem.expected_result || '');
      setQuestionType(caseItem.question_type || 'core');
      setStatus(caseItem.status || 'enabled');
      setFileSelectorTurnId(null);
      return;
    }
    if (!open || !caseItem) {
      setTurns([createTurn()]);
      setScenarioId(scenarios[0]?.id ?? '');
      setExpectedResult('');
      setQuestionType('core');
      setStatus('enabled');
      setFileSelectorTurnId(null);
    }
  }, [caseItem, open, scenarios]);

  const isPending = createCase.isPending || updateCase.isPending;
  const normalizedTurns = React.useMemo(
    () =>
      turns
        .map(turn => ({
          role: 'user',
          content: turn.content.trim(),
          attachments: supportsAttachments ? turn.attachments : [],
        }))
        .filter(turn => turn.content || turn.attachments.length > 0),
    [supportsAttachments, turns]
  );
  const content = normalizedTurns.find(turn => turn.content)?.content ?? '';
  const canSubmit = Boolean(content) && Boolean(scenarioId) && !isPending;
  const title = caseItem ? t('editTitle') : t('createTitle');
  const activeTurn = turns.find(turn => turn.id === fileSelectorTurnId);

  const updateTurn = (turnId: string, patch: Partial<EditableTurn>) => {
    setTurns(prev => prev.map(turn => (turn.id === turnId ? { ...turn, ...patch } : turn)));
  };

  const removeTurn = (turnId: string) => {
    setTurns(prev => (prev.length <= 1 ? prev : prev.filter(turn => turn.id !== turnId)));
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{t('description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-5">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div className="space-y-2">
                <Label>{t('scenarioLabel')}</Label>
                <Select value={scenarioId} onValueChange={setScenarioId}>
                  <SelectTrigger>
                    <SelectValue placeholder={t('scenarioPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {scenarios.map(scene => (
                      <SelectItem key={scene.id} value={scene.id}>
                        {scene.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t('typeLabel')}</Label>
                <Select value={questionType} onValueChange={setQuestionType}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {QUESTION_TYPE_OPTIONS.map(item => (
                      <SelectItem key={item.value} value={item.value}>
                        {typeT(item.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t('statusLabel')}</Label>
                <Select value={status} onValueChange={setStatus}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="enabled">{commonT('enabled')}</SelectItem>
                    <SelectItem value="disabled">{commonT('disabled')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-3">
              <div>
                <Label>{t('turnsLabel')}</Label>
                <p className="mt-1 text-sm text-slate-500">{t('turnsDescription')}</p>
              </div>
              <div className="space-y-3">
                {turns.map((turn, index) => (
                  <div key={turn.id} className="rounded-lg border border-slate-200 bg-slate-50 p-4">
                    <div className="mb-3 flex items-center justify-between gap-3">
                      <div className="font-semibold text-slate-950">
                        {t('turnTitle', { index: index + 1 })}
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={index === 0 || turns.length === 1}
                        onClick={() => removeTurn(turn.id)}
                      >
                        <Trash2 className="mr-1 size-4" />
                        {t('removeTurn')}
                      </Button>
                    </div>
                    <Textarea
                      value={turn.content}
                      onChange={event => updateTurn(turn.id, { content: event.target.value })}
                      placeholder={t('contentPlaceholder')}
                      className="min-h-24 resize-none bg-white"
                    />
                    {supportsAttachments ? (
                      <div className="mt-3 space-y-2">
                        <div className="flex items-center justify-between">
                          <div className="text-sm font-medium text-slate-700">{t('attachmentsLabel')}</div>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={() => setFileSelectorTurnId(turn.id)}
                          >
                            <Paperclip className="mr-2 size-4" />
                            {t('selectFiles')}
                          </Button>
                        </div>
                        {turn.attachments.length === 0 ? (
                          <div className="rounded-lg border border-dashed border-slate-200 bg-white px-4 py-4 text-center text-sm text-slate-500">
                            {t('emptyAttachments')}
                          </div>
                        ) : (
                          <div className="space-y-2">
                            {turn.attachments.map(file => (
                              <div
                                key={attachmentKey(file)}
                                className="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-3 py-2"
                              >
                                <div className="flex min-w-0 items-center gap-2">
                                  {file.type === 'image' ? (
                                    <ImageIcon className="size-4 shrink-0 text-blue-500" />
                                  ) : (
                                    <FileText className="size-4 shrink-0 text-slate-500" />
                                  )}
                                  <div className="min-w-0 truncate text-sm">
                                    {file.name || file.upload_file_id || file.url}
                                  </div>
                                </div>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  className="h-7 w-7 p-0"
                                  onClick={() =>
                                    updateTurn(turn.id, {
                                      attachments: turn.attachments.filter(
                                        item => attachmentKey(item) !== attachmentKey(file)
                                      ),
                                    })
                                  }
                                >
                                  <X className="size-4" />
                                </Button>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => setTurns(prev => [...prev, createTurn()])}
              >
                <Plus className="mr-2 size-4" />
                {t('addTurn')}
              </Button>
            </div>

            <div className="space-y-2">
              <Label htmlFor="workflow-test-case-expected-result">{t('expectedResultLabel')}</Label>
              <Textarea
                id="workflow-test-case-expected-result"
                value={expectedResult}
                onChange={event => setExpectedResult(event.target.value)}
                placeholder={t('expectedResultPlaceholder')}
                className="min-h-28 resize-none"
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {commonT('cancel')}
            </Button>
            <Button
              disabled={!canSubmit}
              onClick={() => {
                const data = {
                  content,
                  expected_result: expectedResult,
                  scenario_id: scenarioId || undefined,
                  question_type: questionType,
                  status,
                  turns: normalizedTurns,
                };
                if (caseItem) {
                  updateCase.mutate(
                    { caseId: caseItem.id, data },
                    { onSuccess: () => onOpenChange(false) }
                  );
                } else {
                  createCase.mutate(data, { onSuccess: () => onOpenChange(false) });
                }
              }}
            >
              {t('submit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <FileSelectorDialog
        open={Boolean(fileSelectorTurnId)}
        onOpenChange={nextOpen => {
          if (!nextOpen) setFileSelectorTurnId(null);
        }}
        initSelectedFiles={activeTurn?.selectedFiles ?? []}
        onConfirm={files => {
          if (!fileSelectorTurnId) return;
          const nextAttachments = files.map(attachmentFromFile);
          const nextKeys = new Set(nextAttachments.map(attachmentKey));
          setTurns(prev =>
            prev.map(turn =>
              turn.id === fileSelectorTurnId
                ? {
                    ...turn,
                    selectedFiles: files,
                    attachments: [
                      ...turn.attachments.filter(item => !nextKeys.has(attachmentKey(item))),
                      ...nextAttachments,
                    ],
                  }
                : turn
            )
          );
        }}
        maxCount={10}
      />
    </>
  );
}
