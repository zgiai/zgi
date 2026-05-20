'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ArrowLeft, CheckCircle2, Circle, LibraryBig, ListChecks, Play, Tags } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  useCreateWorkflowTestBatch,
  useExecuteWorkflowTestBatch,
  useWorkflowTestBatches,
  useWorkflowTestCases,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { WorkflowTestCase } from '@/services/types/workflow-test';
import { formatQuestionTypeLabel } from './question-type';

interface CreateBatchPageProps {
  agentId: string;
  agentName?: string;
  agentDescription?: string;
}

type ScopeMode = 'all' | 'scenarios' | 'cases';

function defaultBatchName(template: (values: { date: string; time: string }) => string) {
  const now = new Date();
  const yyyy = now.getFullYear();
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  const dd = String(now.getDate()).padStart(2, '0');
  const hh = String(now.getHours()).padStart(2, '0');
  const min = String(now.getMinutes()).padStart(2, '0');
  return template({ date: `${yyyy}-${mm}-${dd}`, time: `${hh}:${min}` });
}

function questionTitle(item: WorkflowTestCase) {
  return item.turns?.[0]?.content || item.content;
}

function unique(values: string[]) {
  return Array.from(new Set(values));
}

export function CreateBatchPage({ agentId, agentName, agentDescription }: CreateBatchPageProps) {
  const t = useT('agents.workflowTest.createBatchPage');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const batchStatusT = useT('agents.workflowTest.batchStatus');
  const router = useRouter();
  const { data: casesData, isLoading: casesLoading } = useWorkflowTestCases(agentId);
  const { data: scenariosData } = useWorkflowTestScenarios(agentId);
  const { data: batchesData } = useWorkflowTestBatches(agentId);
  const createBatch = useCreateWorkflowTestBatch(agentId);
  const executeBatch = useExecuteWorkflowTestBatch(agentId);
  const [name, setName] = React.useState(() => defaultBatchName(values => t('defaultName', values)));
  const [scopeMode, setScopeMode] = React.useState<ScopeMode>('all');
  const [selectedScenarioIds, setSelectedScenarioIds] = React.useState<string[]>([]);
  const [selectedCaseIds, setSelectedCaseIds] = React.useState<string[]>([]);

  const cases = React.useMemo(() => casesData?.data?.items ?? [], [casesData]);
  const enabledCases = React.useMemo(
    () => cases.filter(item => item.status === 'enabled'),
    [cases]
  );
  const scenarios = React.useMemo(() => scenariosData?.data?.items ?? [], [scenariosData]);
  const batches = React.useMemo(() => batchesData?.data?.items ?? [], [batchesData]);
  const hasActiveBatch = batches.some(item => item.status === 'queued' || item.status === 'running');
  const scenarioById = React.useMemo(
    () => new Map(scenarios.map(item => [item.id, item])),
    [scenarios]
  );

  const selectedCases = React.useMemo(() => {
    if (scopeMode === 'all') {
      return enabledCases;
    }
    if (scopeMode === 'scenarios') {
      const selected = new Set(selectedScenarioIds);
      return enabledCases.filter(item => item.scenario_id && selected.has(item.scenario_id));
    }
    const selected = new Set(selectedCaseIds);
    return enabledCases.filter(item => selected.has(item.id));
  }, [enabledCases, scopeMode, selectedCaseIds, selectedScenarioIds]);

  const selectedScenarioCount = React.useMemo(
    () => unique(selectedCases.map(item => item.scenario_id).filter(Boolean) as string[]).length,
    [selectedCases]
  );
  const unassignedSelectedCount = selectedCases.filter(item => !item.scenario_id).length;
  const canSubmit = name.trim() && selectedCases.length > 0 && !createBatch.isPending && !executeBatch.isPending;

  React.useEffect(() => {
    setSelectedScenarioIds(prev => prev.filter(id => scenarios.some(item => item.id === id)));
  }, [scenarios]);

  React.useEffect(() => {
    setSelectedCaseIds(prev => prev.filter(id => enabledCases.some(item => item.id === id)));
  }, [enabledCases]);

  const scopeOptions = [
    {
      value: 'all' as const,
      title: t('scope.all.title'),
      description: t('scope.all.description'),
      icon: ListChecks,
      disabled: false,
    },
    {
      value: 'scenarios' as const,
      title: t('scope.scenarios.title'),
      description: t('scope.scenarios.description'),
      icon: Tags,
      disabled: scenarios.length === 0,
    },
    {
      value: 'cases' as const,
      title: t('scope.cases.title'),
      description: t('scope.cases.description'),
      icon: LibraryBig,
      disabled: false,
    },
  ];

  const createAndExecute = () => {
    if (!canSubmit) return;
    createBatch.mutate(
      {
        name: name.trim(),
        case_ids: selectedCases.map(item => item.id),
        workflow_version_mode: 'draft',
      },
      {
        onSuccess: response => {
          const batchId = response.data.id;
          executeBatch.mutate(batchId, {
            onSuccess: () => {
              router.push(`/console/agents/${agentId}/batch-test/${batchId}`);
            },
            onError: () => {
              router.push(`/console/agents/${agentId}/batch-test/${batchId}`);
            },
          });
        },
      }
    );
  };

  return (
    <div className="min-h-full bg-slate-50 px-8 py-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-6">
        <div className="text-sm text-slate-500">
          {t('breadcrumb', { agentName: agentName || commonT('agentFallback') })}
        </div>

        <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <Button variant="ghost" className="-ml-3 mb-4" asChild>
            <Link href={`/console/agents/${agentId}/batch-test/batches`}>
              <ArrowLeft className="mr-2 size-4" />
              {t('back')}
            </Link>
          </Button>
          <h1 className="text-2xl font-semibold text-slate-950">{t('title')}</h1>
          <p className="mt-3 max-w-3xl text-sm text-slate-600">{t('description')}</p>
          <div className="mt-4 flex items-center gap-2 text-sm text-slate-500">
            <Badge variant="outline">{commonT('chatWorkflow')}</Badge>
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
            <span>{commonT('currentDraftSnapshot')}</span>
            {agentDescription ? <span className="truncate text-slate-400">{agentDescription}</span> : null}
          </div>
        </section>

        <Card className="rounded-2xl">
          <CardContent className="space-y-7 p-6">
            <div className="space-y-2">
              <label htmlFor="workflow-test-batch-name" className="text-sm font-semibold text-slate-950">
                {t('nameLabel')}
              </label>
              <Input
                id="workflow-test-batch-name"
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder={t('namePlaceholder')}
              />
            </div>

            <div className="space-y-3">
              <div className="text-sm font-semibold text-slate-950">{t('scopeTitle')}</div>
              <div className="grid gap-3 md:grid-cols-3">
                {scopeOptions.map(option => {
                  const Icon = option.icon;
                  const active = scopeMode === option.value;
                  return (
                    <button
                      key={option.value}
                      type="button"
                      disabled={option.disabled}
                      onClick={() => setScopeMode(option.value)}
                      className={cn(
                        'flex min-h-[84px] items-start gap-3 rounded-xl border bg-white p-4 text-left transition',
                        active
                          ? 'border-blue-500 bg-blue-50 ring-1 ring-blue-500'
                          : 'border-slate-200 hover:bg-slate-50',
                        option.disabled && 'cursor-not-allowed opacity-50 hover:bg-white'
                      )}
                    >
                      <span
                        className={cn(
                          'mt-1 flex size-5 items-center justify-center rounded-full',
                          active ? 'text-blue-600' : 'text-slate-500'
                        )}
                      >
                        {active ? <CheckCircle2 className="size-5" /> : <Circle className="size-5" />}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex items-center gap-2 font-semibold text-slate-950">
                          <Icon className="size-4" />
                          {option.title}
                        </span>
                        <span className="mt-1 block text-sm text-slate-500">{option.description}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>

            {scopeMode === 'scenarios' ? (
              <div className="space-y-3">
                <div className="text-sm font-semibold text-slate-950">{t('scenarioSelectTitle')}</div>
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {scenarios.map(scenario => {
                    const caseCount = enabledCases.filter(item => item.scenario_id === scenario.id).length;
                    const selected = selectedScenarioIds.includes(scenario.id);
                    return (
                      <button
                        key={scenario.id}
                        type="button"
                        disabled={caseCount === 0}
                        onClick={() =>
                          setSelectedScenarioIds(prev =>
                            selected ? prev.filter(id => id !== scenario.id) : [...prev, scenario.id]
                          )
                        }
                        className={cn(
                          'rounded-xl border p-4 text-left transition',
                          selected
                            ? 'border-blue-500 bg-blue-50 ring-1 ring-blue-500'
                            : 'border-slate-200 bg-white hover:bg-slate-50',
                          caseCount === 0 && 'cursor-not-allowed opacity-50 hover:bg-white'
                        )}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="font-semibold text-slate-950">{scenario.name}</div>
                          <Badge variant="outline">{t('caseCount', { count: caseCount })}</Badge>
                        </div>
                        <p className="mt-2 line-clamp-2 text-sm text-slate-500">{scenario.description}</p>
                      </button>
                    );
                  })}
                </div>
                {selectedCases.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50 px-4 py-6 text-center text-sm text-slate-500">
                    {t('emptyScenarioSelection')}
                  </div>
                ) : null}
              </div>
            ) : null}

            {scopeMode === 'cases' ? (
              <div className="rounded-xl border border-slate-200">
                <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
                  <div>
                    <div className="font-semibold text-slate-950">{t('caseSelectTitle')}</div>
                    <div className="text-sm text-slate-500">
                      {t('selectedProgress', {
                        selected: selectedCaseIds.length,
                        total: enabledCases.length,
                      })}
                    </div>
                  </div>
                  <label className="flex items-center gap-2 text-sm text-slate-700">
                    <Checkbox
                      checked={enabledCases.length > 0 && selectedCaseIds.length === enabledCases.length}
                      onCheckedChange={checked =>
                        setSelectedCaseIds(checked ? enabledCases.map(item => item.id) : [])
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
                      const selected = selectedCaseIds.includes(item.id);
                      const scenarioName = item.scenario_id
                        ? scenarioById.get(item.scenario_id)?.name || commonT('none')
                        : commonT('none');
                      return (
                        <label
                          key={item.id}
                          className="flex cursor-pointer items-start gap-3 border-b border-slate-100 px-4 py-3 last:border-b-0 hover:bg-slate-50"
                        >
                          <Checkbox
                            checked={selected}
                            onCheckedChange={checked =>
                              setSelectedCaseIds(prev =>
                                checked
                                  ? unique([...prev, item.id])
                                  : prev.filter(id => id !== item.id)
                              )
                            }
                          />
                          <span className="min-w-0 flex-1">
                            <span className="block text-sm font-medium text-slate-950">
                              {questionTitle(item)}
                            </span>
                            <span className="mt-2 flex flex-wrap items-center gap-2">
                              <Badge variant="outline">{scenarioName}</Badge>
                              <Badge variant="outline">
                                {formatQuestionTypeLabel(item.question_type, typeT)}
                              </Badge>
                              {item.turns?.length > 1 ? (
                                <Badge className="bg-blue-50 text-blue-700">
                                  {t('turnCount', { count: item.turns.length })}
                                </Badge>
                              ) : null}
                            </span>
                          </span>
                        </label>
                      );
                    })
                  )}
                </div>
              </div>
            ) : null}

            <div className="space-y-3 border-t border-slate-200 pt-6">
              <div className="text-sm font-semibold text-slate-950">{t('previewTitle')}</div>
              <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-4 text-sm text-slate-700">
                {t('preview', {
                  count: selectedCases.length,
                  scenarioCount: selectedScenarioCount,
                })}
                {unassignedSelectedCount > 0 ? (
                  <span className="ml-2 text-slate-500">
                    {t('unassignedPreview', { count: unassignedSelectedCount })}
                  </span>
                ) : null}
              </div>
            </div>

            <div className="space-y-3">
              <div className="text-sm font-semibold text-slate-950">{t('checksTitle')}</div>
              <div className="space-y-2 text-sm">
                <div className="flex items-center gap-2 text-emerald-700">
                  <CheckCircle2 className="size-4" />
                  {enabledCases.length > 0 ? t('checks.hasCases') : t('checks.noCases')}
                </div>
                <div className="flex items-center gap-2 text-emerald-700">
                  <CheckCircle2 className="size-4" />
                  {t('checks.workflowRunnable')}
                </div>
                {hasActiveBatch ? (
                  <div className="flex items-center gap-2 text-amber-600">
                    <CheckCircle2 className="size-4" />
                    {t('checks.activeBatch')}
                  </div>
                ) : null}
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <Button variant="outline" asChild>
                <Link href={`/console/agents/${agentId}/batch-test/batches`}>{commonT('cancel')}</Link>
              </Button>
              <Button
                className="bg-slate-950 text-white hover:bg-slate-800"
                disabled={!canSubmit}
                onClick={createAndExecute}
              >
                <Play className="mr-2 size-4" />
                {createBatch.isPending || executeBatch.isPending ? t('submitting') : t('submit')}
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-2xl">
          <CardHeader>
            <CardTitle>{t('selectedCasesTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('table.question')}</TableHead>
                  <TableHead>{t('table.scenario')}</TableHead>
                  <TableHead>{t('table.type')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {casesLoading ? (
                  <TableRow>
                    <TableCell colSpan={3} className="h-24 text-center text-slate-500">
                      {commonT('loading')}
                    </TableCell>
                  </TableRow>
                ) : selectedCases.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={3} className="h-24 text-center text-slate-500">
                      {t('emptySelection')}
                    </TableCell>
                  </TableRow>
                ) : (
                  selectedCases.map(item => (
                    <TableRow key={item.id}>
                      <TableCell className="max-w-3xl">
                        <div className="font-medium text-slate-950">{questionTitle(item)}</div>
                        {item.turns?.length > 1 ? (
                          <div className="mt-1 text-xs text-slate-500">
                            {t('turnCount', { count: item.turns.length })}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        {item.scenario_id
                          ? scenarioById.get(item.scenario_id)?.name || commonT('none')
                          : commonT('none')}
                      </TableCell>
                      <TableCell>{formatQuestionTypeLabel(item.question_type, typeT)}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
