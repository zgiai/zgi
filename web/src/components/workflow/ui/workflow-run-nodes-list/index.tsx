import React, { useMemo, useState } from 'react';
import { NODE_THEMES } from '@/components/workflow/nodes/custom/config';
import { cn } from '@/lib/utils';
import { AlertTriangle, Check, ChevronDown, ChevronLeft, Copy, Filter, Loader } from 'lucide-react';
import { useT } from '@/i18n';
import { formatMs } from '@/utils/format';
import MarkdownViewer from '@/components/common/markdown-viewer';
import type { RuntimeLabel, WorkflowRunNodesListProps } from './types';
import {
  getCanvasPreviewRows,
  getNodeSummary,
  getNodesElapsedTime,
  getRuntimeLogSections,
  groupWorkflowRunItems,
  isEmptyValue,
  normalizeNodeRunStatus,
  previewToneClass,
  serializeForClipboard,
} from './utils';
import { RuntimeStructuredView, RuntimeValuePreview } from './runtime-structured-view';

export type { NodeRunStatus, WorkflowRunNodeListItem } from './types';
const WorkflowRunNodesList: React.FC<WorkflowRunNodesListProps> = ({
  items,
  showDetail = true,
  variant = 'panel',
  hideCanvasNodeChrome = false,
}) => {
  const t = useT();
  const runtimeLabel: RuntimeLabel = (key, params) =>
    t(`agents.workflow.runtimeLog.${key}` as Parameters<typeof t>[0], params);
  const isCanvasVariant = variant === 'canvas';
  const isCanvasDetailOnly = isCanvasVariant && hideCanvasNodeChrome;
  const styles = {
    running: {
      wrap: 'border-l-4 border-l-info border border-border/50 bg-card',
      dot: 'bg-info animate-pulse',
      text: 'text-info',
      label: t('agents.workflow.running'),
    },
    succeeded: {
      wrap: 'border-l-4 border-l-success border border-border/50 bg-card',
      dot: 'bg-success',
      text: 'text-success',
      label: t('agents.workflow.succeeded'),
    },
    failed: {
      wrap: 'border-l-4 border-l-destructive border border-border/50 bg-card',
      dot: 'bg-destructive',
      text: 'text-destructive',
      label: t('agents.workflow.failed'),
    },
    stopped: {
      wrap: 'border-l-4 border-l-muted-foreground border border-border/50 bg-card',
      dot: 'bg-muted-foreground',
      text: 'text-muted-foreground',
      label: t('agents.workflow.stopped'),
    },
    paused: {
      wrap: 'border-l-4 border-l-warning border border-border/50 bg-card',
      dot: 'bg-warning',
      text: 'text-warning',
      label: t('agents.workflow.paused'),
    },
  };
  const [openSet, setOpenSet] = useState<Set<string>>(() => new Set());
  const [selectedExecutionByNode, setSelectedExecutionByNode] = useState<Record<string, number>>(
    {}
  );
  const [errorOnlyByNode, setErrorOnlyByNode] = useState<Record<string, boolean>>({});
  const [copiedSection, setCopiedSection] = useState<string | null>(null);
  const groupedItems = useMemo(() => groupWorkflowRunItems(items), [items]);
  const elapsedTimeClass =
    'text-[10px] text-muted-foreground/60 tabular-nums px-1 py-0.5 tracking-tighter';

  const isOpen = (id: string) => openSet.has(id);
  const toggleOpen = (id: string) =>
    setOpenSet(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const copySectionValue = async (id: string, value: unknown, e?: React.MouseEvent) => {
    e?.stopPropagation();
    if (typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(serializeForClipboard(value));
    setCopiedSection(id);
    window.setTimeout(() => setCopiedSection(current => (current === id ? null : current)), 1200);
  };
  const [sectionOpenSet, setSectionOpenSet] = useState<Set<string>>(() => new Set());
  const isSectionOpen = (id: string, defaultOpen = false) =>
    defaultOpen ? !sectionOpenSet.has(id) : sectionOpenSet.has(id);
  const toggleSection = (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setSectionOpenSet(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const [containerRoundsViewSet, setContainerRoundsViewSet] = useState<Set<string>>(
    () => new Set()
  );
  const isContainerRoundsView = (id: string) => containerRoundsViewSet.has(id);
  const enterContainerRoundsView = (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setContainerRoundsViewSet(prev => {
      const next = new Set(prev);
      next.add(id);
      return next;
    });
  };
  const exitContainerRoundsView = (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setContainerRoundsViewSet(prev => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };
  const isRoundOpen = (id: string, defaultOpen = false) =>
    defaultOpen ? !openSet.has(id) : openSet.has(id);

  return (
    <div className={cn('relative', isCanvasDetailOnly ? 'space-y-1.5' : 'space-y-2')}>
      {groupedItems.map(group => {
        const errorOnly = Boolean(errorOnlyByNode[group.key]);
        const visibleExecutions = errorOnly
          ? group.executions.filter(
              item => normalizeNodeRunStatus(item.status) === 'failed' || Boolean(item.error)
            )
          : group.executions;
        const executions = visibleExecutions;
        if (executions.length === 0) return null;
        const selectedIndex = Math.min(
          selectedExecutionByNode[group.key] ?? executions.length - 1,
          Math.max(0, executions.length - 1)
        );
        const raw = executions[selectedIndex];
        if (!raw) return null;
        const itemKey = group.key;
        const executionKey = raw.executionId ?? `${raw.nodeId}-${selectedIndex}`;
        const summary = getNodeSummary(raw, runtimeLabel);
        const sections = getRuntimeLogSections(raw, runtimeLabel);
        const previewRows = isCanvasVariant ? getCanvasPreviewRows(raw, runtimeLabel) : [];
        const theme =
          raw.nodeType in NODE_THEMES
            ? NODE_THEMES[raw.nodeType as keyof typeof NODE_THEMES]
            : undefined;
        const Icon = theme?.icon;

        const rawStatus = normalizeNodeRunStatus(raw.status);
        const statusCfg = styles[rawStatus];
        const isIteration = raw.nodeType === 'iteration';
        const isLoop = raw.nodeType === 'loop';
        const isContainer = isIteration || isLoop;
        const rounds = isLoop ? (raw.loopRounds ?? []) : (raw.iterationRounds ?? []);
        const shouldInlineContainerRounds = isContainer && isCanvasDetailOnly;
        const inContainerRoundsView =
          isContainer && (shouldInlineContainerRounds || isContainerRoundsView(itemKey));
        const debugDetailsId = `${itemKey}-debug-details`;
        const hasDebugDetails =
          showDetail &&
          !inContainerRoundsView &&
          (raw.modelInput !== undefined ||
            raw.nodeInput !== undefined ||
            raw.loopInputs !== undefined ||
            !isEmptyValue(raw.processData) ||
            !isEmptyValue(raw.executionMetadata));
        const debugDetailsOpen = isSectionOpen(debugDetailsId);
        const canToggle =
          !isCanvasVariant &&
          (isContainer ||
            (showDetail &&
              (sections.length > 0 ||
                raw.nodeInput !== undefined ||
                raw.nodeOutput !== undefined ||
                raw.processData !== undefined ||
                raw.executionMetadata !== undefined)));
        const shouldRenderDetailsArea = isContainer
          ? shouldInlineContainerRounds || isOpen(itemKey)
          : showDetail && isOpen(itemKey);

        return (
          <div
            key={`run-item-${itemKey}`}
            className={cn(
              'rounded-lg px-2 py-2 transition-all duration-500 relative z-10 border',
              isCanvasVariant && 'rounded-md border-border/70 bg-background shadow-sm',
              isCanvasDetailOnly && 'border-border/70 bg-background/95 p-2 shadow-none',
              statusCfg.wrap,
              isCanvasDetailOnly && 'border-l border-l-border/70',
              isCanvasDetailOnly
                ? 'border-border/70 hover:border-border/70'
                : isCanvasVariant
                ? 'border-border/70 hover:border-border/80 hover:bg-background hover:shadow-md'
                : isOpen(itemKey)
                  ? 'shadow-[0_4px_12px_rgba(0,0,0,0.06),0_0_1px_rgba(0,0,0,0.1)] bg-card border-border/80 scale-[1.01]'
                  : 'border-transparent hover:border-border/40 hover:bg-muted/10 hover:shadow-sm'
            )}
          >
            {!isCanvasDetailOnly ? (
              <div
                className={cn(
                  'flex items-center gap-2',
                  canToggle ? 'cursor-pointer select-none' : 'cursor-default'
                )}
                onClick={() => canToggle && toggleOpen(itemKey)}
              >
                {/* Toggle button on the left */}
                <div className="w-3 flex items-center justify-center shrink-0">
                  {canToggle ? (
                    <ChevronDown
                      className={cn(
                        'h-3.5 w-3.5 transition-transform text-muted-foreground/40 hover:text-foreground',
                        isOpen(itemKey) ? '' : '-rotate-90'
                      )}
                    />
                  ) : null}
                </div>

                {/* Icon wrap - smaller and more refined */}
                <div
                  className={cn(
                    'w-5 h-5 flex items-center justify-center rounded text-white shrink-0 shadow-[0_1px_2px_rgba(0,0,0,0.1)] transition-all duration-300',
                    theme?.classNames.iconBg,
                    rawStatus === 'paused' && 'bg-warning text-white shadow-none',
                    isOpen(itemKey) ? 'ring-2 ring-background ring-offset-1 scale-105' : ''
                  )}
                  aria-label={raw.nodeType}
                >
                  {Icon ? <Icon className="w-3 h-3" /> : null}
                </div>

                {/* Title and status */}
                <div className="flex-1 min-w-0 ml-0.5">
                  <div
                    className={cn(
                      'text-[13px] font-semibold truncate tracking-tight text-foreground/80',
                      theme?.classNames.title
                    )}
                  >
                    {raw.title}
                  </div>
                  {summary ? (
                    <div className="mt-0.5 w-fit max-w-full truncate rounded bg-primary/5 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                      {summary}
                    </div>
                  ) : null}
                </div>
                {isCanvasVariant && group.executions.length > 1 ? (
                  <div className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    {runtimeLabel('executionCount', { count: group.executions.length })}
                  </div>
                ) : null}
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      'w-1.5 h-1.5 rounded-full shrink-0 shadow-[0_0_4px_rgba(0,0,0,0.1)]',
                      statusCfg.dot,
                      rawStatus === 'running' && 'animate-pulse-subtle'
                    )}
                  />
                  {rawStatus === 'running' ? (
                    <Loader className="h-3 w-3 animate-spin text-info" />
                  ) : rawStatus === 'paused' ? null : (
                    <div className={elapsedTimeClass}>
                      {typeof raw?.elapsedTime === 'number' && raw.elapsedTime > 0
                        ? formatMs(raw.elapsedTime)
                        : '-'}
                    </div>
                  )}
                </div>
              </div>
            ) : null}
            {isCanvasVariant && previewRows.length > 0 ? (
              <div className={cn('grid gap-1.5', isCanvasDetailOnly ? 'mt-0' : 'mt-2')}>
                {previewRows.map(row => (
                  <div
                    key={`${executionKey}-${row.label}`}
                    className={cn(
                      'rounded-md border px-2 py-1.5 text-[11px] leading-4',
                      row.tone ? previewToneClass[row.tone] : 'border-border/50 bg-muted/20'
                    )}
                  >
                    <div className="mb-0.5 text-[10px] font-medium text-muted-foreground">
                      {row.label}
                    </div>
                    <RuntimeValuePreview
                      value={row.value}
                      lines={
                        row.label === runtimeLabel('reply') ||
                        row.label === runtimeLabel('replyContent')
                          ? 3
                          : 2
                      }
                      expandable
                      maxRecordEntries={row.maxRecordEntries}
                      runtimeLabel={runtimeLabel}
                    />
                  </div>
                ))}
              </div>
            ) : null}
            {!isCanvasVariant && group.executions.length > 1 ? (
              <div className="mt-2 flex flex-wrap items-center gap-2 rounded-md border border-border/40 bg-muted/20 px-2 py-1.5">
                <span className="text-[11px] text-muted-foreground">
                  {runtimeLabel('totalCount', { count: group.executions.length })}
                </span>
                <label
                  className="ml-auto inline-flex cursor-pointer select-none items-center gap-1.5 text-[11px] text-muted-foreground"
                  onClick={e => e.stopPropagation()}
                >
                  <input
                    type="checkbox"
                    className="h-3 w-3 rounded border-border"
                    checked={errorOnly}
                    onChange={event => {
                      const checked = event.currentTarget.checked;
                      setErrorOnlyByNode(prev => ({ ...prev, [group.key]: checked }));
                      setSelectedExecutionByNode(prev => ({ ...prev, [group.key]: 0 }));
                    }}
                  />
                  <Filter className="h-3 w-3" />
                  {runtimeLabel('errorsOnly')}
                </label>
                <div className="flex max-w-full flex-wrap gap-1">
                  {executions.map((execution, index) => (
                    <button
                      key={`${group.key}-execution-${execution.executionId ?? index}`}
                      type="button"
                      className={cn(
                        'h-6 min-w-6 rounded-md border px-2 text-[11px] tabular-nums transition-colors',
                        index === selectedIndex
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border/60 bg-background text-muted-foreground hover:bg-muted'
                      )}
                      onClick={event => {
                        event.stopPropagation();
                        setSelectedExecutionByNode(prev => ({ ...prev, [group.key]: index }));
                      }}
                    >
                      {index + 1}
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
            {!isCanvasVariant && raw.error ? (
              <div
                className="mt-2 rounded-md border border-destructive/15 bg-destructive/[0.03] px-2.5 py-2"
                title={raw.error}
              >
                <div className="mb-1 flex items-center gap-1.5 text-[11px] font-medium text-destructive/80">
                  <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
                  <span>{t('agents.workflow.errors.executionFailed')}</span>
                </div>
                <p className="text-[12px] leading-5 text-foreground/75 break-words">{raw.error}</p>
              </div>
            ) : null}

            {shouldRenderDetailsArea && (
              <div className="mt-2.5 grid gap-2 pl-4.5 min-w-0">
                {/* LLM result section */}
                {(() => {
                  if (!showDetail || raw.nodeType !== 'llm') return null;
                  const output = raw.nodeOutput as { text?: string } | null | undefined;
                  const textResult = output?.text;
                  if (!textResult) return null;
                  const resultId = `${itemKey}-result`;
                  const resultOpen = isSectionOpen(resultId, true);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(resultId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-primary/40 rounded-full" />
                          {t('agents.workflow.results')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(resultId, textResult, e)}
                            aria-label={runtimeLabel('copyResult')}
                          >
                            {copiedSection === resultId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              resultOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {resultOpen && (
                        <div className="bg-muted/10 rounded-md px-3 py-2 overflow-auto max-h-80 text-[12.5px] leading-relaxed text-foreground/90 border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <MarkdownViewer content={textResult} />
                        </div>
                      )}
                    </div>
                  );
                })()}

                {hasDebugDetails ? (
                  <button
                    type="button"
                    className="ml-1 inline-flex w-fit items-center gap-1 rounded-md border border-border/40 bg-muted/20 px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    onClick={e => toggleSection(debugDetailsId, e)}
                  >
                    <ChevronDown
                      className={cn(
                        'h-3 w-3 transition-transform',
                        debugDetailsOpen ? '' : '-rotate-90'
                      )}
                    />
                    {runtimeLabel('debugDetails')}
                  </button>
                ) : null}

                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    raw.nodeType !== 'llm' ||
                    raw.modelInput === undefined
                  ) {
                    return null;
                  }
                  const modelInputId = `${itemKey}-model-input`;
                  const modelInputOpen = isSectionOpen(modelInputId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(modelInputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-primary/40 rounded-full" />
                          {t('agents.workflow.modelInput')}
                        </div>
                        <ChevronDown
                          className={cn(
                            'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                            modelInputOpen ? '' : '-rotate-90'
                          )}
                        />
                      </div>
                      {modelInputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.modelInput ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}

                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    raw.nodeInput === undefined
                  ) {
                    return null;
                  }
                  const inputId = `${itemKey}-input`;
                  const inputOpen = isSectionOpen(inputId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(inputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-info/40 rounded-full" />
                          {t('agents.workflow.input')}
                        </div>
                        <ChevronDown
                          className={cn(
                            'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                            inputOpen ? '' : '-rotate-90'
                          )}
                        />
                      </div>
                      {inputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.nodeInput ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    raw.loopInputs === undefined
                  ) {
                    return null;
                  }
                  const loopInputId = `${itemKey}-loop-input`;
                  const loopInputOpen = isSectionOpen(loopInputId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(loopInputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-info/40 rounded-full" />
                          {t('agents.workflow.loopInputs')}
                        </div>
                        <ChevronDown
                          className={cn(
                            'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                            loopInputOpen ? '' : '-rotate-90'
                          )}
                        />
                      </div>
                      {loopInputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.loopInputs ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}

                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    raw.nodeOutput === undefined ||
                    raw.nodeType === 'loop'
                  ) {
                    return null;
                  }
                  const outputId = `${itemKey}-output`;
                  const outputOpen = isSectionOpen(outputId, raw.nodeType !== 'llm');
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(outputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-success/40 rounded-full" />
                          {t('agents.workflow.output')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(outputId, raw.nodeOutput, e)}
                            aria-label={runtimeLabel('copyOutput')}
                          >
                            {copiedSection === outputId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              outputOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {outputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.nodeOutput ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (!showDetail || inContainerRoundsView || raw.loopOutputs === undefined) {
                    return null;
                  }
                  const loopOutputId = `${itemKey}-loop-output`;
                  const loopOutputOpen = isSectionOpen(loopOutputId, true);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(loopOutputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-success/40 rounded-full" />
                          {t('agents.workflow.loopOutputs')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(loopOutputId, raw.loopOutputs, e)}
                            aria-label={runtimeLabel('copyOutput')}
                          >
                            {copiedSection === loopOutputId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              loopOutputOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {loopOutputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.loopOutputs ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    isEmptyValue(raw.processData)
                  ) {
                    return null;
                  }
                  const processId = `${executionKey}-process-data`;
                  const processOpen = isSectionOpen(processId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(processId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-warning/60 rounded-full" />
                          {runtimeLabel('processInfo')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(processId, raw.processData, e)}
                            aria-label={runtimeLabel('copyProcessInfo')}
                          >
                            {copiedSection === processId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              processOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {processOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.processData ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    isEmptyValue(raw.executionMetadata)
                  ) {
                    return null;
                  }
                  const metadataId = `${executionKey}-execution-metadata`;
                  const metadataOpen = isSectionOpen(metadataId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(metadataId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-primary/50 rounded-full" />
                          {runtimeLabel('executionMetadata')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(metadataId, raw.executionMetadata, e)}
                            aria-label={runtimeLabel('copyExecutionMetadata')}
                          >
                            {copiedSection === metadataId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              metadataOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {metadataOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.executionMetadata ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {isContainer && !inContainerRoundsView && rounds.length > 0 && (
                  <button
                    type="button"
                    className="mt-1 w-full rounded-md border border-border/40 bg-muted/40 px-2 py-2 text-xs text-left text-foreground hover:bg-muted transition-colors"
                    onClick={e => enterContainerRoundsView(itemKey, e)}
                  >
                    <div className="flex items-center justify-between">
                      <span>
                        {isLoop
                          ? t('agents.workflow.loopRoundsTotal', { count: rounds.length })
                          : t('agents.workflow.iterationRoundsTotal', { count: rounds.length })}
                      </span>
                      <span className="text-muted-foreground">
                        {t('agents.workflow.viewRoundsDetails')}
                      </span>
                    </div>
                  </button>
                )}
                {isContainer &&
                  inContainerRoundsView &&
                  !shouldInlineContainerRounds && (
                  <button
                    type="button"
                    className="inline-flex w-fit items-center gap-1 rounded-md border border-border/40 bg-muted/30 px-2 py-1 text-xs text-foreground hover:bg-muted transition-colors"
                    onClick={e => exitContainerRoundsView(itemKey, e)}
                  >
                    <ChevronLeft className="h-3 w-3" />
                    {t('agents.workflow.backToSummary')}
                  </button>
                  )}
                {isContainer && inContainerRoundsView && rounds.length > 0 && (
                  <div className="grid gap-1 bg-muted shadow-md rounded-md p-1">
                    {rounds.map(round => {
                      const roundKey = `${itemKey}-round-${round.index}`;
                      const roundOpen = isRoundOpen(roundKey, shouldInlineContainerRounds);
                      const roundElapsedTime =
                        round.elapsedTime ?? getNodesElapsedTime(round.nodes);
                      return (
                        <div
                          key={`round-${itemKey}-${round.index}`}
                          className="rounded-md border p-1 bg-background"
                        >
                          <div
                            className={cn('flex items-center justify-between cursor-pointer')}
                            onClick={() => toggleOpen(roundKey)}
                          >
                            <div className="flex items-center gap-1">
                              <ChevronDown
                                className={cn(
                                  'h-3.5 w-3.5 transition-transform',
                                  roundOpen ? '' : '-rotate-90'
                                )}
                              />
                              <span className="text-xs font-medium text-foreground">
                                {isLoop
                                  ? t('agents.workflow.loopRound', { index: round.index + 1 })
                                  : t('agents.workflow.iterationRound', {
                                      index: round.index + 1,
                                    })}
                              </span>
                            </div>
                            <span className={elapsedTimeClass}>
                              {typeof roundElapsedTime === 'number' && roundElapsedTime > 0
                                ? formatMs(roundElapsedTime)
                                : '-'}
                            </span>
                          </div>
                          {roundOpen && (
                            <div className="mt-2">
                              <WorkflowRunNodesList
                                items={round.nodes}
                                showDetail={showDetail}
                                variant="panel"
                              />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
};

export default React.memo(WorkflowRunNodesList);
