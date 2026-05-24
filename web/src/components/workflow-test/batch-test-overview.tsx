'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  Ban,
  Loader2,
  WandSparkles,
  Plus,
  Settings2,
  MoreHorizontal,
  Trash2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Progress } from '@/components/ui/progress';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { JudgePromptSettingsDialog } from './judge-prompt-settings-dialog';
import { CaseDialog } from './case-dialog';
import { GenerateCasesDialog } from './generate-cases-dialog';
import { ScenarioDialog } from './scenario-dialog';
import { RecognizeScenariosDialog } from './recognize-scenarios-dialog';
import {
  useCancelWorkflowTestBatch,
  useDeleteWorkflowTestCases,
  useExecuteWorkflowTestBatch,
  useRetestWorkflowTestBatch,
  useUpdateWorkflowTestCase,
  useWorkflowTestBatches,
  useWorkflowTestCases,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useWorkflowDraft } from '@/hooks/workflow/use-workflow';
import { WORKFLOW_TEST_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { workflowTestService } from '@/services/workflow-test.service';
import type { WorkflowNode } from '@/components/workflow/store';
import { formatQuestionTypeLabel } from './question-type';

interface BatchTestOverviewProps {
  agentId: string;
  agentName?: string;
  agentDescription?: string;
  view?: 'case-library' | 'batches';
}

type BatchStatusKey = 'queued' | 'running' | 'completed' | 'stopped' | 'canceled';
type BatchResultKey = 'running' | 'incomplete' | 'completed';

function statusLabel(status: string, commonT: (key: 'enabled' | 'disabled' | 'none') => string) {
  if (status === 'enabled') return commonT('enabled');
  if (status === 'disabled') return commonT('disabled');
  return status || commonT('none');
}

function batchStatusLabel(status: string, statusT: (key: BatchStatusKey) => string, none: string) {
  const map: Record<string, string> = {
    queued: statusT('queued'),
    running: statusT('running'),
    completed: statusT('completed'),
    stopped: statusT('stopped'),
    canceled: statusT('canceled'),
  };
  return map[status] || status || none;
}

function batchStatusClass(status: string) {
  if (status === 'queued' || status === 'running') return 'bg-blue-50 text-blue-700';
  if (status === 'completed') return 'bg-emerald-50 text-emerald-700';
  if (status === 'stopped') return 'bg-amber-50 text-amber-700';
  if (status === 'canceled') return 'bg-slate-100 text-slate-500';
  return '';
}

function batchResultText(
  batch: {
    status: string;
    case_count: number;
    passed_count: number;
    failed_count: number;
    review_count: number;
  },
  resultT: (key: BatchResultKey, values?: Record<string, string | number | Date>) => string
) {
  if (batch.status === 'running') {
    return resultT('running', { count: batch.case_count });
  }
  if (batch.status !== 'completed') {
    return resultT('incomplete');
  }
  return resultT('completed', {
    passed: batch.passed_count,
    total: batch.case_count,
    failed: batch.failed_count,
    review: batch.review_count,
  });
}

function batchFinishedCount(batch: {
  passed_count: number;
  failed_count: number;
  review_count: number;
}) {
  return batch.passed_count + batch.failed_count + batch.review_count;
}

function batchProgressValue(batch: {
  case_count: number;
  passed_count: number;
  failed_count: number;
  review_count: number;
}) {
  if (batch.case_count <= 0) return 0;
  return Math.min(100, Math.round((batchFinishedCount(batch) / batch.case_count) * 100));
}

function isFileInputType(type: unknown) {
  return type === 'file' || type === 'file-list' || type === 'array[file]';
}

function draftSupportsFileInputs(draft: { graph?: { nodes?: WorkflowNode[] } } | undefined) {
  const nodes = draft?.graph?.nodes;
  if (!Array.isArray(nodes)) return true;
  const startNode = nodes.find(node => node?.data?.type === 'start');
  const variables = startNode?.data?.variables;
  if (!Array.isArray(variables)) return false;
  return variables.some(variable => isFileInputType(variable?.type));
}

export function BatchTestOverview({
  agentId,
  agentName,
  agentDescription,
  view = 'case-library',
}: BatchTestOverviewProps) {
  const t = useT('agents.workflowTest.overview');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const batchStatusT = useT('agents.workflowTest.batchStatus');
  const batchResultT = useT('agents.workflowTest.batchResult');
  const queryClient = useQueryClient();
  const [settingsOpen, setSettingsOpen] = React.useState(false);
  const [caseDialogOpen, setCaseDialogOpen] = React.useState(false);
  const [generateDialogOpen, setGenerateDialogOpen] = React.useState(false);
  const [scenarioDialogOpen, setScenarioDialogOpen] = React.useState(false);
  const [recognizeScenariosOpen, setRecognizeScenariosOpen] = React.useState(false);
  const [editingCaseId, setEditingCaseId] = React.useState<string | null>(null);
  const [deletingCaseIds, setDeletingCaseIds] = React.useState<string[]>([]);
  const [selectedCaseIds, setSelectedCaseIds] = React.useState<string[]>([]);
  const [caseSearch, setCaseSearch] = React.useState('');
  const [caseScenarioFilter, setCaseScenarioFilter] = React.useState('all');
  const [caseTypeFilter, setCaseTypeFilter] = React.useState('all');
  const [caseStatusFilter, setCaseStatusFilter] = React.useState('all');
  const [batchStatusFilter, setBatchStatusFilter] = React.useState('all');
  const { data: casesData, isLoading: casesLoading } = useWorkflowTestCases(agentId);
  const { data: scenariosData } = useWorkflowTestScenarios(agentId);
  const { data: batchesData } = useWorkflowTestBatches(agentId);
  const { data: workflowDraft } = useWorkflowDraft(agentId);
  const executeBatch = useExecuteWorkflowTestBatch(agentId);
  const cancelBatch = useCancelWorkflowTestBatch(agentId);
  const retestBatch = useRetestWorkflowTestBatch(agentId);
  const updateCase = useUpdateWorkflowTestCase(agentId);
  const deleteCases = useDeleteWorkflowTestCases(agentId);
  const cases = React.useMemo(() => casesData?.data?.items ?? [], [casesData]);
  const scenarios = React.useMemo(() => scenariosData?.data?.items ?? [], [scenariosData]);
  const batches = React.useMemo(() => batchesData?.data?.items ?? [], [batchesData]);
  const enabledCases = cases.filter(item => item.status === 'enabled');
  const disabledCases = cases.filter(item => item.status !== 'enabled');
  const coveredScenarioIds = new Set(cases.map(item => item.scenario_id).filter(Boolean));
  const scenariosById = new Map(scenarios.map(scene => [scene.id, scene.name]));
  const editingCase = cases.find(item => item.id === editingCaseId) ?? null;
  const filteredCases = cases.filter(item => {
    const matchesSearch = !caseSearch.trim() || item.content.includes(caseSearch.trim());
    const matchesScenario = caseScenarioFilter === 'all' || item.scenario_id === caseScenarioFilter;
    const matchesType = caseTypeFilter === 'all' || item.question_type === caseTypeFilter;
    const matchesStatus = caseStatusFilter === 'all' || item.status === caseStatusFilter;
    return matchesSearch && matchesScenario && matchesType && matchesStatus;
  });
  const filteredBatches =
    batchStatusFilter === 'all'
      ? batches
      : batches.filter(item => item.status === batchStatusFilter);
  const runningBatch = batches.find(item => item.status === 'running') ?? null;
  const shouldShowRunningBatch =
    view === 'batches' &&
    !!runningBatch &&
    (batchStatusFilter === 'all' || batchStatusFilter === 'running');
  const tableBatches = shouldShowRunningBatch
    ? filteredBatches.filter(item => item.id !== runningBatch.id)
    : filteredBatches;
  const selectedCases = cases.filter(item => selectedCaseIds.includes(item.id));
  const supportsAttachments = draftSupportsFileInputs(workflowDraft);
  const allCasesSelected =
    filteredCases.length > 0 && filteredCases.every(item => selectedCaseIds.includes(item.id));
  React.useEffect(() => {
    setSelectedCaseIds(prev => prev.filter(id => cases.some(item => item.id === id)));
  }, [cases]);

  const updateSelectedCaseStatus = async (status: 'enabled' | 'disabled') => {
    await Promise.all(
      selectedCases.map(item =>
        workflowTestService.updateCase(agentId, item.id, {
          content: item.content,
          expected_result: item.expected_result,
          scenario_id: item.scenario_id,
          question_type: item.question_type,
          status,
          turns: item.turns,
        })
      )
    );
    setSelectedCaseIds([]);
    toast.success(
      t(status === 'enabled' ? 'cases.batchEnabled' : 'cases.batchDisabled', {
        count: selectedCases.length,
      })
    );
    queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
  };

  const requestDeleteCases = (caseIds: string[]) => {
    setDeletingCaseIds(Array.from(new Set(caseIds)));
  };

  const confirmDeleteCases = () => {
    if (deletingCaseIds.length === 0) return;
    const ids = deletingCaseIds;
    deleteCases.mutate(
      { case_ids: ids },
      {
        onSuccess: () => {
          setSelectedCaseIds(prev => prev.filter(id => !ids.includes(id)));
          setDeletingCaseIds([]);
        },
      }
    );
  };

  const buildRetestName = (batchName: string) => t('batches.retestName', { name: batchName });
  const pageDescription =
    view === 'batches' ? t('batches.description') : agentDescription || t('descriptionFallback');
  const defaultRecognitionContext = [agentName, agentDescription].filter(Boolean).join('\n');

  const sceneCards = scenarios.map(scene => {
    const count = cases.filter(item => item.scenario_id === scene.id).length;
    return {
      name: scene.name,
      description: scene.description,
      count,
      covered: coveredScenarioIds.has(scene.id),
    };
  });

  return (
    <div className="min-h-full bg-slate-50 px-8 py-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-6">
        <div className="text-sm text-slate-500">
          {t('breadcrumb', { agentName: agentName || commonT('agentFallback') })}
        </div>

        <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h1 className="text-2xl font-semibold text-slate-950">
                {agentName || t('titleFallback')}
              </h1>
              <p className="mt-2 max-w-3xl text-sm text-slate-600">{pageDescription}</p>
              <div className="mt-4 flex items-center gap-2 text-sm text-slate-500">
                <Badge variant="outline">{commonT('chatWorkflow')}</Badge>
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                <span>{commonT('currentDraftSnapshot')}</span>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {view === 'batches' ? (
                <>
                  <Button variant="outline" onClick={() => setSettingsOpen(true)}>
                    <Settings2 className="mr-2 size-4" />
                    {t('actions.judgeSettings')}
                  </Button>
                  <Button className="bg-slate-950 text-white hover:bg-slate-800" asChild>
                    <Link href={`/console/agents/${agentId}/batch-test/batches/new`}>
                      <WandSparkles className="mr-2 size-4" />
                      {t('actions.createBatch')}
                    </Link>
                  </Button>
                </>
              ) : scenarios.length > 0 ? (
                <>
                  <Button
                    className="bg-slate-950 text-white hover:bg-slate-800"
                    onClick={() => setGenerateDialogOpen(true)}
                  >
                    <WandSparkles className="mr-2 size-4" />
                    {t('actions.generateCases')}
                  </Button>
                  <Button variant="outline" onClick={() => setCaseDialogOpen(true)}>
                    <Plus className="mr-2 size-4" />
                    {t('actions.createCase')}
                  </Button>
                </>
              ) : null}
            </div>
          </div>

          {view === 'case-library' ? (
            <div className="mt-6 grid grid-cols-4 rounded-xl border border-slate-200 bg-slate-50">
              <div className="p-4">
                <div className="text-sm text-slate-500">{t('stats.totalCases')}</div>
                <div className="mt-2 text-3xl font-semibold">{cases.length}</div>
              </div>
              <div className="p-4">
                <div className="text-sm text-slate-500">{t('stats.enabledCases')}</div>
                <div className="mt-2 text-3xl font-semibold">{enabledCases.length}</div>
              </div>
              <div className="p-4">
                <div className="text-sm text-slate-500">{t('stats.disabledCases')}</div>
                <div className="mt-2 text-3xl font-semibold">{disabledCases.length}</div>
              </div>
              <div className="p-4">
                <div className="text-sm text-slate-500">{t('stats.batches')}</div>
                <div className="mt-2 text-3xl font-semibold">{batches.length}</div>
              </div>
            </div>
          ) : null}
        </section>

        {view === 'case-library' ? (
          <>
            <Card className="rounded-2xl">
              <CardHeader className="flex-row items-center justify-between">
                <div>
                  <CardTitle>{t('scenarios.title')}</CardTitle>
                  <p className="mt-2 text-sm text-slate-600">{t('scenarios.description')}</p>
                </div>
                {sceneCards.length > 0 ? (
                  <div className="flex gap-2">
                    <Button variant="outline" onClick={() => setRecognizeScenariosOpen(true)}>
                      {t('actions.rerecognizeScenarios')}
                    </Button>
                    <Button variant="outline" onClick={() => setScenarioDialogOpen(true)}>
                      {t('actions.editScenarios')}
                    </Button>
                  </div>
                ) : null}
              </CardHeader>
              <CardContent>
                {sceneCards.length === 0 ? (
                  <div className="flex min-h-[220px] flex-col items-center justify-center rounded-xl border border-dashed border-slate-200 bg-slate-50 px-6 text-center">
                    <div className="rounded-xl bg-white p-3 text-blue-600 shadow-sm">
                      <WandSparkles className="size-5" />
                    </div>
                    <div className="mt-4 font-semibold text-slate-950">
                      {t('scenarios.emptyTitle')}
                    </div>
                    <p className="mt-2 max-w-md text-sm text-slate-600">
                      {t('scenarios.emptyDescription')}
                    </p>
                    <Button className="mt-5" onClick={() => setRecognizeScenariosOpen(true)}>
                      {t('actions.recognizeScenarios')}
                    </Button>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-3 xl:grid-cols-5">
                    {sceneCards.map(scene => (
                      <div
                        key={scene.name}
                        className="rounded-xl border border-slate-200 bg-slate-50 p-4"
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div className="font-semibold text-slate-950">{scene.name}</div>
                          <Badge
                            className={
                              scene.covered
                                ? 'bg-emerald-50 text-emerald-700'
                                : 'bg-slate-100 text-slate-500'
                            }
                          >
                            {scene.covered ? t('scenarios.covered') : t('scenarios.uncovered')}
                          </Badge>
                        </div>
                        <div className="mt-1 text-sm text-slate-500">
                          {t('scenarios.caseCount', { count: scene.count })}
                        </div>
                        <p className="mt-3 line-clamp-3 text-sm text-slate-600">
                          {scene.description}
                        </p>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card id="case-library" className="scroll-mt-6 rounded-2xl">
              <CardHeader>
                <CardTitle>{t('cases.title')}</CardTitle>
                <p className="text-sm text-slate-600">{t('cases.description')}</p>
              </CardHeader>
              <CardContent className="p-0">
                <div className="grid grid-cols-4 gap-3 border-y border-slate-200 p-4">
                  <Input
                    value={caseSearch}
                    onChange={event => setCaseSearch(event.target.value)}
                    placeholder={t('cases.searchPlaceholder')}
                  />
                  <Select value={caseScenarioFilter} onValueChange={setCaseScenarioFilter}>
                    <SelectTrigger>
                      <SelectValue placeholder={t('table.businessScenario')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{t('cases.scenarioAll')}</SelectItem>
                      {scenarios.map(scene => (
                        <SelectItem key={scene.id} value={scene.id}>
                          {scene.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Select value={caseTypeFilter} onValueChange={setCaseTypeFilter}>
                    <SelectTrigger>
                      <SelectValue placeholder={t('table.questionType')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{t('cases.typeAll')}</SelectItem>
                      <SelectItem value="core">{typeT('core')}</SelectItem>
                      <SelectItem value="extension">{typeT('extension')}</SelectItem>
                      <SelectItem value="fuzzy">{typeT('fuzzy')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <Select value={caseStatusFilter} onValueChange={setCaseStatusFilter}>
                    <SelectTrigger>
                      <SelectValue placeholder={t('table.status')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{t('cases.statusAll')}</SelectItem>
                      <SelectItem value="enabled">{commonT('enabled')}</SelectItem>
                      <SelectItem value="disabled">{commonT('disabled')}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-12">
                        <Checkbox
                          checked={allCasesSelected}
                          onCheckedChange={checked => {
                            const filteredIds = filteredCases.map(item => item.id);
                            setSelectedCaseIds(prev =>
                              checked
                                ? Array.from(new Set([...prev, ...filteredIds]))
                                : prev.filter(id => !filteredIds.includes(id))
                            );
                          }}
                        />
                      </TableHead>
                      <TableHead>{t('table.questionContent')}</TableHead>
                      <TableHead>{t('table.businessScenario')}</TableHead>
                      <TableHead>{t('table.questionType')}</TableHead>
                      <TableHead>{t('table.status')}</TableHead>
                      <TableHead>{t('table.updatedAt')}</TableHead>
                      <TableHead className="text-right">{t('table.operations')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {casesLoading ? (
                      <TableRow>
                        <TableCell colSpan={7} className="h-32 text-center text-slate-500">
                          {t('cases.loading')}
                        </TableCell>
                      </TableRow>
                    ) : cases.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={7} className="h-32 text-center text-slate-500">
                          {t('cases.empty')}
                        </TableCell>
                      </TableRow>
                    ) : filteredCases.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={7} className="h-32 text-center text-slate-500">
                          {t('cases.filteredEmpty')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      filteredCases.map(item => (
                        <TableRow key={item.id}>
                          <TableCell>
                            <Checkbox
                              checked={selectedCaseIds.includes(item.id)}
                              onCheckedChange={checked => {
                                setSelectedCaseIds(prev =>
                                  checked
                                    ? Array.from(new Set([...prev, item.id]))
                                    : prev.filter(id => id !== item.id)
                                );
                              }}
                            />
                          </TableCell>
                          <TableCell className="max-w-xl">
                            <div className="font-medium text-slate-950">{item.content}</div>
                            {item.turns?.length > 1 ? (
                              <div className="mt-1 text-xs text-slate-500">
                                共 {item.turns.length} 轮对话
                              </div>
                            ) : null}
                            {item.turns?.some(turn => turn.attachments?.length) ? (
                              <div className="mt-1 text-xs text-slate-500">
                                {commonT('attachmentsIncluded')}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            {item.scenario_id
                              ? scenariosById.get(item.scenario_id) || commonT('none')
                              : commonT('none')}
                          </TableCell>
                          <TableCell>{formatQuestionTypeLabel(item.question_type, typeT)}</TableCell>
                          <TableCell>
                            <Badge
                              className={
                                item.status === 'enabled'
                                  ? 'bg-emerald-50 text-emerald-700'
                                  : 'bg-slate-100 text-slate-500'
                              }
                            >
                              {statusLabel(item.status, commonT)}
                            </Badge>
                          </TableCell>
                          <TableCell>{new Date(item.updated_at).toLocaleString()}</TableCell>
                          <TableCell className="text-right">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setEditingCaseId(item.id)}
                            >
                              {commonT('edit')}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              disabled={updateCase.isPending}
                              onClick={() =>
                                updateCase.mutate({
                                  caseId: item.id,
                                  data: {
                                    content: item.content,
                                    expected_result: item.expected_result,
                                    scenario_id: item.scenario_id,
                                    question_type: item.question_type,
                                    status: item.status === 'enabled' ? 'disabled' : 'enabled',
                                    turns: item.turns,
                                  },
                                })
                              }
                            >
                              {item.status === 'enabled' ? commonT('disable') : commonT('enable')}
                            </Button>
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                                  <MoreHorizontal className="size-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end">
                                <DropdownMenuItem
                                  className="text-red-600 focus:text-red-600"
                                  onSelect={() => requestDeleteCases([item.id])}
                                >
                                  <Trash2 className="mr-2 size-4" />
                                  {commonT('delete')}
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
                {selectedCaseIds.length > 0 ? (
                  <div className="sticky bottom-0 z-10 flex items-center justify-between border-t border-slate-200 bg-white px-4 py-3 shadow-[0_-8px_24px_rgba(15,23,42,0.08)]">
                    <div className="text-sm text-slate-600">
                      {t('cases.selectedCount', { count: selectedCaseIds.length })}
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={updateCase.isPending}
                        onClick={() => updateSelectedCaseStatus('enabled')}
                      >
                        {t('cases.batchEnable')}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={updateCase.isPending}
                        onClick={() => updateSelectedCaseStatus('disabled')}
                      >
                        {t('cases.batchDisable')}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="border-red-200 text-red-600 hover:bg-red-50 hover:text-red-700"
                        disabled={deleteCases.isPending}
                        onClick={() => requestDeleteCases(selectedCaseIds)}
                      >
                        <Trash2 className="mr-1 size-4" />
                        {t('cases.batchDelete')}
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => setSelectedCaseIds([])}>
                        {t('cases.clearSelection')}
                      </Button>
                    </div>
                  </div>
                ) : null}
              </CardContent>
            </Card>
          </>
        ) : null}

        {view === 'batches' ? (
          <Card id="batches" className="scroll-mt-6 rounded-2xl">
            <CardHeader className="flex-row items-center justify-between">
              <div>
                <CardTitle>{t('batches.title')}</CardTitle>
                <p className="mt-2 text-sm text-slate-600">{t('batches.description')}</p>
              </div>
              <div className="flex items-center gap-2">
                {[
                  ['all', commonT('all')],
                  ['queued', batchStatusT('queued')],
                  ['running', batchStatusT('running')],
                  ['completed', batchStatusT('completed')],
                  ['stopped', batchStatusT('stopped')],
                  ['canceled', batchStatusT('canceled')],
                ].map(([value, label]) => (
                  <Button
                    key={value}
                    variant={batchStatusFilter === value ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setBatchStatusFilter(value)}
                  >
                    {label}
                  </Button>
                ))}
              </div>
            </CardHeader>
            <CardContent className="p-0">
              {shouldShowRunningBatch && runningBatch ? (
                <div className="border-t border-slate-200 p-4">
                  <div className="rounded-xl border border-blue-100 bg-blue-50/50 px-4 py-3">
                    <div className="flex items-center justify-between gap-4">
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <div className="truncate text-sm font-medium text-slate-950">
                            {t('batches.activeTitle', { name: runningBatch.name })}
                          </div>
                          <Badge className="bg-blue-100 text-blue-700">
                            <Loader2 className="mr-1 size-3 animate-spin" />
                            {batchStatusT('running')}
                          </Badge>
                        </div>
                        <div className="mt-3 flex items-center gap-3">
                          <Progress
                            value={batchProgressValue(runningBatch)}
                            className="h-1.5 max-w-md flex-1 bg-white"
                          />
                          <span className="shrink-0 text-xs text-slate-500">
                            {t('batches.activeProgress', {
                              done: batchFinishedCount(runningBatch),
                              total: runningBatch.case_count,
                            })}
                          </span>
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        <Button variant="outline" size="sm" asChild>
                          <Link href={`/console/agents/${agentId}/batch-test/${runningBatch.id}`}>
                            {t('batchActions.viewProgress')}
                          </Link>
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={cancelBatch.isPending}
                          onClick={() => cancelBatch.mutate(runningBatch.id)}
                        >
                          <Ban className="mr-1 size-4" />
                          {commonT('cancel')}
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>
              ) : null}
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('table.batch')}</TableHead>
                    <TableHead>{t('table.batchStatus')}</TableHead>
                    <TableHead>{t('table.questionCount')}</TableHead>
                    <TableHead>{t('table.testResult')}</TableHead>
                    <TableHead className="text-right">{t('table.operations')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {batches.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="h-28 text-center text-slate-500">
                        {t('batches.empty')}
                      </TableCell>
                    </TableRow>
                  ) : tableBatches.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="h-28 text-center text-slate-500">
                        {t('batches.filteredEmpty')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    tableBatches.map(batch => (
                      <TableRow key={batch.id}>
                        <TableCell>
                          <div className="font-semibold text-slate-950">{batch.name}</div>
                          <div className="text-xs text-slate-500">
                            {t('batches.createdAt')} {new Date(batch.created_at).toLocaleString()}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge className={batchStatusClass(batch.status)}>
                            {batchStatusLabel(batch.status, batchStatusT, commonT('none'))}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          {t('scenarios.caseCount', { count: batch.case_count })}
                        </TableCell>
                        <TableCell>{batchResultText(batch, batchResultT)}</TableCell>
                        <TableCell className="text-right">
                          {batch.status === 'queued' ? (
                            <Button
                              variant="link"
                              size="sm"
                              disabled={executeBatch.isPending}
                              onClick={() => executeBatch.mutate(batch.id)}
                            >
                              {t('batchActions.start')}
                            </Button>
                          ) : null}
                          {batch.status === 'queued' ? (
                            <Button variant="link" size="sm" asChild>
                              <Link href={`/console/agents/${agentId}/batch-test/${batch.id}`}>
                                {t('batchActions.viewDetail')}
                              </Link>
                            </Button>
                          ) : null}
                          {batch.status === 'running' ? (
                            <Button variant="link" size="sm" asChild>
                              <Link href={`/console/agents/${agentId}/batch-test/${batch.id}`}>
                                {t('batchActions.viewProgress')}
                              </Link>
                            </Button>
                          ) : null}
                          {batch.status === 'queued' || batch.status === 'running' ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              disabled={cancelBatch.isPending}
                              onClick={() => cancelBatch.mutate(batch.id)}
                            >
                              <Ban className="mr-1 size-4" />
                              {commonT('cancel')}
                            </Button>
                          ) : batch.status !== 'queued' ? (
                            <Button variant="link" size="sm" asChild>
                              <Link href={`/console/agents/${agentId}/batch-test/${batch.id}`}>
                                {t('batchActions.viewResult')}
                              </Link>
                            </Button>
                          ) : null}
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                                <MoreHorizontal className="size-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              {batch.status === 'queued' || batch.status === 'running' ? (
                                <DropdownMenuItem
                                  className="text-red-600"
                                  disabled={cancelBatch.isPending}
                                  onSelect={() => cancelBatch.mutate(batch.id)}
                                >
                                  {commonT('cancelTest')}
                                </DropdownMenuItem>
                              ) : (
                                <DropdownMenuItem
                                  disabled={retestBatch.isPending}
                                  onSelect={() =>
                                    retestBatch.mutate({
                                      batchId: batch.id,
                                      data: { name: buildRetestName(batch.name) },
                                    })
                                  }
                                >
                                  {commonT('retest')}
                                </DropdownMenuItem>
                              )}
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        ) : null}
      </div>

      <JudgePromptSettingsDialog
        agentId={agentId}
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
      />
      <ScenarioDialog
        agentId={agentId}
        scenarios={scenarios}
        open={scenarioDialogOpen}
        onOpenChange={setScenarioDialogOpen}
      />
      <RecognizeScenariosDialog
        agentId={agentId}
        defaultContext={defaultRecognitionContext}
        open={recognizeScenariosOpen}
        onOpenChange={setRecognizeScenariosOpen}
      />
      <CaseDialog
        agentId={agentId}
        scenarios={scenarios.map(scene => ({ id: scene.id, name: scene.name }))}
        open={caseDialogOpen}
        onOpenChange={setCaseDialogOpen}
        supportsAttachments={supportsAttachments}
      />
      <CaseDialog
        agentId={agentId}
        scenarios={scenarios.map(scene => ({ id: scene.id, name: scene.name }))}
        caseItem={editingCase}
        open={!!editingCaseId}
        onOpenChange={open => {
          if (!open) setEditingCaseId(null);
        }}
        supportsAttachments={supportsAttachments}
      />
      <GenerateCasesDialog
        agentId={agentId}
        scenarios={scenarios.map(scene => ({ id: scene.id, name: scene.name }))}
        open={generateDialogOpen}
        onOpenChange={setGenerateDialogOpen}
      />
      <ConfirmDialog
        open={deletingCaseIds.length > 0}
        onOpenChange={open => {
          if (!open && !deleteCases.isPending) setDeletingCaseIds([]);
        }}
        title={
          deletingCaseIds.length > 1
            ? t('cases.batchDeleteConfirmTitle')
            : t('cases.deleteConfirmTitle')
        }
        description={
          deletingCaseIds.length > 1
            ? t('cases.batchDeleteConfirmDescription', { count: deletingCaseIds.length })
            : t('cases.deleteConfirmDescription')
        }
        confirmText={commonT('delete')}
        cancelText={commonT('cancel')}
        loading={deleteCases.isPending}
        variant="warning"
        onConfirm={confirmDeleteCases}
      />
    </div>
  );
}
