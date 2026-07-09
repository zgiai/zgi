'use client';

import * as React from 'react';
import { ChevronDown, FileText, ImageIcon, Paperclip, Plus, SlidersHorizontal, Trash2, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
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
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import {
  useCreateWorkflowTestCase,
  useUpdateWorkflowTestCase,
} from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { FileItem } from '@/services/types/file';
import type { WorkflowTestAttachment, WorkflowTestCase } from '@/services/types/workflow-test';
import { QUESTION_TYPE_OPTIONS } from './question-type';
import {
  CASE_MODE_KEY,
  CONVERSATION_CHECKS_KEY,
  EXPECTED_CHECKS_KEY,
  TURN_CHECKS_KEY,
  TURN_EXPECTATION_KEY,
  buildConversationChecksPayload,
  buildExpectedChecksPayload,
  buildWorkflowCheckOptions,
  buildWorkflowInputVariableOptions,
  conversationCheckTypes,
  createConversationCheckCondition,
  createExpectedCheckCondition,
  fixtureSpecsFromInputs,
  normalizeConversationChecks,
  normalizeExpectedChecks,
  parseLineList,
  serializeLineList,
  turnInputs,
  type ConversationCheckCondition,
  type ConversationCheckType,
  type ExpectedCheckCondition,
  type ExpectedCheckType,
  type WorkflowCheckOption,
  type WorkflowInputVariableOption,
  type WorkflowTestMode,
} from './case-metadata';

interface CaseDialogProps {
  agentId: string;
  scenarios: Array<{ id: string; name: string }>;
  caseItem?: WorkflowTestCase | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  supportsAttachments?: boolean;
  attachmentAcceptExt?: string[];
  mode?: WorkflowTestMode;
  workflowDraft?: { graph?: { nodes?: unknown[] } };
}

interface EditableTurn {
  id: string;
  content: string;
  expectation: string;
  checks: ConversationCheckCondition[];
  attachments: WorkflowTestAttachment[];
  selectedFiles: FileItem[];
  inputs: Record<string, unknown>;
}

interface TaskVariableRow {
  id: string;
  key: string;
  value: string;
}

function createTurn(
  content = '',
  attachments: WorkflowTestAttachment[] = [],
  inputs: Record<string, unknown> = {}
): EditableTurn {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`,
    content,
    expectation: typeof inputs[TURN_EXPECTATION_KEY] === 'string' ? inputs[TURN_EXPECTATION_KEY] : '',
    checks: normalizeConversationChecks(inputs[TURN_CHECKS_KEY], typeof inputs[TURN_EXPECTATION_KEY] === 'string' ? inputs[TURN_EXPECTATION_KEY] : '').conditions ?? [],
    attachments,
    selectedFiles: [],
    inputs,
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

function createLocalId(prefix: string) {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}_${Date.now()}_${Math.random()}`;
}

export function CaseDialog({
  agentId,
  scenarios,
  caseItem,
  open,
  onOpenChange,
  supportsAttachments = true,
  attachmentAcceptExt = [],
  mode = 'conversation',
  workflowDraft,
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
  const [taskVariableRows, setTaskVariableRows] = React.useState<TaskVariableRow[]>([]);
  const [checkConditions, setCheckConditions] = React.useState<ExpectedCheckCondition[]>([]);
  const [conversationCheckConditions, setConversationCheckConditions] = React.useState<ConversationCheckCondition[]>([]);
  const [advancedChecksOpen, setAdvancedChecksOpen] = React.useState(false);
  const workflowCheckOptions = React.useMemo(
    () => buildWorkflowCheckOptions(workflowDraft),
    [workflowDraft]
  );
  const workflowInputVariableOptions = React.useMemo(
    () => buildWorkflowInputVariableOptions(workflowDraft),
    [workflowDraft]
  );
  const conversationLabels = React.useMemo(
    () => ({
      conditionTitle: (index: number) => t('conversationCheckTitle', { index }),
      typeLabel: t('conversationCheckTypeLabel'),
      ruleLabel: t('conversationCheckRuleLabel'),
      severityLabel: t('checkSeverityLabel'),
      valuesLabel: t('conversationCheckValuesLabel'),
      matchModeLabel: t('matchModeLabel'),
      remove: t('removeCheckCondition'),
      lineListPlaceholder: t('lineListPlaceholder'),
      matchSemantic: t('matchSemantic'),
      matchKeyword: t('matchKeyword'),
      severityCritical: t('severityCritical'),
      severityNormal: t('severityNormal'),
      severityHint: t('severityHint'),
      operatorPassed: t('conversationOperatorPassed'),
      operatorContains: t('operatorContains'),
      operatorNotContains: t('operatorNotContains'),
      typeLabels: {
        intent_understanding: t('conversationCheckTypeIntent'),
        context_following: t('conversationCheckTypeContext'),
        memory: t('conversationCheckTypeMemory'),
        clarification: t('conversationCheckTypeClarification'),
        output_format: t('conversationCheckTypeOutputFormat'),
        fallback: t('conversationCheckTypeFallback'),
        reply_contains: t('conversationCheckTypeReplyContains'),
        safety: t('conversationCheckTypeSafety'),
        task_completion: t('conversationCheckTypeTaskCompletion'),
        consistency: t('conversationCheckTypeConsistency'),
        no_hallucination: t('conversationCheckTypeNoHallucination'),
        no_system_leak: t('conversationCheckTypeNoSystemLeak'),
        tone: t('conversationCheckTypeTone'),
      } satisfies Record<ConversationCheckType, string>,
    }),
    [t]
  );

  React.useEffect(() => {
    if (open && caseItem) {
      const existingTurns = caseItem.turns?.length
        ? caseItem.turns.map(turn => createTurn(turn.content, turn.attachments ?? [], turnInputs(turn)))
        : [createTurn(caseItem.content)];
      setTurns(existingTurns.length ? existingTurns : [createTurn(caseItem.content)]);
      setScenarioId(caseItem.scenario_id || '');
      setExpectedResult(caseItem.expected_result || '');
      setQuestionType(caseItem.question_type || 'core');
      setStatus(caseItem.status || 'enabled');
      const firstInputs = turnInputs(caseItem.turns?.[0]);
      setTaskVariableRows(formatTaskVariableRows(firstInputs));
      const checks = normalizeExpectedChecks(firstInputs[EXPECTED_CHECKS_KEY]);
      setCheckConditions(checks.conditions ?? []);
      setConversationCheckConditions(normalizeConversationChecks(firstInputs[CONVERSATION_CHECKS_KEY]).conditions ?? []);
      setAdvancedChecksOpen(false);
      setFileSelectorTurnId(null);
      return;
    }
    if (!open || !caseItem) {
      setTurns([createTurn()]);
      setScenarioId(scenarios[0]?.id ?? '');
      setExpectedResult('');
      setQuestionType('core');
      setStatus('enabled');
      setTaskVariableRows([]);
      setCheckConditions([]);
      setConversationCheckConditions([]);
      setAdvancedChecksOpen(false);
      setFileSelectorTurnId(null);
    }
  }, [caseItem, open, scenarios]);

  const isPending = createCase.isPending || updateCase.isPending;
  const normalizedTurns = React.useMemo(
    () =>
      turns
        .map((turn, index) => ({
          role: 'user',
          content: turn.content.trim(),
          attachments: supportsAttachments ? turn.attachments : [],
          inputs: buildTurnInputs(turn, index, mode, {
            taskVariableRows,
            checkConditions,
            conversationCheckConditions,
            workflowCheckOptions,
          }),
        }))
        .filter(turn => turn.content || turn.attachments.length > 0),
    [checkConditions, conversationCheckConditions, mode, supportsAttachments, taskVariableRows, turns, workflowCheckOptions]
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

  const addCheckCondition = (type: ExpectedCheckType = 'output_contains') => {
    setCheckConditions(prev => [...prev, createExpectedCheckCondition(type)]);
  };

  const updateCheckCondition = (conditionId: string, patch: Partial<ExpectedCheckCondition>) => {
    setCheckConditions(prev =>
      prev.map(condition => (condition.id === conditionId ? { ...condition, ...patch } : condition))
    );
  };

  const changeCheckConditionType = (conditionId: string, type: ExpectedCheckType) => {
    setCheckConditions(prev =>
      prev.map(condition =>
        condition.id === conditionId ? { ...createExpectedCheckCondition(type), id: condition.id } : condition
      )
    );
  };

  const removeCheckCondition = (conditionId: string) => {
    setCheckConditions(prev => prev.filter(condition => condition.id !== conditionId));
  };

  const addTurnCheckCondition = (turnId: string) => {
    updateTurn(turnId, {
      checks: [...(turns.find(turn => turn.id === turnId)?.checks ?? []), createConversationCheckCondition()],
    });
  };

  const updateTurnCheckCondition = (
    turnId: string,
    conditionId: string,
    patch: Partial<ConversationCheckCondition>
  ) => {
    updateTurn(turnId, {
      checks: (turns.find(turn => turn.id === turnId)?.checks ?? []).map(condition =>
        condition.id === conditionId ? { ...condition, ...patch } : condition
      ),
    });
  };

  const changeTurnCheckConditionType = (
    turnId: string,
    conditionId: string,
    type: ConversationCheckType
  ) => {
    updateTurn(turnId, {
      checks: (turns.find(turn => turn.id === turnId)?.checks ?? []).map(condition =>
        condition.id === conditionId
          ? { ...createConversationCheckCondition(type), id: condition.id }
          : condition
      ),
    });
  };

  const removeTurnCheckCondition = (turnId: string, conditionId: string) => {
    updateTurn(turnId, {
      checks: (turns.find(turn => turn.id === turnId)?.checks ?? []).filter(
        condition => condition.id !== conditionId
      ),
    });
  };

  const addConversationCheckCondition = () => {
    setConversationCheckConditions(prev => [...prev, createConversationCheckCondition('consistency')]);
  };

  const updateConversationCheckCondition = (
    conditionId: string,
    patch: Partial<ConversationCheckCondition>
  ) => {
    setConversationCheckConditions(prev =>
      prev.map(condition => (condition.id === conditionId ? { ...condition, ...patch } : condition))
    );
  };

  const changeConversationCheckConditionType = (
    conditionId: string,
    type: ConversationCheckType
  ) => {
    setConversationCheckConditions(prev =>
      prev.map(condition =>
        condition.id === conditionId ? { ...createConversationCheckCondition(type), id: condition.id } : condition
      )
    );
  };

  const removeConversationCheckCondition = (conditionId: string) => {
    setConversationCheckConditions(prev => prev.filter(condition => condition.id !== conditionId));
  };

  const addTaskVariableRow = () => {
    setTaskVariableRows(prev => [
      ...prev,
      {
        id: createLocalId('task_var'),
        key: firstAvailableVariableKey(workflowInputVariableOptions, prev),
        value: '',
      },
    ]);
  };

  const updateTaskVariableRow = (rowId: string, patch: Partial<TaskVariableRow>) => {
    setTaskVariableRows(prev => prev.map(row => (row.id === rowId ? { ...row, ...patch } : row)));
  };

  const removeTaskVariableRow = (rowId: string) => {
    setTaskVariableRows(prev => prev.filter(row => row.id !== rowId));
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="lg" onInteractOutside={event => event.preventDefault()}>
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
                <Label>{mode === 'task' ? t('taskInputLabel') : t('turnsLabel')}</Label>
                <p className="mt-1 text-sm text-slate-500">
                  {mode === 'task' ? t('taskInputDescription') : t('turnsDescription')}
                </p>
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
                      placeholder={mode === 'task' ? t('taskContentPlaceholder') : t('contentPlaceholder')}
                      className="min-h-24 resize-none bg-white"
                    />
                    {mode === 'conversation' ? (
                      <>
                        <div className="mt-3 space-y-2">
                          <Label>{t('turnExpectationLabel')}</Label>
                          <Textarea
                            value={turn.expectation}
                            onChange={event => updateTurn(turn.id, { expectation: event.target.value })}
                            placeholder={t('turnExpectationPlaceholder')}
                            className="min-h-16 resize-none bg-white"
                          />
                        </div>
                      </>
                    ) : null}
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
                    <GeneratedFixtureSummary
                      fixtures={fixtureSpecsFromInputs(turn.inputs)}
                      labels={{
                        title: t('generatedFixtureTitle'),
                        facts: t('generatedFixtureFacts'),
                        checks: t('generatedFixtureChecks'),
                      }}
                    />
                  </div>
                ))}
              </div>
              <Button
                type="button"
                variant="outline"
                className={mode === 'task' ? 'hidden' : undefined}
                onClick={() => setTurns(prev => [...prev, createTurn()])}
              >
                <Plus className="mr-2 size-4" />
                {t('addTurn')}
              </Button>
            </div>

            {mode === 'conversation' ? (
              <div className="space-y-3">
                <Button
                  type="button"
                  variant="outline"
                  className="w-full justify-between bg-white"
                  onClick={() => setAdvancedChecksOpen(prev => !prev)}
                >
                  <span className="flex items-center gap-2">
                    <SlidersHorizontal className="size-4" />
                    {t('conversationAdvancedChecksLabel')}
                  </span>
                  <ChevronDown
                    className={`size-4 transition-transform ${advancedChecksOpen ? 'rotate-180' : ''}`}
                  />
                </Button>
                {advancedChecksOpen ? (
                  <div className="space-y-4 rounded-xl border border-slate-200 bg-slate-50 p-4">
                    <p className="text-sm text-slate-500">{t('conversationAdvancedChecksDescription')}</p>
                    {turns.map((turn, index) => (
                      <ConversationChecksEditor
                        key={turn.id}
                        title={`${t('turnTitle', { index: index + 1 })} - ${t('turnChecksLabel')}`}
                        description={t('turnChecksDescription')}
                        conditions={turn.checks}
                        labels={conversationLabels}
                        emptyText={t('emptyTurnChecks')}
                        addText={t('addTurnCheck')}
                        onAdd={() => addTurnCheckCondition(turn.id)}
                        onChange={(conditionId, patch) =>
                          updateTurnCheckCondition(turn.id, conditionId, patch)
                        }
                        onTypeChange={(conditionId, type) =>
                          changeTurnCheckConditionType(turn.id, conditionId, type)
                        }
                        onRemove={conditionId => removeTurnCheckCondition(turn.id, conditionId)}
                      />
                    ))}
                    <ConversationChecksEditor
                      title={t('conversationChecksLabel')}
                      description={t('conversationChecksDescription')}
                      conditions={conversationCheckConditions}
                      labels={conversationLabels}
                      emptyText={t('emptyConversationChecks')}
                      addText={t('addConversationCheck')}
                      onAdd={addConversationCheckCondition}
                      onChange={updateConversationCheckCondition}
                      onTypeChange={changeConversationCheckConditionType}
                      onRemove={removeConversationCheckCondition}
                    />
                  </div>
                ) : null}
              </div>
            ) : null}

            {mode === 'task' ? (
              <div className="space-y-3">
                <Button
                  type="button"
                  variant="outline"
                  className="w-full justify-between bg-white"
                  onClick={() => setAdvancedChecksOpen(prev => !prev)}
                >
                  <span className="flex items-center gap-2">
                    <SlidersHorizontal className="size-4" />
                    {t('advancedChecksLabel')}
                  </span>
                  <ChevronDown
                    className={`size-4 transition-transform ${advancedChecksOpen ? 'rotate-180' : ''}`}
                  />
                </Button>
                {advancedChecksOpen ? (
                  <div className="space-y-4 rounded-xl border border-slate-200 bg-slate-50 p-4">
                    <p className="text-sm text-slate-500">{t('advancedChecksDescription')}</p>
                    <div>
                      <Label>{t('taskVariablesLabel')}</Label>
                      <p className="mt-1 text-sm text-slate-500">{t('taskVariablesDescription')}</p>
                    </div>
                    <TaskVariablesEditor
                      rows={taskVariableRows}
                      options={workflowInputVariableOptions}
                      labels={{
                        variableLabel: t('taskVariableNameLabel'),
                        valueLabel: t('taskVariableValueLabel'),
                        add: t('addTaskVariable'),
                        remove: t('removeTaskVariable'),
                        empty: t('emptyTaskVariables'),
                        noOptions: t('noTaskVariableOptions'),
                        customPlaceholder: t('taskVariableCustomPlaceholder'),
                        valuePlaceholder: t('taskVariableValuePlaceholder'),
                      }}
                      onAdd={addTaskVariableRow}
                      onChange={updateTaskVariableRow}
                      onRemove={removeTaskVariableRow}
                    />
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <Label>{t('taskChecksLabel')}</Label>
                        <p className="mt-1 text-sm text-slate-500">{t('taskChecksDescription')}</p>
                      </div>
                      <Button type="button" variant="outline" size="sm" onClick={() => addCheckCondition()}>
                        <Plus className="mr-1 size-4" />
                        {t('addCheckCondition')}
                      </Button>
                    </div>
                    {checkConditions.length === 0 ? (
                      <div className="rounded-lg border border-dashed border-slate-200 bg-white px-4 py-5 text-center text-sm text-slate-500">
                        {t('emptyCheckConditions')}
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {checkConditions.map((condition, index) => (
                          <CheckConditionEditor
                            key={condition.id}
                            condition={condition}
                            index={index}
                            nodeOptions={workflowCheckOptions.nodeOptions}
                            capabilityOptions={workflowCheckOptions.capabilityOptions}
                            labels={{
                              conditionTitle: t('checkConditionTitle', { index: index + 1 }),
                              typeLabel: t('checkTypeLabel'),
                              severityLabel: t('checkSeverityLabel'),
                              ruleLabel: t('checkRuleLabel'),
                              targetNodeLabel: t('targetNodeLabel'),
                              targetCapabilityLabel: t('targetCapabilityLabel'),
                              inputValuesLabel: t('inputValuesLabel'),
                              outputValuesLabel: t('outputValuesLabel'),
                              matchModeLabel: t('matchModeLabel'),
                              latencyValueLabel: t('latencyValueLabel'),
                              noNodeOptions: t('noNodeOptions'),
                              noCapabilityOptions: t('noCapabilityOptions'),
                              lineListPlaceholder: t('lineListPlaceholder'),
                              maxLatencyPlaceholder: t('maxLatencyPlaceholder'),
                              remove: t('removeCheckCondition'),
                              typeNode: t('checkTypeNode'),
                              typeCapability: t('checkTypeCapability'),
                              typeOutput: t('checkTypeOutput'),
                              typeLatency: t('checkTypeLatency'),
                              operatorVisited: t('operatorVisited'),
                              operatorNotVisited: t('operatorNotVisited'),
                              operatorInputContains: t('operatorInputContains'),
                              operatorInputNotContains: t('operatorInputNotContains'),
                              operatorOutputContains: t('operatorOutputContains'),
                              operatorOutputNotContains: t('operatorOutputNotContains'),
                              operatorCalled: t('operatorCalled'),
                              operatorNotCalled: t('operatorNotCalled'),
                              operatorContains: t('operatorContains'),
                              operatorNotContains: t('operatorNotContains'),
                              operatorLatencyLte: t('operatorLatencyLte'),
                              matchSemantic: t('matchSemantic'),
                              matchKeyword: t('matchKeyword'),
                              severityCritical: t('severityCritical'),
                              severityNormal: t('severityNormal'),
                              severityHint: t('severityHint'),
                              sourceAiGenerated: t('sourceAiGenerated'),
                              sourceUserAdded: t('sourceUserAdded'),
                              sourceSystemDefault: t('sourceSystemDefault'),
                            }}
                            onTypeChange={type => changeCheckConditionType(condition.id, type)}
                            onChange={patch => updateCheckCondition(condition.id, patch)}
                            onRemove={() => removeCheckCondition(condition.id)}
                          />
                        ))}
                      </div>
                    )}
                  </div>
                ) : null}
              </div>
            ) : null}

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
                if (mode === 'task') {
                  const checks = buildExpectedChecksPayload(checkConditions, workflowCheckOptions);
                  if ((checks.conditions?.length ?? 0) !== checkConditions.length) {
                    toast.error(t('invalidCheckConditions'));
                    setAdvancedChecksOpen(true);
                    return;
                  }
                }
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
        acceptExt={attachmentAcceptExt}
      />
    </>
  );
}

interface CheckConditionEditorLabels {
  conditionTitle: string;
  typeLabel: string;
  severityLabel: string;
  ruleLabel: string;
  targetNodeLabel: string;
  targetCapabilityLabel: string;
  inputValuesLabel: string;
  outputValuesLabel: string;
  matchModeLabel: string;
  latencyValueLabel: string;
  noNodeOptions: string;
  noCapabilityOptions: string;
  lineListPlaceholder: string;
  maxLatencyPlaceholder: string;
  remove: string;
  typeNode: string;
  typeCapability: string;
  typeOutput: string;
  typeLatency: string;
  operatorVisited: string;
  operatorNotVisited: string;
  operatorInputContains: string;
  operatorInputNotContains: string;
  operatorOutputContains: string;
  operatorOutputNotContains: string;
  operatorCalled: string;
  operatorNotCalled: string;
  operatorContains: string;
  operatorNotContains: string;
  operatorLatencyLte: string;
  matchSemantic: string;
  matchKeyword: string;
  severityCritical: string;
  severityNormal: string;
  severityHint: string;
  sourceAiGenerated: string;
  sourceUserAdded: string;
  sourceSystemDefault: string;
}

interface ConversationChecksEditorLabels {
  conditionTitle: (index: number) => string;
  typeLabel: string;
  ruleLabel: string;
  severityLabel: string;
  valuesLabel: string;
  matchModeLabel: string;
  remove: string;
  lineListPlaceholder: string;
  matchSemantic: string;
  matchKeyword: string;
  severityCritical: string;
  severityNormal: string;
  severityHint: string;
  operatorPassed: string;
  operatorContains: string;
  operatorNotContains: string;
  typeLabels: Record<ConversationCheckType, string>;
}

function GeneratedFixtureSummary({
  fixtures,
  labels,
}: {
  fixtures: ReturnType<typeof fixtureSpecsFromInputs>;
  labels: { title: string; facts: string; checks: string };
}) {
  if (fixtures.length === 0) {
    return null;
  }
  return (
    <div className="mt-3 space-y-2 rounded-lg border border-blue-100 bg-blue-50 p-3">
      <div className="text-sm font-medium text-blue-900">{labels.title}</div>
      <div className="space-y-2">
        {fixtures.map((fixture, index) => (
          <div key={`${fixture.name || fixture.title || 'fixture'}-${index}`} className="rounded-md bg-white p-3 text-sm">
            <div className="font-medium text-slate-900">
              {[fixture.name || fixture.title, fixture.format].filter(Boolean).join(' · ')}
            </div>
            {fixture.facts.length > 0 ? (
              <div className="mt-2 text-slate-600">
                <span className="font-medium text-slate-700">{labels.facts}</span>
                <span className="ml-1">{fixture.facts.slice(0, 4).join('；')}</span>
              </div>
            ) : null}
            {fixture.expected_checks.length > 0 ? (
              <div className="mt-1 text-slate-600">
                <span className="font-medium text-slate-700">{labels.checks}</span>
                <span className="ml-1">{fixture.expected_checks.slice(0, 4).join('；')}</span>
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}

function ConversationChecksEditor({
  title,
  description,
  conditions,
  labels,
  emptyText,
  addText,
  className,
  onAdd,
  onChange,
  onTypeChange,
  onRemove,
}: {
  title: string;
  description: string;
  conditions: ConversationCheckCondition[];
  labels: ConversationChecksEditorLabels;
  emptyText: string;
  addText: string;
  className?: string;
  onAdd: () => void;
  onChange: (conditionId: string, patch: Partial<ConversationCheckCondition>) => void;
  onTypeChange: (conditionId: string, type: ConversationCheckType) => void;
  onRemove: (conditionId: string) => void;
}) {
  return (
    <div className={cn('space-y-3 rounded-lg border border-slate-200 bg-white p-4', className)}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <Label>{title}</Label>
          <p className="mt-1 text-sm text-slate-500">{description}</p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onAdd}>
          <Plus className="mr-1 size-4" />
          {addText}
        </Button>
      </div>
      {conditions.length === 0 ? (
        <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-4 py-4 text-center text-sm text-slate-500">
          {emptyText}
        </div>
      ) : (
        <div className="space-y-3">
          {conditions.map((condition, index) => (
            <ConversationCheckConditionEditor
              key={condition.id}
              condition={condition}
              index={index}
              labels={labels}
              onChange={patch => onChange(condition.id, patch)}
              onTypeChange={type => onTypeChange(condition.id, type)}
              onRemove={() => onRemove(condition.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function ConversationCheckConditionEditor({
  condition,
  index,
  labels,
  onChange,
  onTypeChange,
  onRemove,
}: {
  condition: ConversationCheckCondition;
  index: number;
  labels: ConversationChecksEditorLabels;
  onChange: (patch: Partial<ConversationCheckCondition>) => void;
  onTypeChange: (type: ConversationCheckType) => void;
  onRemove: () => void;
}) {
  const severityOptions = [
    { value: 'critical', label: labels.severityCritical, className: 'border-red-200 bg-red-50 text-red-700' },
    { value: 'normal', label: labels.severityNormal, className: 'border-blue-200 bg-blue-50 text-blue-700' },
    { value: 'hint', label: labels.severityHint, className: 'border-slate-200 bg-slate-100 text-slate-600' },
  ];
  const selectedSeverity = severityOptions.find(item => item.value === condition.severity) ?? severityOptions[1];

  return (
    <div className="space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <div className="font-medium text-slate-950">{labels.conditionTitle(index + 1)}</div>
          <Badge variant="outline" className={selectedSeverity.className}>
            {selectedSeverity.label}
          </Badge>
        </div>
        <Button type="button" variant="ghost" size="sm" onClick={onRemove}>
          <Trash2 className="mr-1 size-4" />
          {labels.remove}
        </Button>
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        <div className="space-y-2">
          <Label>{labels.typeLabel}</Label>
          <Select value={condition.type} onValueChange={value => onTypeChange(value as ConversationCheckType)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {conversationCheckTypes().map(type => (
                <SelectItem key={type} value={type}>
                  {labels.typeLabels[type]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{labels.severityLabel}</Label>
          <Select
            value={condition.severity ?? 'normal'}
            onValueChange={value =>
              onChange({ severity: value as ConversationCheckCondition['severity'] })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {severityOptions.map(item => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{labels.ruleLabel}</Label>
          <Select value={condition.operator || 'passed'} onValueChange={operator => onChange({ operator })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="passed">{labels.operatorPassed}</SelectItem>
              <SelectItem value="contains">{labels.operatorContains}</SelectItem>
              <SelectItem value="not_contains">{labels.operatorNotContains}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_180px]">
        <div className="space-y-2">
          <Label>{labels.valuesLabel}</Label>
          <Textarea
            value={serializeLineList(condition.values)}
            onChange={event => onChange({ values: parseLineList(event.target.value) })}
            placeholder={labels.lineListPlaceholder}
            className="min-h-20 resize-none bg-white"
          />
        </div>
        <div className="space-y-2">
          <Label>{labels.matchModeLabel}</Label>
          <Select
            value={condition.match_mode ?? 'semantic'}
            onValueChange={value =>
              onChange({ match_mode: value as ConversationCheckCondition['match_mode'] })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="semantic">{labels.matchSemantic}</SelectItem>
              <SelectItem value="keyword">{labels.matchKeyword}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
    </div>
  );
}

function TaskVariablesEditor({
  rows,
  options,
  labels,
  onAdd,
  onChange,
  onRemove,
}: {
  rows: TaskVariableRow[];
  options: WorkflowInputVariableOption[];
  labels: {
    variableLabel: string;
    valueLabel: string;
    add: string;
    remove: string;
    empty: string;
    noOptions: string;
    customPlaceholder: string;
    valuePlaceholder: string;
  };
  onAdd: () => void;
  onChange: (rowId: string, patch: Partial<TaskVariableRow>) => void;
  onRemove: (rowId: string) => void;
}) {
  if (options.length === 0 && rows.length === 0) {
    return (
      <div className="space-y-3 rounded-lg border border-slate-200 bg-white p-3">
        <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-4 py-4 text-center text-sm text-slate-500">
          {labels.noOptions}
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onAdd}>
          <Plus className="mr-1 size-4" />
          {labels.add}
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-3 rounded-lg border border-slate-200 bg-white p-3">
      {rows.length === 0 ? (
        <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-4 py-4 text-center text-sm text-slate-500">
          {labels.empty}
        </div>
      ) : (
        <div className="space-y-2">
          {rows.map(row => (
            <div key={row.id} className="grid grid-cols-1 gap-2 md:grid-cols-[220px_1fr_auto]">
              <div className="space-y-1">
                <Label>{labels.variableLabel}</Label>
                {options.length > 0 ? (
                  <Select value={row.key} onValueChange={key => onChange(row.id, { key })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {mergeVariableOptions(options, row.key).map(option => (
                      <SelectItem key={option.key} value={option.key}>
                        {option.label} · {option.type}
                      </SelectItem>
                    ))}
                  </SelectContent>
                  </Select>
                ) : (
                  <Input
                    value={row.key}
                    onChange={event => onChange(row.id, { key: event.target.value })}
                    placeholder={labels.customPlaceholder}
                  />
                )}
              </div>
              <div className="space-y-1">
                <Label>{labels.valueLabel}</Label>
                <Input
                  value={row.value}
                  onChange={event => onChange(row.id, { value: event.target.value })}
                  placeholder={labels.valuePlaceholder}
                />
              </div>
              <div className="flex items-end">
                <Button type="button" variant="ghost" size="sm" onClick={() => onRemove(row.id)}>
                  <Trash2 className="mr-1 size-4" />
                  {labels.remove}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      <Button type="button" variant="outline" size="sm" onClick={onAdd}>
        <Plus className="mr-1 size-4" />
        {labels.add}
      </Button>
    </div>
  );
}

function mergeVariableOptions(options: WorkflowInputVariableOption[], currentKey: string) {
  const key = currentKey.trim();
  if (!key || options.some(option => option.key === key)) {
    return options;
  }
  return [
    {
      key,
      label: key,
      type: 'saved',
      stale: true,
    },
    ...options,
  ];
}

function CheckConditionEditor({
  condition,
  nodeOptions,
  capabilityOptions,
  labels,
  onTypeChange,
  onChange,
  onRemove,
}: {
  condition: ExpectedCheckCondition;
  index: number;
  nodeOptions: WorkflowCheckOption[];
  capabilityOptions: WorkflowCheckOption[];
  labels: CheckConditionEditorLabels;
  onTypeChange: (type: ExpectedCheckType) => void;
  onChange: (patch: Partial<ExpectedCheckCondition>) => void;
  onRemove: () => void;
}) {
  const typeOptions: Array<{ value: ExpectedCheckType; label: string }> = [
    { value: 'node', label: labels.typeNode },
    { value: 'capability', label: labels.typeCapability },
    { value: 'output_contains', label: labels.typeOutput },
    { value: 'latency', label: labels.typeLatency },
  ];
  const severityOptions = [
    { value: 'critical', label: labels.severityCritical, className: 'border-red-200 bg-red-50 text-red-700' },
    { value: 'normal', label: labels.severityNormal, className: 'border-blue-200 bg-blue-50 text-blue-700' },
    { value: 'hint', label: labels.severityHint, className: 'border-slate-200 bg-slate-100 text-slate-600' },
  ];

  const nodeSelectOptions = mergeCheckOptions(nodeOptions, condition);
  const capabilitySelectOptions = mergeCheckOptions(capabilityOptions, condition);
  const selectedNodeOption = nodeSelectOptions.find(option => option.id === condition.target_id);
  const selectTarget = (option: WorkflowCheckOption | undefined) => {
    if (!option) return;
    onChange({
      target_id: option.id,
      target_label: option.label,
      target_type: option.type,
      operator:
        condition.type === 'node' && !nodeRuleOptions(option.type).some(item => item.value === condition.operator)
          ? defaultNodeOperator(option.type)
          : condition.operator,
    });
  };

  const selectedSeverity = severityOptions.find(item => item.value === condition.severity) ?? severityOptions[1];
  const showNodeValueAssertion = condition.type === 'node' && isNodeValueOperator(condition.operator);
  const sourceLabel = sourceLabelForCondition(condition.source, labels);
  const valueLabel =
    condition.type === 'node' && condition.operator?.startsWith('input_')
      ? labels.inputValuesLabel
      : labels.outputValuesLabel;

  return (
    <div className="space-y-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <div className="font-medium text-slate-950">{labels.conditionTitle}</div>
          <Badge variant="outline" className={selectedSeverity.className}>
            {selectedSeverity.label}
          </Badge>
          {sourceLabel ? (
            <Badge variant="outline" className="border-slate-200 bg-slate-50 text-slate-600">
              {sourceLabel}
            </Badge>
          ) : null}
        </div>
        <Button type="button" variant="ghost" size="sm" onClick={onRemove}>
          <Trash2 className="mr-1 size-4" />
          {labels.remove}
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        <div className="space-y-2">
          <Label>{labels.typeLabel}</Label>
          <Select value={condition.type} onValueChange={value => onTypeChange(value as ExpectedCheckType)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {typeOptions.map(item => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{labels.severityLabel}</Label>
          <Select
            value={condition.severity ?? 'normal'}
            onValueChange={value =>
              onChange({ severity: value as ExpectedCheckCondition['severity'] })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {severityOptions.map(item => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{labels.ruleLabel}</Label>
          <RuleSelect
            condition={condition}
            labels={labels}
            selectedNodeType={selectedNodeOption?.type ?? condition.target_type}
            onChange={onChange}
          />
        </div>
      </div>

      {condition.type === 'node' ? (
        <div className="space-y-2">
          <Label>{labels.targetNodeLabel}</Label>
          {nodeSelectOptions.length > 0 ? (
            <Select
              value={condition.target_id}
              onValueChange={value => selectTarget(nodeSelectOptions.find(option => option.id === value))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {nodeSelectOptions.map(option => (
                  <SelectItem key={option.id} value={option.id}>
                    {option.label} · {option.type}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-500">
              {labels.noNodeOptions}
            </div>
          )}
        </div>
      ) : null}

      {showNodeValueAssertion ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_180px]">
          <div className="space-y-2">
            <Label>{valueLabel}</Label>
            <Textarea
              value={serializeLineList(condition.values)}
              onChange={event => onChange({ values: parseLineList(event.target.value) })}
              placeholder={labels.lineListPlaceholder}
              className="min-h-20 resize-none bg-white"
            />
          </div>
          <div className="space-y-2">
            <Label>{labels.matchModeLabel}</Label>
            <Select
              value={condition.match_mode ?? 'semantic'}
              onValueChange={value =>
                onChange({ match_mode: value as ExpectedCheckCondition['match_mode'] })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="semantic">{labels.matchSemantic}</SelectItem>
                <SelectItem value="keyword">{labels.matchKeyword}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      ) : null}

      {condition.type === 'capability' ? (
        <div className="space-y-2">
          <Label>{labels.targetCapabilityLabel}</Label>
          {capabilitySelectOptions.length > 0 ? (
            <Select
              value={condition.target_id}
              onValueChange={value => selectTarget(capabilitySelectOptions.find(option => option.id === value))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {capabilitySelectOptions.map(option => (
                  <SelectItem key={option.id} value={option.id}>
                    {option.label} · {option.type}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-500">
              {labels.noCapabilityOptions}
            </div>
          )}
        </div>
      ) : null}

      {condition.type === 'output_contains' ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_180px]">
          <div className="space-y-2">
            <Label>{labels.outputValuesLabel}</Label>
            <Textarea
              value={serializeLineList(condition.values)}
              onChange={event => onChange({ values: parseLineList(event.target.value) })}
              placeholder={labels.lineListPlaceholder}
              className="min-h-20 resize-none bg-white"
            />
          </div>
          <div className="space-y-2">
            <Label>{labels.matchModeLabel}</Label>
            <Select
              value={condition.match_mode ?? 'semantic'}
              onValueChange={value =>
                onChange({ match_mode: value as ExpectedCheckCondition['match_mode'] })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="semantic">{labels.matchSemantic}</SelectItem>
                <SelectItem value="keyword">{labels.matchKeyword}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      ) : null}

      {condition.type === 'latency' ? (
        <div className="max-w-xs space-y-2">
          <Label>{labels.latencyValueLabel}</Label>
          <Input
            value={condition.value_ms === undefined ? '' : String(condition.value_ms)}
            onChange={event => onChange({ value_ms: event.target.value })}
            placeholder={labels.maxLatencyPlaceholder}
          />
        </div>
      ) : null}
    </div>
  );
}

function RuleSelect({
  condition,
  labels,
  selectedNodeType,
  onChange,
}: {
  condition: ExpectedCheckCondition;
  labels: CheckConditionEditorLabels;
  selectedNodeType?: string;
  onChange: (patch: Partial<ExpectedCheckCondition>) => void;
}) {
  if (condition.type === 'node') {
    const rules = nodeRuleOptions(selectedNodeType).map(rule => ({
      ...rule,
      label: nodeRuleLabel(rule.value, labels),
    }));
    const value = rules.some(rule => rule.value === condition.operator)
      ? condition.operator
      : defaultNodeOperator(selectedNodeType);
    return (
      <Select value={value} onValueChange={operator => onChange({ operator })}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {rules.map(rule => (
            <SelectItem key={rule.value} value={rule.value}>
              {rule.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  if (condition.type === 'capability') {
    return (
      <Select value={condition.operator || 'called'} onValueChange={operator => onChange({ operator })}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="called">{labels.operatorCalled}</SelectItem>
          <SelectItem value="not_called">{labels.operatorNotCalled}</SelectItem>
        </SelectContent>
      </Select>
    );
  }
  if (condition.type === 'latency') {
    return (
      <Select value="lte" onValueChange={operator => onChange({ operator })}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="lte">{labels.operatorLatencyLte}</SelectItem>
        </SelectContent>
      </Select>
    );
  }
  return (
    <Select value={condition.operator || 'contains'} onValueChange={operator => onChange({ operator })}>
      <SelectTrigger>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="contains">{labels.operatorContains}</SelectItem>
        <SelectItem value="not_contains">{labels.operatorNotContains}</SelectItem>
      </SelectContent>
    </Select>
  );
}

function isNodeValueOperator(operator: string | undefined) {
  return (
    operator === 'input_contains' ||
    operator === 'input_not_contains' ||
    operator === 'output_contains' ||
    operator === 'output_not_contains'
  );
}

function defaultNodeOperator(nodeType: string | undefined) {
  const normalized = normalizeNodeTypeForUi(nodeType);
  if (normalized === 'start') return 'input_contains';
  if (normalized === 'end' || normalized === 'answer') return 'output_contains';
  return 'visited';
}

function nodeRuleOptions(nodeType: string | undefined) {
  const normalized = normalizeNodeTypeForUi(nodeType);
  if (normalized === 'start') {
    return [{ value: 'input_contains' }, { value: 'input_not_contains' }];
  }
  if (normalized === 'end' || normalized === 'answer') {
    return [{ value: 'visited' }, { value: 'output_contains' }, { value: 'output_not_contains' }];
  }
  return [
    { value: 'visited' },
    { value: 'not_visited' },
    { value: 'input_contains' },
    { value: 'input_not_contains' },
    { value: 'output_contains' },
    { value: 'output_not_contains' },
  ];
}

function nodeRuleLabel(value: string, labels: CheckConditionEditorLabels) {
  const map: Record<string, string> = {
    visited: labels.operatorVisited,
    not_visited: labels.operatorNotVisited,
    input_contains: labels.operatorInputContains,
    input_not_contains: labels.operatorInputNotContains,
    output_contains: labels.operatorOutputContains,
    output_not_contains: labels.operatorOutputNotContains,
  };
  return map[value] || value;
}

function sourceLabelForCondition(
  source: ExpectedCheckCondition['source'],
  labels: CheckConditionEditorLabels
) {
  if (source === 'ai_generated') return labels.sourceAiGenerated;
  if (source === 'system_default') return labels.sourceSystemDefault;
  if (source === 'user_added') return labels.sourceUserAdded;
  return '';
}

function mergeCheckOptions(options: WorkflowCheckOption[], condition: ExpectedCheckCondition) {
  const targetId = condition.target_id?.trim();
  if (!targetId || options.some(option => option.id === targetId)) {
    return options;
  }
  return [
    {
      id: targetId,
      label: condition.target_label || targetId,
      type: condition.target_type || 'saved',
    },
    ...options,
  ];
}

function normalizeNodeTypeForUi(type: string | undefined) {
  return (type || '').trim().toLowerCase();
}

function formatTaskVariableRows(inputs: Record<string, unknown>): TaskVariableRow[] {
  return Object.entries(inputs)
    .filter(([key]) => !key.startsWith('__'))
    .map(([key, value]) => ({
      id: createLocalId('task_var'),
      key,
      value: typeof value === 'string' ? value : JSON.stringify(value),
    }));
}

function buildTaskInputs(rows: TaskVariableRow[]): Record<string, unknown> {
  return Object.fromEntries(
    rows
      .map(row => [row.key.trim(), row.value.trim()] as const)
      .filter(([key, value]) => key && value && !key.startsWith('__'))
  );
}

function firstAvailableVariableKey(options: WorkflowInputVariableOption[], rows: TaskVariableRow[]) {
  const usedKeys = new Set(rows.map(row => row.key).filter(Boolean));
  const knownKey = options.find(option => !usedKeys.has(option.key))?.key || options[0]?.key;
  if (knownKey) {
    return knownKey;
  }
  let index = rows.length + 1;
  let key = `custom_variable_${index}`;
  while (usedKeys.has(key)) {
    index += 1;
    key = `custom_variable_${index}`;
  }
  return key;
}

function buildTurnInputs(
  turn: EditableTurn,
  index: number,
  mode: WorkflowTestMode,
  taskState: {
    taskVariableRows: TaskVariableRow[];
    checkConditions: ExpectedCheckCondition[];
    conversationCheckConditions: ConversationCheckCondition[];
    workflowCheckOptions: { nodeOptions: WorkflowCheckOption[]; capabilityOptions: WorkflowCheckOption[] };
  }
): Record<string, unknown> {
  if (mode === 'task') {
    const checks = buildExpectedChecksPayload(taskState.checkConditions, taskState.workflowCheckOptions);
    return {
      ...buildTaskInputs(taskState.taskVariableRows),
      [CASE_MODE_KEY]: 'task',
      [EXPECTED_CHECKS_KEY]: checks,
    };
  }

  const inputs = Object.fromEntries(
    Object.entries(turn.inputs).filter(
      ([key]) =>
        key !== TURN_EXPECTATION_KEY &&
        key !== TURN_CHECKS_KEY &&
        key !== CONVERSATION_CHECKS_KEY &&
        key !== CASE_MODE_KEY
    )
  );
  const turnChecksPayload = buildConversationChecksPayload(turn.checks);
  const conversationChecksPayload =
    index === 0 ? buildConversationChecksPayload(taskState.conversationCheckConditions) : {};
  return {
    ...inputs,
    [CASE_MODE_KEY]: 'conversation',
    ...(turn.expectation.trim() ? { [TURN_EXPECTATION_KEY]: turn.expectation.trim() } : {}),
    ...((turnChecksPayload.conditions?.length ?? 0) > 0 ? { [TURN_CHECKS_KEY]: turnChecksPayload } : {}),
    ...((conversationChecksPayload.conditions?.length ?? 0) > 0
      ? { [CONVERSATION_CHECKS_KEY]: conversationChecksPayload }
      : {}),
  };
}
