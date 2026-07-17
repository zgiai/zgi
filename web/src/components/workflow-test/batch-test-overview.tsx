'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  Ban,
  Clock,
  Loader2,
  WandSparkles,
  Plus,
  ScanSearch,
  Settings2,
  SquarePen,
  MoreHorizontal,
  Trash2,
  PlayCircle,
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
  useCancelWorkflowTestGenerationTask,
  useCancelWorkflowTestScenarioRecognitionTask,
  useCreateWorkflowTestBatch,
  useDeleteWorkflowTestGenerationTask,
  useDeleteWorkflowTestCases,
  useExecuteWorkflowTestBatch,
  useLatestWorkflowTestScenarioRecognitionTask,
  useLatestWorkflowTestGenerationTask,
  useResumeWorkflowTestGenerationTask,
  useRetestWorkflowTestBatch,
  useUpdateWorkflowTestCase,
  useWorkflowTestBatches,
  useWorkflowTestCases,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useWorkflowDraft } from '@/hooks/workflow/use-workflow';
import { WORKFLOW_TEST_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { workflowTestService } from '@/services/workflow-test.service';
import { localizeWorkflowTestError } from '@/utils/workflow-test-error';
import type {
  WorkflowTestBatch,
  WorkflowTestCase,
  WorkflowTestGenerationTask,
} from '@/services/types/workflow-test';
import { QUESTION_TYPE_OPTIONS, formatQuestionTypeLabel } from './question-type';
import {
  draftAttachmentAcceptExtensions,
  draftRequiresCurrentTurnFiles,
  draftSupportsFileInputs,
  expectedCheckConditionCount,
  expectedChecks,
  generatedFixtureSpecs,
  hasGeneratedAssets,
  turnExpectation,
  visibleInputEntries,
  workflowDraftMode,
} from './case-metadata';
import { getAgentDetailBatchTestHref } from '@/utils/agent-detail-routes';

interface BatchTestOverviewProps {
  agentId: string;
  agentName?: string;
  agentDescription?: string;
  view?: 'case-library' | 'batches';
  permissions?: WorkflowTestActionPermissions;
}

interface WorkflowTestActionPermissions {
  canUpdate: boolean;
  canDebug: boolean;
  canStop: boolean;
  canViewLogs: boolean;
}

type BatchStatusKey = 'queued' | 'running' | 'completed' | 'stopped' | 'canceled';
type BatchResultKey = 'running' | 'incomplete' | 'completed';
type GenerationKind = 'conversation' | 'task' | 'file';

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

function defaultBatchName(template: (values: { date: string; time: string }) => string) {
  const now = new Date();
  const yyyy = now.getFullYear();
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  const dd = String(now.getDate()).padStart(2, '0');
  const hh = String(now.getHours()).padStart(2, '0');
  const min = String(now.getMinutes()).padStart(2, '0');
  return template({ date: `${yyyy}-${mm}-${dd}`, time: `${hh}:${min}` });
}

function generationContextMode(context?: string) {
  const match = (context || '').match(/\[workflow_test_case_mode:(task|conversation)\]/);
  return match?.[1] as 'task' | 'conversation' | undefined;
}

function generationContextHasFileGeneration(context?: string) {
  return (context || '').includes('[workflow_test_file_generation:');
}

function generationKindFromTask(
  task: WorkflowTestGenerationTask | null,
  fallbackMode: 'task' | 'conversation',
  fallbackHasFiles: boolean
): GenerationKind {
  if (task) {
    const mode = generationContextMode(task.context);
    if (mode === 'task') {
      return generationContextHasFileGeneration(task.context) ? 'file' : 'task';
    }
    if (mode === 'conversation') {
      return 'conversation';
    }
  }
  if (fallbackMode === 'task') {
    return fallbackHasFiles ? 'file' : 'task';
  }
  return 'conversation';
}

function generationProgressValue(created: number, requested: number, status?: string) {
  if (status === 'completed') return 100;
  if (requested <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((created / requested) * 100)));
}

function nextGenerationIndex(created: number, requested: number) {
  if (requested <= 0) return 1;
  return Math.min(requested, created + 1);
}

function generationTitleKey(kind: GenerationKind, status?: string) {
  if (status === 'canceling') return 'generationCancelingTitle';
  if (status === 'canceled') return 'generationCanceledTitle';
  if (status === 'failed') return 'generationFailedTitle';
  if (status === 'completed') return 'generationCompletedTitle';
  if (kind === 'file') return 'generatingFileTitle';
  if (kind === 'task') return 'generatingTaskCaseTitle';
  return 'generatingQuestionTitle';
}

function generationStatusKey(status: string | undefined, created: number) {
  if (status === 'queued') return 'generationStatusQueued';
  if (status === 'canceling') return 'generationStatusCanceling';
  if (status === 'canceled') return 'generationStatusCanceled';
  if (status === 'failed') return 'generationStatusFailed';
  if (status === 'completed') return 'generationStatusCompleted';
  if (created === 0) return 'generationStatusFirst';
  return 'generationStatusNext';
}

function scenarioRecognitionProgressValue(status?: string) {
  if (status === 'completed') return 100;
  if (status === 'canceled') return 100;
  if (status === 'failed') return 100;
  if (status === 'canceling') return 80;
  if (status === 'running') return 45;
  if (status === 'queued') return 8;
  return 0;
}

function scenarioRecognitionTitleKey(status?: string) {
  if (status === 'canceling') return 'recognitionCancelingTitle';
  if (status === 'canceled') return 'recognitionCanceledTitle';
  if (status === 'failed') return 'recognitionFailedTitle';
  if (status === 'completed') return 'recognitionCompletedTitle';
  if (status === 'queued') return 'recognitionQueuedTitle';
  return 'recognitionRunningTitle';
}

function scenarioRecognitionStatusKey(status?: string) {
  if (status === 'queued') return 'recognitionStatusQueued';
  if (status === 'canceling') return 'recognitionStatusCanceling';
  if (status === 'canceled') return 'recognitionStatusCanceled';
  if (status === 'failed') return 'recognitionStatusFailed';
  if (status === 'completed') return 'recognitionStatusCompleted';
  return 'recognitionStatusRunning';
}

function formatTaskCaseFocus(
  item: WorkflowTestCase,
  none: string,
  labels: { variables: string; files: string; checks: string; criticalChecks: string }
) {
  const inputs = visibleInputEntries(item.turns?.[0]);
  const checks = expectedChecks(item);
  const checkCount = expectedCheckConditionCount(checks);
  const criticalCheckCount =
    checks.conditions?.filter(condition => condition.severity === 'critical').length ?? 0;
  const fixtureCount = generatedFixtureSpecs(item).length;
  const parts: string[] = [];
  if (inputs.length > 0) {
    parts.push(
      `${labels.variables}: ${inputs
        .slice(0, 2)
        .map(([key]) => key)
        .join(', ')}`
    );
  }
  if (fixtureCount > 0) {
    parts.push(`${labels.files}: ${fixtureCount}`);
  }
  if (checkCount > 0) {
    parts.push(
      criticalCheckCount > 0
        ? `${labels.checks}: ${checkCount} (${labels.criticalChecks}: ${criticalCheckCount})`
        : `${labels.checks}: ${checkCount}`
    );
  }
  return parts.length > 0 ? parts.join(' / ') : none;
}

function formatConversationCaseFocus(
  item: WorkflowTestCase,
  labels: { turns: string; expectations: string }
) {
  const turns = item.turns ?? [];
  const turnCount = Math.max(turns.length, 1);
  const expectationCount = turns.filter(turn => turnExpectation(turn).trim()).length;
  return expectationCount > 0
    ? `${turnCount} ${labels.turns} / ${expectationCount} ${labels.expectations}`
    : `${turnCount} ${labels.turns}`;
}

export function BatchTestOverview({
  agentId,
  agentName,
  agentDescription,
  view = 'case-library',
  permissions,
}: BatchTestOverviewProps) {
  const t = useT('agents.workflowTest.overview');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const batchStatusT = useT('agents.workflowTest.batchStatus');
  const batchResultT = useT('agents.workflowTest.batchResult');
  const toastT = useT('agents.workflowTest.toasts');
  const errorT = useT('agents.workflowTest.userErrors');
  const router = useRouter();
  const queryClient = useQueryClient();
  const batchTestHref = getAgentDetailBatchTestHref(agentId, 'workflow');
  const newBatchHref = `${getAgentDetailBatchTestHref(agentId, 'workflow', 'batches')}/new`;
  const getBatchResultHref = React.useCallback(
    (batchId: string) => `${batchTestHref}/${batchId}`,
    [batchTestHref]
  );
  const [settingsOpen, setSettingsOpen] = React.useState(false);
  const [caseDialogOpen, setCaseDialogOpen] = React.useState(false);
  const [generateDialogOpen, setGenerateDialogOpen] = React.useState(false);
  const [scenarioDialogOpen, setScenarioDialogOpen] = React.useState(false);
  const [recognizeScenariosOpen, setRecognizeScenariosOpen] = React.useState(false);
  const [pendingGenerationCount, setPendingGenerationCount] = React.useState<number | null>(null);
  const [completedGenerationTaskId, setCompletedGenerationTaskId] = React.useState<string | null>(
    null
  );
  const [completedScenarioRecognitionTaskId, setCompletedScenarioRecognitionTaskId] =
    React.useState<string | null>(null);
  const [editingCaseId, setEditingCaseId] = React.useState<string | null>(null);
  const [deletingCaseIds, setDeletingCaseIds] = React.useState<string[]>([]);
  const [retestingBatch, setRetestingBatch] = React.useState<WorkflowTestBatch | null>(null);
  const [expandedCaseIds, setExpandedCaseIds] = React.useState<string[]>([]);
  const [selectedCaseIds, setSelectedCaseIds] = React.useState<string[]>([]);
  const [caseSearch, setCaseSearch] = React.useState('');
  const [caseScenarioFilter, setCaseScenarioFilter] = React.useState('all');
  const [caseTypeFilter, setCaseTypeFilter] = React.useState('all');
  const [caseStatusFilter, setCaseStatusFilter] = React.useState('all');
  const [batchStatusFilter, setBatchStatusFilter] = React.useState('all');
  const { data: casesData, isLoading: casesLoading } = useWorkflowTestCases(agentId);
  const { data: scenariosData } = useWorkflowTestScenarios(agentId);
  const { data: batchesData } = useWorkflowTestBatches(agentId);
  const { data: latestGenerationTaskData } = useLatestWorkflowTestGenerationTask(agentId);
  const { data: latestScenarioRecognitionTaskData } =
    useLatestWorkflowTestScenarioRecognitionTask(agentId);
  const { data: workflowDraft } = useWorkflowDraft(agentId);
  const workflowTestMode = workflowDraftMode(workflowDraft);
  const supportsAttachments = draftSupportsFileInputs(workflowDraft);
  const requiresCurrentTurnFiles = draftRequiresCurrentTurnFiles(workflowDraft);
  const attachmentAcceptExt = React.useMemo(
    () => draftAttachmentAcceptExtensions(workflowDraft),
    [workflowDraft]
  );
  const createBatch = useCreateWorkflowTestBatch(agentId);
  const executeBatch = useExecuteWorkflowTestBatch(agentId);
  const cancelBatch = useCancelWorkflowTestBatch(agentId);
  const cancelGenerationTask = useCancelWorkflowTestGenerationTask(agentId);
  const resumeGenerationTask = useResumeWorkflowTestGenerationTask(agentId);
  const deleteGenerationTask = useDeleteWorkflowTestGenerationTask(agentId);
  const cancelScenarioRecognitionTask = useCancelWorkflowTestScenarioRecognitionTask(agentId);
  const retestBatch = useRetestWorkflowTestBatch(agentId);
  const updateCase = useUpdateWorkflowTestCase(agentId);
  const deleteCases = useDeleteWorkflowTestCases(agentId);
  const cases = React.useMemo(() => casesData?.data?.items ?? [], [casesData]);
  const scenarios = React.useMemo(() => scenariosData?.data?.items ?? [], [scenariosData]);
  const scenarioOptions = React.useMemo(
    () => scenarios.map(scene => ({ id: scene.id, name: scene.name })),
    [scenarios]
  );
  const batches = React.useMemo(() => batchesData?.data?.items ?? [], [batchesData]);
  const generationTask = latestGenerationTaskData?.data?.task ?? null;
  const scenarioRecognitionTask = latestScenarioRecognitionTaskData?.data?.task ?? null;
  const generationTaskError = generationTask?.error
    ? localizeWorkflowTestError(generationTask.error, errorT)
    : commonT('none');
  const scenarioRecognitionTaskError = scenarioRecognitionTask?.error
    ? localizeWorkflowTestError(scenarioRecognitionTask.error, errorT)
    : commonT('none');
  const isScenarioRecognitionActive =
    scenarioRecognitionTask?.status === 'queued' ||
    scenarioRecognitionTask?.status === 'running' ||
    scenarioRecognitionTask?.status === 'canceling';
  const isGenerationActive =
    generationTask?.status === 'queued' ||
    generationTask?.status === 'running' ||
    generationTask?.status === 'canceling';
  const canCancelScenarioRecognition =
    scenarioRecognitionTask?.id &&
    (scenarioRecognitionTask.status === 'queued' || scenarioRecognitionTask.status === 'running');
  const canCancelGeneration =
    generationTask?.id &&
    (generationTask.status === 'queued' || generationTask.status === 'running');
  const canResumeGeneration =
    generationTask?.id &&
    ['failed', 'canceled'].includes(generationTask.status) &&
    generationTask.created_count < generationTask.requested_count;
  const canDeleteGenerationTask =
    generationTask?.id && ['completed', 'failed', 'canceled'].includes(generationTask.status);
  const isGenerationPendingLocally = pendingGenerationCount !== null && !isGenerationActive;
  const displayedGenerationStatus = isGenerationPendingLocally ? 'running' : generationTask?.status;
  const isGenerationCompleted =
    displayedGenerationStatus === 'completed' && generationTask?.id === completedGenerationTaskId;
  const isScenarioRecognitionCompleted =
    scenarioRecognitionTask?.status === 'completed' &&
    scenarioRecognitionTask.id === completedScenarioRecognitionTaskId;
  const shouldShowScenarioRecognitionBanner =
    view === 'case-library' &&
    (isScenarioRecognitionActive ||
      isScenarioRecognitionCompleted ||
      scenarioRecognitionTask?.status === 'canceled' ||
      scenarioRecognitionTask?.status === 'failed');
  const scenarioRecognitionProgress = scenarioRecognitionProgressValue(
    scenarioRecognitionTask?.status
  );
  const scenarioRecognitionTitle = t(
    `scenarios.${scenarioRecognitionTitleKey(scenarioRecognitionTask?.status)}`,
    {
      recognized: scenarioRecognitionTask?.recognized_count ?? 0,
      assigned: scenarioRecognitionTask?.assigned_case_count ?? 0,
    }
  );
  const scenarioRecognitionStatusText = t(
    `scenarios.${scenarioRecognitionStatusKey(scenarioRecognitionTask?.status)}`,
    {
      recognized: scenarioRecognitionTask?.recognized_count ?? 0,
      assigned: scenarioRecognitionTask?.assigned_case_count ?? 0,
      error: scenarioRecognitionTaskError,
    }
  );
  const scenarioRecognitionProgressText = t('scenarios.recognitionProgressText', {
    percent: scenarioRecognitionProgress,
  });
  const shouldShowGenerationBanner =
    view === 'case-library' &&
    (isGenerationActive ||
      isGenerationCompleted ||
      displayedGenerationStatus === 'canceled' ||
      displayedGenerationStatus === 'failed' ||
      pendingGenerationCount !== null);
  const generationBannerRequested = isGenerationPendingLocally
    ? pendingGenerationCount
    : (generationTask?.requested_count ?? pendingGenerationCount ?? 0);
  const generationBannerCreated = isGenerationPendingLocally
    ? 0
    : (generationTask?.created_count ?? 0);
  const generationKind = generationKindFromTask(
    generationTask,
    workflowTestMode,
    supportsAttachments
  );
  const generationProgress = generationProgressValue(
    generationBannerCreated,
    generationBannerRequested,
    displayedGenerationStatus
  );
  const generationCurrentIndex = nextGenerationIndex(
    generationBannerCreated,
    generationBannerRequested
  );
  const generationTitle = t(
    `cases.${generationTitleKey(generationKind, displayedGenerationStatus)}`,
    {
      count: generationBannerRequested,
      created: generationBannerCreated,
    }
  );
  const generationStatusText = t(
    `cases.${generationStatusKey(displayedGenerationStatus, generationBannerCreated)}`,
    {
      created: generationBannerCreated,
      total: generationBannerRequested,
      current: generationCurrentIndex,
      percent: generationProgress,
      error: generationTaskError,
    }
  );
  const generationProgressText = t('cases.generationProgressText', {
    created: generationBannerCreated,
    total: generationBannerRequested,
    percent: generationProgress,
  });
  const generationBannerTone =
    displayedGenerationStatus === 'failed'
      ? {
          wrapper: 'border-red-200 bg-red-50',
          icon: 'bg-white text-red-600',
          title: 'text-red-700',
          description: 'text-red-700',
        }
      : isGenerationCompleted || displayedGenerationStatus === 'canceled'
        ? {
            wrapper: 'border-emerald-200 bg-emerald-50',
            icon: 'bg-white text-emerald-600',
            title: 'text-emerald-700',
            description: 'text-slate-700',
          }
        : {
            wrapper: 'border-blue-200 bg-blue-50',
            icon: 'bg-white text-blue-600',
            title: 'text-blue-700',
            description: 'text-slate-700',
          };
  const scenarioRecognitionBannerTone =
    scenarioRecognitionTask?.status === 'failed'
      ? {
          wrapper: 'border-red-200 bg-red-50',
          icon: 'bg-white text-red-600',
          title: 'text-red-700',
          description: 'text-red-700',
        }
      : isScenarioRecognitionCompleted || scenarioRecognitionTask?.status === 'canceled'
        ? {
            wrapper: 'border-emerald-200 bg-emerald-50',
            icon: 'bg-white text-emerald-600',
            title: 'text-emerald-700',
            description: 'text-slate-700',
          }
        : {
            wrapper: 'border-blue-200 bg-blue-50',
            icon: 'bg-white text-blue-600',
            title: 'text-blue-700',
            description: 'text-slate-700',
          };
  const previousGenerationTaskStatusRef = React.useRef<string | null>(null);
  const previousScenarioRecognitionTaskStatusRef = React.useRef<string | null>(null);
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
  const activeBatches = batches.filter(
    (item): item is WorkflowTestBatch & { status: 'queued' | 'running' } =>
      item.status === 'queued' || item.status === 'running'
  );
  const activeBatchIds = new Set(activeBatches.map(item => item.id));
  const shouldShowActiveBatches =
    view === 'batches' &&
    activeBatches.length > 0 &&
    (batchStatusFilter === 'all' ||
      batchStatusFilter === 'queued' ||
      batchStatusFilter === 'running');
  const tableBatches = shouldShowActiveBatches
    ? filteredBatches.filter(item => !activeBatchIds.has(item.id))
    : filteredBatches;
  const selectedCases = cases.filter(item => selectedCaseIds.includes(item.id));
  const selectedEnabledCases = selectedCases.filter(item => item.status === 'enabled');
  const allCasesSelected =
    filteredCases.length > 0 && filteredCases.every(item => selectedCaseIds.includes(item.id));
  const canUpdateTestAssets = permissions?.canUpdate ?? true;
  const canDebugTest = permissions?.canDebug ?? true;
  const canStopTestRun = permissions?.canStop ?? true;
  const canViewBatchResults = permissions?.canViewLogs ?? true;
  const canCreateAndRunBatch = canUpdateTestAssets && canDebugTest && canViewBatchResults;
  const canRetestBatch = canDebugTest && canViewBatchResults;
  React.useEffect(() => {
    setSelectedCaseIds(prev => prev.filter(id => cases.some(item => item.id === id)));
    setExpandedCaseIds(prev => prev.filter(id => cases.some(item => item.id === id)));
  }, [cases]);

  React.useEffect(() => {
    if (isGenerationActive) {
      setPendingGenerationCount(null);
    }
  }, [isGenerationActive]);

  React.useEffect(() => {
    const previousStatus = previousGenerationTaskStatusRef.current;
    const currentStatus = generationTask?.status ?? null;
    if (currentStatus === previousStatus) return;

    if (generationTask?.status === 'failed' && generationTask.error) {
      toast.error(generationTaskError);
    } else if (
      generationTask?.status === 'completed' &&
      (previousStatus === 'queued' ||
        previousStatus === 'running' ||
        previousStatus === 'canceling')
    ) {
      setCompletedGenerationTaskId(generationTask.id);
    }

    if (
      previousStatus &&
      (currentStatus === null ||
        currentStatus === 'completed' ||
        currentStatus === 'failed' ||
        currentStatus === 'canceled')
    ) {
      setPendingGenerationCount(null);
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.cases(agentId) });
    }

    previousGenerationTaskStatusRef.current = currentStatus;
  }, [agentId, generationTask, generationTaskError, queryClient, toastT]);

  React.useEffect(() => {
    if (!completedGenerationTaskId) return;
    const timer = window.setTimeout(() => setCompletedGenerationTaskId(null), 6000);
    return () => window.clearTimeout(timer);
  }, [completedGenerationTaskId]);

  React.useEffect(() => {
    const previousStatus = previousScenarioRecognitionTaskStatusRef.current;
    const currentStatus = scenarioRecognitionTask?.status ?? null;
    if (currentStatus === previousStatus) return;

    if (scenarioRecognitionTask?.status === 'failed' && scenarioRecognitionTask.error) {
      toast.error(scenarioRecognitionTaskError);
    } else if (
      scenarioRecognitionTask?.status === 'completed' &&
      (previousStatus === 'queued' ||
        previousStatus === 'running' ||
        previousStatus === 'canceling')
    ) {
      setCompletedScenarioRecognitionTaskId(scenarioRecognitionTask.id);
    }

    if (
      previousStatus &&
      (currentStatus === null ||
        currentStatus === 'completed' ||
        currentStatus === 'failed' ||
        currentStatus === 'canceled')
    ) {
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.scenarios(agentId) });
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.cases(agentId) });
    }

    previousScenarioRecognitionTaskStatusRef.current = currentStatus;
  }, [agentId, queryClient, scenarioRecognitionTask, scenarioRecognitionTaskError]);

  React.useEffect(() => {
    if (!completedScenarioRecognitionTaskId) return;
    const timer = window.setTimeout(() => setCompletedScenarioRecognitionTaskId(null), 6000);
    return () => window.clearTimeout(timer);
  }, [completedScenarioRecognitionTaskId]);

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

  const runSelectedCases = () => {
    if (selectedEnabledCases.length === 0 || createBatch.isPending || executeBatch.isPending) {
      return;
    }
    const name = defaultBatchName(values => t('batches.selectedBatchName', values));
    createBatch.mutate(
      {
        name,
        case_ids: selectedEnabledCases.map(item => item.id),
      },
      {
        onSuccess: response => {
          const batchId = response.data.id;
          executeBatch.mutate(batchId, {
            onSuccess: () => {
              setSelectedCaseIds([]);
              router.push(getBatchResultHref(batchId));
            },
          });
        },
      }
    );
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
          setExpandedCaseIds(prev => prev.filter(id => !ids.includes(id)));
          setDeletingCaseIds([]);
        },
      }
    );
  };

  const toggleCaseExpanded = (caseId: string) => {
    setExpandedCaseIds(prev =>
      prev.includes(caseId) ? prev.filter(id => id !== caseId) : [...prev, caseId]
    );
  };

  const buildRetestName = React.useCallback(
    (batchName: string) => {
      const escapedRetestLabel = commonT('retest').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const suffixPattern = new RegExp(`\\s+${escapedRetestLabel}\\s*\\d*$`, 'u');
      const baseName = batchName.replace(suffixPattern, '').trim() || batchName;
      const escapedBaseName = baseName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const pattern = new RegExp(`^${escapedBaseName}\\s+${escapedRetestLabel}\\s*(\\d*)$`, 'u');
      const maxIndex = batches.reduce((max, item) => {
        const match = item.name.match(pattern);
        if (!match) return max;
        const value = match[1] ? Number(match[1]) : 1;
        return Number.isFinite(value) ? Math.max(max, value) : max;
      }, 1);
      return t('batches.retestName', { name: baseName, index: maxIndex + 1 });
    },
    [batches, commonT, t]
  );

  const confirmRetestBatch = () => {
    if (!retestingBatch) return;
    retestBatch.mutate(
      {
        batchId: retestingBatch.id,
        data: { name: buildRetestName(retestingBatch.name) },
      },
      { onSuccess: () => setRetestingBatch(null) }
    );
  };
  const pageDescription =
    view === 'batches'
      ? t('batches.description')
      : agentDescription ||
        (workflowTestMode === 'task' ? t('taskDescriptionFallback') : t('descriptionFallback'));
  const generateCasesLabel =
    workflowTestMode === 'task' && supportsAttachments
      ? t('actions.generateFileCases')
      : workflowTestMode === 'task'
        ? t('actions.generateTaskCases')
        : t('actions.generateCases');
  const createCaseLabel =
    workflowTestMode === 'task' ? t('actions.createTaskCase') : t('actions.createCase');
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
                <Badge variant="outline">
                  {workflowTestMode === 'task' ? t('mode.task') : t('mode.conversation')}
                </Badge>
              </div>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              {view === 'batches' ? (
                <>
                  {canUpdateTestAssets ? (
                    <Button variant="outline" onClick={() => setSettingsOpen(true)}>
                      <Settings2 className="mr-2 size-4" />
                      {t('actions.judgeSettings')}
                    </Button>
                  ) : null}
                  {canCreateAndRunBatch ? (
                    <Button className="bg-slate-950 text-white hover:bg-slate-800" asChild>
                      <Link href={newBatchHref}>
                        <WandSparkles className="mr-2 size-4" />
                        {t('actions.createBatch')}
                      </Link>
                    </Button>
                  ) : null}
                </>
              ) : scenarios.length > 0 ? (
                <>
                  {canCreateAndRunBatch ? (
                    <Button className="bg-slate-950 text-white hover:bg-slate-800" asChild>
                      <Link href={newBatchHref}>
                        <PlayCircle className="mr-2 size-4" />
                        {t('actions.goTest')}
                      </Link>
                    </Button>
                  ) : null}
                  {canDebugTest ? (
                    <Button variant="outline" onClick={() => setGenerateDialogOpen(true)}>
                      <WandSparkles className="mr-2 size-4" />
                      {generateCasesLabel}
                    </Button>
                  ) : null}
                </>
              ) : (
                <Button
                  variant="outline"
                  disabled={isScenarioRecognitionActive}
                  onClick={() => setRecognizeScenariosOpen(true)}
                >
                  <ScanSearch className="mr-2 size-4" />
                  {isScenarioRecognitionActive
                    ? t('actions.recognizingScenarios')
                    : t('actions.recognizeScenarios')}
                </Button>
              )}
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
              <CardHeader className="flex-row items-start justify-between gap-4">
                <div>
                  <CardTitle>{t('scenarios.title')}</CardTitle>
                  <p className="mt-2 text-sm text-slate-600">{t('scenarios.description')}</p>
                </div>
                <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                  {sceneCards.length > 0 ? (
                    <Button
                      variant="outline"
                      disabled={isScenarioRecognitionActive}
                      onClick={() => setRecognizeScenariosOpen(true)}
                    >
                      <ScanSearch className="mr-2 size-4" />
                      {isScenarioRecognitionActive
                        ? t('actions.recognizingScenarios')
                        : t('actions.rerecognizeScenarios')}
                    </Button>
                  ) : null}
                  {canCancelScenarioRecognition ? (
                    <Button
                      variant="outline"
                      disabled={cancelScenarioRecognitionTask.isPending}
                      onClick={() =>
                        cancelScenarioRecognitionTask.mutate(scenarioRecognitionTask.id)
                      }
                    >
                      <Ban className="mr-2 size-4" />
                      {t('actions.cancelRecognition')}
                    </Button>
                  ) : null}
                  <Button variant="outline" onClick={() => setScenarioDialogOpen(true)}>
                    <SquarePen className="mr-2 size-4" />
                    {t('actions.editScenarios')}
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                {shouldShowScenarioRecognitionBanner ? (
                  <div
                    className={`mb-4 flex items-start gap-3 rounded-xl border px-4 py-4 ${scenarioRecognitionBannerTone.wrapper}`}
                  >
                    <div
                      className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${scenarioRecognitionBannerTone.icon}`}
                    >
                      <ScanSearch className="size-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div
                        className={`text-sm font-semibold ${scenarioRecognitionBannerTone.title}`}
                      >
                        {scenarioRecognitionTitle}
                      </div>
                      <div className="mt-3 space-y-2">
                        <div className="flex items-center justify-between gap-3 text-xs font-medium text-slate-600">
                          <span>{scenarioRecognitionStatusText}</span>
                          <span className="shrink-0">{scenarioRecognitionProgressText}</span>
                        </div>
                        <Progress value={scenarioRecognitionProgress} className="h-2 bg-white/70" />
                      </div>
                      <div className={`mt-2 text-sm ${scenarioRecognitionBannerTone.description}`}>
                        {scenarioRecognitionTask?.status === 'failed'
                          ? t('scenarios.recognitionFailedDescription', {
                              error: scenarioRecognitionTaskError,
                            })
                          : scenarioRecognitionTask?.status === 'canceled'
                            ? t('scenarios.recognitionCanceledDescription')
                            : isScenarioRecognitionCompleted
                              ? t('scenarios.recognitionCompletedDescription', {
                                  recognized: scenarioRecognitionTask?.recognized_count ?? 0,
                                  assigned: scenarioRecognitionTask?.assigned_case_count ?? 0,
                                })
                              : t('scenarios.recognitionProgressDescription')}
                      </div>
                    </div>
                    {canCancelScenarioRecognition ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={cancelScenarioRecognitionTask.isPending}
                        onClick={() =>
                          cancelScenarioRecognitionTask.mutate(scenarioRecognitionTask.id)
                        }
                      >
                        <Ban className="mr-2 size-4" />
                        {t('actions.cancelRecognition')}
                      </Button>
                    ) : null}
                  </div>
                ) : null}
                {sceneCards.length === 0 ? (
                  <div className="flex min-h-[220px] flex-col items-center justify-center rounded-xl border border-dashed border-slate-200 bg-slate-50 px-6 text-center">
                    <div className="rounded-xl bg-white p-3 text-blue-600 shadow-sm">
                      <ScanSearch className="size-5" />
                    </div>
                    <div className="mt-4 font-semibold text-slate-950">
                      {t('scenarios.emptyTitle')}
                    </div>
                    <p
                      className={cn(
                        'mt-2 max-w-md text-sm',
                        isScenarioRecognitionActive ? 'font-medium text-blue-600' : 'text-slate-600'
                      )}
                    >
                      {isScenarioRecognitionActive
                        ? t('scenarios.recognizingDescription')
                        : t('scenarios.emptyDescription')}
                    </p>
                    <Button
                      className="mt-5"
                      disabled={isScenarioRecognitionActive}
                      onClick={() => setRecognizeScenariosOpen(true)}
                    >
                      <ScanSearch className="mr-2 size-4" />
                      {isScenarioRecognitionActive
                        ? t('actions.recognizingScenarios')
                        : t('actions.recognizeScenarios')}
                    </Button>
                  </div>
                ) : (
                  <>
                    <div className="scrollbar-thin flex gap-4 overflow-x-auto pb-2">
                      {sceneCards.map(scene => (
                        <div
                          key={scene.name}
                          className="min-h-[150px] w-[280px] shrink-0 rounded-xl border border-slate-200 bg-slate-50 p-4"
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
                  </>
                )}
              </CardContent>
            </Card>

            <Card id="case-library" className="scroll-mt-6 rounded-2xl">
              <CardHeader className="flex-row items-start justify-between gap-4">
                <div>
                  <CardTitle>
                    {workflowTestMode === 'task' ? t('cases.taskTitle') : t('cases.title')}
                  </CardTitle>
                  <p className="text-sm text-slate-600">
                    {workflowTestMode === 'task'
                      ? t('cases.taskDescription')
                      : t('cases.description')}
                  </p>
                </div>
                <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                  {canUpdateTestAssets ? (
                    <Button variant="outline" onClick={() => setCaseDialogOpen(true)}>
                      <Plus className="mr-2 size-4" />
                      {createCaseLabel}
                    </Button>
                  ) : null}
                </div>
              </CardHeader>
              <CardContent className="p-0">
                {shouldShowGenerationBanner ? (
                  <div
                    className={`flex items-start gap-3 border-y px-6 py-4 ${generationBannerTone.wrapper}`}
                  >
                    <div
                      className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${generationBannerTone.icon}`}
                    >
                      <WandSparkles className="size-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className={`text-sm font-semibold ${generationBannerTone.title}`}>
                        {generationTitle}
                      </div>
                      <div className="mt-3 space-y-2">
                        <div className="flex items-center justify-between gap-3 text-xs font-medium text-slate-600">
                          <span>{generationStatusText}</span>
                          <span className="shrink-0">{generationProgressText}</span>
                        </div>
                        <Progress value={generationProgress} className="h-2 bg-white/70" />
                      </div>
                      <div className={`mt-2 text-sm ${generationBannerTone.description}`}>
                        {displayedGenerationStatus === 'failed'
                          ? t('cases.generationFailedDescription', {
                              created: generationBannerCreated,
                              error: generationTaskError,
                            })
                          : displayedGenerationStatus === 'canceled'
                            ? t('cases.generationCanceledDescription', {
                                created: generationBannerCreated,
                              })
                            : isGenerationCompleted
                              ? t('cases.generationCompletedDescription')
                              : t('cases.generationProgressDescription')}
                      </div>
                    </div>
                    {canCancelGeneration ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={cancelGenerationTask.isPending}
                        onClick={() => cancelGenerationTask.mutate(generationTask.id)}
                      >
                        <Ban className="mr-2 size-4" />
                        {t('actions.cancelGeneration')}
                      </Button>
                    ) : null}
                    {canResumeGeneration && generationTask ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={resumeGenerationTask.isPending}
                        onClick={() => resumeGenerationTask.mutate(generationTask.id)}
                      >
                        <PlayCircle className="mr-2 size-4" />
                        {t('actions.resumeGeneration')}
                      </Button>
                    ) : null}
                    {canDeleteGenerationTask && generationTask ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={deleteGenerationTask.isPending}
                        onClick={() => deleteGenerationTask.mutate(generationTask.id)}
                      >
                        <Trash2 className="mr-2 size-4" />
                        {t('actions.deleteGenerationTask')}
                      </Button>
                    ) : null}
                  </div>
                ) : null}
                <div className="sticky top-0 z-10 grid grid-cols-1 gap-3 border-y border-slate-200 bg-white p-4 md:grid-cols-4">
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
                      {QUESTION_TYPE_OPTIONS.map(item => (
                        <SelectItem key={item.value} value={item.value}>
                          {typeT(item.labelKey)}
                        </SelectItem>
                      ))}
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
                <div className="max-h-[560px] overflow-auto">
                  <Table className="table-fixed" containerClassName="overflow-visible">
                    <TableHeader className="bg-white">
                      <TableRow>
                        <TableHead className="sticky top-0 z-20 w-12 bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
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
                        <TableHead className="sticky top-0 z-20 w-[34%] bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {t('table.questionContent')}
                        </TableHead>
                        <TableHead className="sticky top-0 z-20 w-[13%] bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {t('table.businessScenario')}
                        </TableHead>
                        <TableHead className="sticky top-0 z-20 w-[10%] bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {t('table.questionType')}
                        </TableHead>
                        <TableHead className="sticky top-0 z-20 w-[9%] bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {t('table.status')}
                        </TableHead>
                        <TableHead className="sticky top-0 z-20 w-[12%] bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {workflowTestMode === 'task'
                            ? t('table.taskFocus')
                            : t('table.conversationFocus')}
                        </TableHead>
                        <TableHead className="sticky top-0 z-20 w-[14%] bg-white shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {t('table.generatedAt')}
                        </TableHead>
                        <TableHead className="sticky top-0 z-20 w-[150px] bg-white text-right shadow-[inset_0_-1px_0_rgba(148,163,184,0.28)]">
                          {t('table.operations')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {casesLoading ? (
                        <TableRow>
                          <TableCell colSpan={8} className="h-32 text-center text-slate-500">
                            {t('cases.loading')}
                          </TableCell>
                        </TableRow>
                      ) : cases.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={8} className="h-32 text-center text-slate-500">
                            {t('cases.empty')}
                          </TableCell>
                        </TableRow>
                      ) : filteredCases.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={8} className="h-32 text-center text-slate-500">
                            {t('cases.filteredEmpty')}
                          </TableCell>
                        </TableRow>
                      ) : (
                        filteredCases.map(item => {
                          const turns = item.turns ?? [];
                          const hasMultipleTurns = turns.length > 1;
                          const isExpanded = expandedCaseIds.includes(item.id);

                          return (
                            <TableRow key={item.id}>
                              <TableCell className="py-4">
                                {canUpdateTestAssets ? (
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
                                ) : null}
                              </TableCell>
                              <TableCell className="min-w-0 whitespace-normal py-4 align-top">
                                <div className="line-clamp-2 break-words font-medium text-slate-950">
                                  {item.content}
                                </div>
                                {hasMultipleTurns ? (
                                  <>
                                    <div className="mt-1 text-xs text-slate-500">
                                      {t('cases.turnCount', { count: turns.length })}
                                      <button
                                        type="button"
                                        className="ml-1 font-medium text-blue-600 hover:text-blue-700 hover:underline"
                                        onClick={() => toggleCaseExpanded(item.id)}
                                      >
                                        {isExpanded
                                          ? t('cases.collapseTurns')
                                          : t('cases.expandTurns')}
                                      </button>
                                    </div>
                                    {isExpanded ? (
                                      <div className="mt-3 min-w-0 space-y-3 border-l border-slate-200 pl-4 text-sm text-slate-700">
                                        {turns.slice(1).map((turn, index) => (
                                          <div
                                            key={`${item.id}-turn-${index + 2}`}
                                            className="relative min-w-0"
                                          >
                                            <span className="absolute -left-[19px] top-2 h-2 w-2 rounded-full border border-slate-300 bg-white" />
                                            <div className="flex min-w-0 items-baseline gap-1">
                                              <span className="shrink-0 whitespace-nowrap font-medium text-slate-600">
                                                {t('cases.turnTitle', { index: index + 2 })}
                                              </span>
                                              <span className="min-w-0 truncate">
                                                {turn.content}
                                              </span>
                                            </div>
                                          </div>
                                        ))}
                                      </div>
                                    ) : null}
                                  </>
                                ) : null}
                                {item.turns?.some(turn => turn.attachments?.length) ? (
                                  <div className="mt-1 text-xs text-slate-500">
                                    {hasGeneratedAssets(item)
                                      ? commonT('generatedAttachmentsIncluded')
                                      : commonT('attachmentsIncluded')}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell className="py-4 align-top">
                                <div className="line-clamp-2 break-words">
                                  {item.scenario_id
                                    ? scenariosById.get(item.scenario_id) || commonT('none')
                                    : commonT('none')}
                                </div>
                              </TableCell>
                              <TableCell className="py-4 align-top">
                                {formatQuestionTypeLabel(item.question_type, typeT)}
                              </TableCell>
                              <TableCell className="py-4 align-top">
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
                              <TableCell className="py-4 align-top text-xs text-slate-600">
                                {workflowTestMode === 'task'
                                  ? formatTaskCaseFocus(item, commonT('none'), {
                                      variables: t('cases.focusVariables'),
                                      files: t('cases.focusFiles'),
                                      checks: t('cases.focusChecks'),
                                      criticalChecks: t('cases.focusCriticalChecks'),
                                    })
                                  : formatConversationCaseFocus(item, {
                                      turns: t('cases.focusTurns'),
                                      expectations: t('cases.focusExpectations'),
                                    })}
                              </TableCell>
                              <TableCell className="py-4 align-top text-xs text-slate-600">
                                {new Date(item.created_at).toLocaleString()}
                              </TableCell>
                              <TableCell className="py-3 text-right align-top">
                                {canUpdateTestAssets ? (
                                  <>
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
                                            status:
                                              item.status === 'enabled' ? 'disabled' : 'enabled',
                                            turns: item.turns,
                                          },
                                        })
                                      }
                                    >
                                      {item.status === 'enabled'
                                        ? commonT('disable')
                                        : commonT('enable')}
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
                                  </>
                                ) : null}
                              </TableCell>
                            </TableRow>
                          );
                        })
                      )}
                    </TableBody>
                  </Table>
                </div>
                {canUpdateTestAssets && selectedCaseIds.length > 0 ? (
                  <div className="sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 bg-white px-4 py-3 shadow-[0_-8px_24px_rgba(15,23,42,0.08)]">
                    <div className="min-w-0">
                      <div className="text-sm text-slate-600">
                        {t('cases.selectedCount', { count: selectedCaseIds.length })}
                      </div>
                      {selectedEnabledCases.length !== selectedCaseIds.length ? (
                        <div className="mt-0.5 text-xs text-slate-500">
                          {t('cases.selectedRunnableCount', { count: selectedEnabledCases.length })}
                        </div>
                      ) : null}
                    </div>
                    <div className="flex flex-wrap items-center justify-end gap-2">
                      <Button
                        size="sm"
                        disabled={
                          selectedEnabledCases.length === 0 ||
                          createBatch.isPending ||
                          executeBatch.isPending
                        }
                        onClick={runSelectedCases}
                      >
                        {createBatch.isPending || executeBatch.isPending ? (
                          <Loader2 className="mr-1 size-4 animate-spin" />
                        ) : (
                          <PlayCircle className="mr-1 size-4" />
                        )}
                        {t('cases.batchRun')}
                      </Button>
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
              {shouldShowActiveBatches ? (
                <div className="border-t border-slate-200 p-4">
                  <div className="space-y-2">
                    {activeBatches.map(activeBatch => (
                      <div
                        key={activeBatch.id}
                        className="rounded-xl border border-blue-100 bg-blue-50/50 px-4 py-3"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-4">
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-3">
                              <div className="truncate text-sm font-medium text-slate-950">
                                {t('batches.activeTitle', { name: activeBatch.name })}
                              </div>
                              <Badge className="bg-blue-100 text-blue-700">
                                {activeBatch.status === 'running' ? (
                                  <Loader2 className="mr-1 size-3 animate-spin" />
                                ) : (
                                  <Clock className="mr-1 size-3" />
                                )}
                                {batchStatusT(activeBatch.status)}
                              </Badge>
                              <Progress
                                value={batchProgressValue(activeBatch)}
                                className="h-1.5 min-w-[180px] flex-1 bg-white sm:max-w-xl"
                              />
                              <span className="shrink-0 text-xs text-slate-500">
                                {t('batches.activeProgress', {
                                  done: batchFinishedCount(activeBatch),
                                  total: activeBatch.case_count,
                                })}
                              </span>
                            </div>
                          </div>
                          <div className="flex shrink-0 items-center gap-2">
                            {canViewBatchResults ? (
                              <Button variant="outline" size="sm" asChild>
                                <Link href={getBatchResultHref(activeBatch.id)}>
                                  {t('batchActions.viewProgress')}
                                </Link>
                              </Button>
                            ) : null}
                            {canStopTestRun ? (
                              <Button
                                variant="ghost"
                                size="sm"
                                disabled={cancelBatch.isPending}
                                onClick={() => cancelBatch.mutate(activeBatch.id)}
                              >
                                <Ban className="mr-1 size-4" />
                                {commonT('cancel')}
                              </Button>
                            ) : null}
                          </div>
                        </div>
                      </div>
                    ))}
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
                        <TableCell>
                          <div className="flex items-center justify-end gap-2">
                            {canDebugTest && batch.status === 'queued' ? (
                              <Button
                                variant="link"
                                size="sm"
                                className="h-8 px-2"
                                disabled={executeBatch.isPending}
                                onClick={() => executeBatch.mutate(batch.id)}
                              >
                                {t('batchActions.start')}
                              </Button>
                            ) : null}
                            {canViewBatchResults && batch.status === 'queued' ? (
                              <Button variant="link" size="sm" className="h-8 px-2" asChild>
                                <Link href={getBatchResultHref(batch.id)}>
                                  {t('batchActions.viewDetail')}
                                </Link>
                              </Button>
                            ) : null}
                            {canViewBatchResults && batch.status === 'running' ? (
                              <Button variant="link" size="sm" className="h-8 px-2" asChild>
                                <Link href={getBatchResultHref(batch.id)}>
                                  {t('batchActions.viewProgress')}
                                </Link>
                              </Button>
                            ) : null}
                            {canStopTestRun &&
                            (batch.status === 'queued' || batch.status === 'running') ? (
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 px-2"
                                disabled={cancelBatch.isPending}
                                onClick={() => cancelBatch.mutate(batch.id)}
                              >
                                <Ban className="size-4" />
                                {commonT('cancel')}
                              </Button>
                            ) : canViewBatchResults && batch.status !== 'queued' ? (
                              <Button variant="link" size="sm" className="h-8 px-2" asChild>
                                <Link href={getBatchResultHref(batch.id)}>
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
                                {canStopTestRun &&
                                (batch.status === 'queued' || batch.status === 'running') ? (
                                  <DropdownMenuItem
                                    className="text-red-600"
                                    disabled={cancelBatch.isPending}
                                    onSelect={() => cancelBatch.mutate(batch.id)}
                                  >
                                    {commonT('cancelTest')}
                                  </DropdownMenuItem>
                                ) : canRetestBatch ? (
                                  <DropdownMenuItem
                                    disabled={retestBatch.isPending}
                                    onSelect={() => setRetestingBatch(batch)}
                                  >
                                    {commonT('retest')}
                                  </DropdownMenuItem>
                                ) : null}
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </div>
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
        mode={workflowTestMode}
      />
      <CaseDialog
        agentId={agentId}
        scenarios={scenarioOptions}
        open={caseDialogOpen}
        onOpenChange={setCaseDialogOpen}
        supportsAttachments={supportsAttachments}
        attachmentAcceptExt={attachmentAcceptExt}
        mode={workflowTestMode}
        workflowDraft={workflowDraft}
      />
      <CaseDialog
        agentId={agentId}
        scenarios={scenarioOptions}
        caseItem={editingCase}
        open={!!editingCaseId}
        onOpenChange={open => {
          if (!open) setEditingCaseId(null);
        }}
        supportsAttachments={supportsAttachments}
        attachmentAcceptExt={attachmentAcceptExt}
        mode={workflowTestMode}
        workflowDraft={workflowDraft}
      />
      <GenerateCasesDialog
        agentId={agentId}
        scenarios={scenarioOptions}
        open={generateDialogOpen}
        onOpenChange={setGenerateDialogOpen}
        mode={workflowTestMode}
        supportsGeneratedFiles={supportsAttachments}
        requiresCurrentTurnFiles={requiresCurrentTurnFiles}
        onGenerationStart={setPendingGenerationCount}
        onGenerationCreateFailed={() => setPendingGenerationCount(null)}
      />
      <ConfirmDialog
        variant="danger"
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
        onConfirm={confirmDeleteCases}
      />
      <ConfirmDialog
        open={Boolean(retestingBatch)}
        onOpenChange={open => {
          if (!open && !retestBatch.isPending) setRetestingBatch(null);
        }}
        title={t('batches.retestConfirmTitle')}
        description={
          retestingBatch
            ? t('batches.retestConfirmDescription', {
                name: retestingBatch.name,
                count: retestingBatch.case_count,
              })
            : ''
        }
        confirmText={t('batches.retestConfirmButton')}
        cancelText={commonT('cancel')}
        loading={retestBatch.isPending}
        contentClassName="max-w-2xl rounded-2xl"
        footerClassName="justify-end bg-white px-8 py-6"
        cancelClassName="border border-slate-200 bg-white hover:bg-slate-50"
        confirmClassName="bg-slate-950 text-white hover:bg-slate-800"
        onConfirm={confirmRetestBatch}
      />
    </div>
  );
}
