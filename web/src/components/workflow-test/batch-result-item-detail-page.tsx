'use client';

import * as React from 'react';
import Link from 'next/link';
import { ArrowLeft, ChevronLeft, ChevronRight, ExternalLink, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useWorkflowTestBatchItems,
  useWorkflowTestBatches,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useWorkflowChatMessages, useWorkflowRunDetail, useWorkflowRunsInfinite } from '@/hooks';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatWorkflowElapsedMs } from '@/utils/format';
import type { WorkflowTestBatchItem } from '@/services/types/workflow-test';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';
import { formatQuestionTypeLabel } from './question-type';

interface BatchResultItemDetailPageProps {
  agentId: string;
  batchId: string;
  itemId: string;
  agentName?: string;
}

function itemStatusClass(status: string) {
  if (status === 'passed') return 'bg-emerald-50 text-emerald-700 border-emerald-200';
  if (status === 'failed') return 'bg-red-50 text-red-700 border-red-200';
  if (status === 'review') return 'bg-amber-50 text-amber-700 border-amber-200';
  if (status === 'running') return 'bg-blue-50 text-blue-700 border-blue-200';
  return 'bg-slate-100 text-slate-500 border-slate-200';
}

function itemStatusLabel(status: string, none: string) {
  const map: Record<string, string> = {
    pending: '待开始',
    running: '执行中',
    passed: '通过',
    failed: '不通过',
    review: '需复核',
    canceled: '已取消',
  };
  return map[status] || status || none;
}

function stringifyOutput(outputs: Record<string, unknown>, none: string) {
  if (!outputs || Object.keys(outputs).length === 0) {
    return none;
  }
  const preferredKeys = ['answer', 'text', 'result', 'output'];
  for (const key of preferredKeys) {
    const value = outputs[key];
    if (typeof value === 'string' && value.trim()) {
      return value;
    }
  }
  return JSON.stringify(outputs, null, 2);
}

function stringifyValue(value: unknown, none: string) {
  if (value === undefined || value === null || value === '') {
    return none;
  }
  if (typeof value === 'string') {
    return value;
  }
  return JSON.stringify(value, null, 2);
}

function stringifyJson(value: unknown, none: string) {
  if (value === undefined || value === null || value === '') {
    return none;
  }
  return JSON.stringify(value, null, 2);
}

function hasAttachments(item: WorkflowTestBatchItem) {
  return item.case_snapshot.turns?.some(turn => turn.attachments?.length) ?? false;
}

function formatResponseTime(item: WorkflowTestBatchItem, none: string) {
  const outputs = item.outputs || {};
  const elapsedKeys = ['elapsed_time', 'elapsed_ms', 'duration_ms', 'latency_ms', 'response_time_ms'];
  for (const key of elapsedKeys) {
    const value = outputs[key];
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
      return formatWorkflowElapsedMs(value);
    }
  }
  return none;
}

function normalizeJudgeScore(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  if (value <= 1) {
    return value * 5;
  }
  if (value <= 5) {
    return value;
  }
  if (value <= 100) {
    return value / 20;
  }
  return value;
}

function extractExecutionNodes(outputs: Record<string, unknown>) {
  const nodeResults = outputs?.node_results;
  if (!nodeResults || typeof nodeResults !== 'object') {
    return [];
  }
  return Object.entries(nodeResults as Record<string, unknown>)
    .map(([id, raw]) => {
      const value = raw as Record<string, unknown>;
      return {
        id,
        status: typeof value.status === 'string' ? value.status : 'unknown',
        error: typeof value.error === 'string' ? value.error : '',
        startTime: typeof value.startTime === 'string' ? value.startTime : '',
        endTime: typeof value.endTime === 'string' ? value.endTime : '',
      };
    })
    .sort((a, b) => a.startTime.localeCompare(b.startTime));
}

function formatExecutionStepLabel(nodeId: string) {
  if (!nodeId) return nodeId;
  if (nodeId.includes('start')) return '开始节点';
  if (nodeId.includes('answer') || nodeId.includes('reply')) return '回复生成';
  if (nodeId.includes('llm')) return 'LLM';
  if (nodeId.includes('branch')) return '分支节点';
  return nodeId;
}

function buildTurnTitle(role: string, index: number) {
  const roleLabel = role || 'user';
  return `第 ${index + 1} 轮 · ${roleLabel}`;
}

function getMessageText(message: WorkflowChatMessageItem | null | undefined, none: string) {
  if (!message) return none;
  const answer = typeof message.answer === 'string' ? message.answer.trim() : '';
  return answer || none;
}

function getMessageQuery(message: WorkflowChatMessageItem | null | undefined, none: string) {
  if (!message) return none;
  const query = typeof message.query === 'string' ? message.query.trim() : '';
  return query || none;
}

type RawViewPayload = {
  title: string;
  content: Record<string, unknown> | unknown[] | string | number | boolean | null;
};

export function BatchResultItemDetailPage({
  agentId,
  batchId,
  itemId,
  agentName,
}: BatchResultItemDetailPageProps) {
  const t = useT('agents.workflowTest.detail');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const {
    data: batchesData,
    isLoading: batchesLoading,
    error: batchesError,
    refetch: refetchBatches,
  } = useWorkflowTestBatches(agentId);
  const {
    data: itemsData,
    isLoading: itemsLoading,
    error: itemsError,
    refetch: refetchItems,
  } = useWorkflowTestBatchItems(agentId, batchId);
  const { data: scenariosData } = useWorkflowTestScenarios(agentId);

  const batches = batchesData?.data?.items ?? [];
  const batch = batches.find(item => item.id === batchId);
  const items = itemsData?.data?.items ?? [];
  const scenarios = scenariosData?.data?.items ?? [];
  const itemIndex = items.findIndex(item => item.id === itemId);
  const selectedItem = itemIndex >= 0 ? items[itemIndex] : null;
  const { detail: runDetail } = useWorkflowRunDetail(
    { agentId, runId: selectedItem?.workflow_run_id ?? null },
    { enabled: Boolean(selectedItem?.workflow_run_id) }
  );
  const {
    pages: runPages,
    isLoading: runListLoading,
    isFetching: runListFetching,
  } = useWorkflowRunsInfinite(
    { agentId, limit: 50, query: { triggered_from: 'debugging' } },
    { enabled: Boolean(selectedItem?.workflow_run_id), refetchOnWindowFocus: false }
  );
  const runListConversationId = React.useMemo(() => {
    if (!selectedItem?.workflow_run_id) return null;
    for (const page of runPages) {
      const matched = page.find(run => run.id === selectedItem.workflow_run_id);
      if (matched?.conversation_id) {
        return matched.conversation_id;
      }
    }
    return null;
  }, [runPages, selectedItem?.workflow_run_id]);
  const conversationId = runDetail?.conversation_id ?? runListConversationId ?? null;
  const {
    messages: chatMessages,
    isLoading: chatMessagesLoading,
    isFetching: chatMessagesFetching,
  } = useWorkflowChatMessages(
    { agentId, conversationId, page: 1, limit: 100 },
    { enabled: Boolean(conversationId), refetchOnWindowFocus: false }
  );
  const previousItem = itemIndex > 0 ? items[itemIndex - 1] : null;
  const nextItem = itemIndex >= 0 && itemIndex < items.length - 1 ? items[itemIndex + 1] : null;
  const [rawView, setRawView] = React.useState<RawViewPayload | null>(null);
  const isLoading = batchesLoading || itemsLoading;
  const error = batchesError || itemsError;
  const scenarioNameById = React.useMemo(
    () => new Map(scenarios.map(scenario => [scenario.id, scenario.name])),
    [scenarios]
  );
  const getScenarioName = React.useCallback(
    (item: WorkflowTestBatchItem) => {
      const scenarioId = item.case_snapshot.scenario_id;
      return scenarioId ? scenarioNameById.get(scenarioId) || commonT('none') : commonT('none');
    },
    [commonT, scenarioNameById]
  );
  const itemPosition = itemIndex >= 0 ? `${itemIndex + 1} / ${items.length}` : '';
  const conversationMessages = React.useMemo(
    () => {
      const runId = selectedItem?.workflow_run_id;
      const matchedMessages = runId
        ? chatMessages.filter(message => message.workflow_run_id === runId)
        : chatMessages;
      const sourceMessages = matchedMessages.length > 0 ? matchedMessages : chatMessages;
      return [...sourceMessages].sort((a, b) => {
        const aTime = typeof a.created_at === 'number' ? a.created_at : 0;
        const bTime = typeof b.created_at === 'number' ? b.created_at : 0;
        return aTime - bTime;
      });
    },
    [chatMessages, selectedItem?.workflow_run_id]
  );

  if (isLoading) {
    return (
      <div className="min-h-full bg-slate-50 p-8">
        <div className="mx-auto max-w-[1600px] space-y-6">
          <Skeleton className="h-48 rounded-2xl" />
          <Skeleton className="h-[720px] rounded-2xl" />
        </div>
      </div>
    );
  }

  if (error || !batch || !selectedItem) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="w-full max-w-xl">
          <Alert variant="destructive">
            <AlertTitle>{t('loadFailedTitle')}</AlertTitle>
            <AlertDescription>
              {error ? getErrorMessage(error) || t('loadFailedFallback') : t('notFound')}
            </AlertDescription>
          </Alert>
          <Button
            className="mt-4"
            onClick={() => {
              void refetchBatches();
              void refetchItems();
            }}
          >
            <RefreshCcw className="mr-2 size-4" />
            {t('retry')}
          </Button>
        </div>
      </div>
    );
  }

  const outputs = selectedItem.outputs || {};
  const executionNodes = extractExecutionNodes(outputs);
  const judgeScore = normalizeJudgeScore(selectedItem.judge_confidence || 0);
  const judgeScoreText = `${Number.isInteger(judgeScore) ? judgeScore.toFixed(0) : judgeScore.toFixed(1)} / 5`;
  const questionSnapshot = selectedItem.case_snapshot.content || commonT('none');
  const expectedResult = selectedItem.case_snapshot.expected_result || commonT('none');
  const reasonText =
    selectedItem.error || selectedItem.judge_reason || (selectedItem.status === 'passed' ? '本轮测试已通过。' : commonT('none'));
  const openRawView = (title: string, content: RawViewPayload['content']) => setRawView({ title, content });
  const isConversationLoading =
    chatMessagesLoading || chatMessagesFetching || (!conversationId && (runListLoading || runListFetching));

  return (
    <div className="min-h-full bg-slate-50 px-8 py-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-6">
        <div className="text-sm text-slate-500">
          {t('breadcrumb', { agentName: agentName || commonT('agentFallback') })}
        </div>

        <Card className="rounded-2xl border-slate-200">
          <CardContent className="p-6">
            <div className="mb-4 flex items-center justify-between gap-4">
              <Button variant="ghost" size="sm" asChild className="px-0 text-slate-600">
                <Link href={`/console/agents/${agentId}/batch-test/${batchId}`}>
                  <ArrowLeft className="mr-2 size-4" />
                  {t('back')}
                </Link>
              </Button>
              <div className="flex items-center gap-3">
                {itemPosition ? <div className="text-sm text-slate-500">{itemPosition}</div> : null}
                <div className="flex items-center gap-2">
                  {previousItem ? (
                    <Button variant="outline" size="sm" asChild>
                      <Link href={`/console/agents/${agentId}/batch-test/${batchId}/items/${previousItem.id}`}>
                        <ChevronLeft className="mr-1 size-4" />
                        上一条
                      </Link>
                    </Button>
                  ) : (
                    <Button variant="outline" size="sm" disabled>
                      <ChevronLeft className="mr-1 size-4" />
                      上一条
                    </Button>
                  )}
                  {nextItem ? (
                    <Button variant="outline" size="sm" asChild>
                      <Link href={`/console/agents/${agentId}/batch-test/${batchId}/items/${nextItem.id}`}>
                        下一条
                        <ChevronRight className="ml-1 size-4" />
                      </Link>
                    </Button>
                  ) : (
                    <Button variant="outline" size="sm" disabled>
                      下一条
                      <ChevronRight className="ml-1 size-4" />
                    </Button>
                  )}
                </div>
              </div>
            </div>

            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-3">
                  <h1 className="text-2xl font-semibold text-slate-950">{selectedItem.case_snapshot.content}</h1>
                  <Badge className={itemStatusClass(selectedItem.status)}>
                    {itemStatusLabel(selectedItem.status, commonT('none'))}
                  </Badge>
                </div>
                <div className="mt-3 flex flex-wrap gap-2 text-sm text-slate-500">
                  <Badge variant="outline">{getScenarioName(selectedItem)}</Badge>
                  <Badge variant="outline">
                    {formatQuestionTypeLabel(selectedItem.case_snapshot.question_type, typeT)}
                  </Badge>
                  <Badge variant="outline">{formatResponseTime(selectedItem, commonT('none'))}</Badge>
                </div>
              </div>
            </div>

            <div className="mt-5 grid grid-cols-1 gap-4 rounded-2xl border border-slate-200 bg-slate-50 p-4 text-sm text-slate-700 md:grid-cols-3">
              <div>
                <div className="text-slate-500">{t('table.scenario')}</div>
                <div className="mt-1 font-semibold text-slate-950">{getScenarioName(selectedItem)}</div>
              </div>
              <div>
                <div className="text-slate-500">{t('table.testResult')}</div>
                <div className="mt-1">
                  <Badge className={itemStatusClass(selectedItem.status)}>
                    {itemStatusLabel(selectedItem.status, commonT('none'))}
                  </Badge>
                </div>
              </div>
              <div>
                <div className="text-slate-500">{t('table.responseTime')}</div>
                <div className="mt-1 font-semibold text-slate-950">
                  {formatResponseTime(selectedItem, commonT('none'))}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-2xl border-slate-200">
          <CardContent className="space-y-6 p-6">
            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('questionSnapshot')}</h2>
              <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                {questionSnapshot}
              </div>
              <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                <div className="mb-2 text-xs font-medium text-slate-500">{t('expectedResult')}</div>
                <div className="whitespace-pre-wrap">{expectedResult}</div>
              </div>
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-lg font-semibold text-slate-950">对话内容</h2>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => openRawView('原始输出', outputs as Record<string, unknown>)}
                >
                  <ExternalLink className="mr-2 size-4" />
                  原始数据
                </Button>
              </div>
              {isConversationLoading ? (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-6 text-sm text-slate-500">
                  正在加载会话记录...
                </div>
              ) : conversationMessages.length > 0 ? (
                <div className="space-y-4">
                  {conversationMessages.map((message, index) => {
                    const turnSnapshot = selectedItem.case_snapshot.turns?.[index];
                    return (
                      <div key={message.id} className="rounded-2xl border border-slate-200 bg-slate-50 p-5">
                        <div className="flex items-center justify-between gap-3">
                          <div className="text-base font-medium text-slate-950">
                            {buildTurnTitle(turnSnapshot?.role || 'user', index)}
                          </div>
                          <Badge variant="outline">{turnSnapshot?.role || 'user'}</Badge>
                        </div>
                        <div className="mt-3 space-y-3">
                          <div className="rounded-xl border border-slate-200 bg-white p-4">
                            <div className="mb-2 text-xs font-medium text-slate-500">{t('questionSnapshot')}</div>
                            <div className="whitespace-pre-wrap text-sm text-slate-800">
                              {getMessageQuery(message, commonT('none'))}
                            </div>
                          </div>
                          <div className="rounded-xl border border-slate-200 bg-white p-4">
                            <div className="mb-2 text-xs font-medium text-slate-500">系统回复</div>
                            <div className="whitespace-pre-wrap text-sm text-slate-800">
                              {getMessageText(message, commonT('none'))}
                            </div>
                          </div>
                          {message.inputs && Object.keys(message.inputs).length > 0 ? (
                            <button
                              type="button"
                              onClick={() => openRawView(`第 ${index + 1} 轮输入`, message.inputs as Record<string, unknown>)}
                              className="text-left text-xs text-slate-500 underline-offset-4 hover:underline"
                            >
                              查看本轮输入原始数据
                            </button>
                          ) : null}
                        </div>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-6 text-sm text-slate-500">
                  暂无会话记录，已回退显示测试快照。
                </div>
              )}
            </section>

            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('judgeDetail')}</h2>
              <div className="grid gap-4 lg:grid-cols-[160px_minmax(0,1fr)]">
                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <div className="text-sm text-slate-500">AI 评分</div>
                  <div className="mt-3 text-3xl font-semibold text-red-600">{judgeScoreText}</div>
                </div>
                <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                  <div className="text-slate-500">评价意见</div>
                  <div className="mt-2 whitespace-pre-wrap">{selectedItem.judge_reason || commonT('none')}</div>
                  {selectedItem.judge_suggestion ? (
                    <div className="mt-4 rounded-xl bg-amber-50 p-4 text-amber-800">
                      <div className="font-medium">{t('suggestionLabel')}</div>
                      <div className="mt-1 whitespace-pre-wrap">{selectedItem.judge_suggestion}</div>
                    </div>
                  ) : null}
                </div>
              </div>
            </section>

            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">执行记录</h2>
              <div className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">
                <div className="text-xs text-red-500">问题原因</div>
                <div className="mt-1 font-medium">{reasonText}</div>
              </div>
              <div className="rounded-2xl border border-slate-200 bg-white p-4">
                <div className="text-xs text-slate-500">执行路径</div>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {executionNodes.length > 0 ? (
                    executionNodes.map((node, index) => (
                      <React.Fragment key={node.id}>
                        <Badge variant="outline" className="rounded-full px-3 py-1 text-slate-700">
                          {formatExecutionStepLabel(node.id)}
                        </Badge>
                        {index < executionNodes.length - 1 ? <span className="text-slate-300">›</span> : null}
                      </React.Fragment>
                    ))
                  ) : (
                    <span className="text-sm text-slate-500">{commonT('none')}</span>
                  )}
                </div>
              </div>
            </section>
          </CardContent>
        </Card>
      </div>

      <Dialog open={Boolean(rawView)} onOpenChange={open => !open && setRawView(null)}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{rawView?.title || '原始数据'}</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <pre className="max-h-[70vh] overflow-auto whitespace-pre-wrap rounded-2xl border border-slate-200 bg-slate-50 p-5 text-xs text-slate-700">
              {stringifyJson(rawView?.content, commonT('none'))}
            </pre>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRawView(null)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
