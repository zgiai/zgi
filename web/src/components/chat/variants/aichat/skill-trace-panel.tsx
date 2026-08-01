'use client';

import { useMemo, useState } from 'react';
import { AlertCircle, CheckCircle2, ChevronDown, Loader2 } from 'lucide-react';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { useT, type ScopedTranslations } from '@/i18n/translations';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import { formatMs } from '@/utils/format';
import type { AIChatSkillInvocation } from '@/services/types/aichat';
import {
  getAIChatSkillResultDisplay,
  getAIChatSkillToolDisplayName,
  getFallbackAIChatSkillDisplayInfo,
  type AIChatSkillDisplayInfo,
  type AIChatSkillDisplayMap,
} from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import { AIChatSkillResultSummary } from '@/components/chat/variants/aichat/skill-result-summary';
import {
  getAIChatExternalInvocationDisplayName,
  isAIChatExternalAppsInvocation,
} from '@/components/chat/variants/aichat/external-app-display';
import { sanitizeTimelineDisplayString } from '@/components/chat/variants/aichat/timeline-display-safety';
import {
  formatAIChatTimelineArgumentSummary,
  formatAIChatTimelineValue,
  getAIChatInvocationKindLabel,
  localizeAIChatRuntimeMessage,
} from '@/components/chat/variants/aichat/timeline-display-i18n';

type SkillTraceTone = 'running' | 'success' | 'error';
type SkillTraceDebugLabel = keyof typeof SKILL_TRACE_DEBUG_LABEL_KEYS;

const SKILL_TRACE_DEBUG_LABEL_KEYS = {
  kind: 'consoleChat.skills.trace.debug.kind',
  skillId: 'consoleChat.skills.trace.debug.skillId',
  toolName: 'consoleChat.skills.trace.debug.toolName',
  path: 'consoleChat.skills.trace.debug.path',
  duration: 'consoleChat.skills.trace.debug.duration',
  arguments: 'consoleChat.skills.trace.debug.arguments',
  message: 'consoleChat.skills.trace.debug.message',
  error: 'consoleChat.skills.trace.debug.error',
} as const;

interface SkillTraceEvent {
  invocation: AIChatSkillInvocation;
  title: string;
  detail?: string;
  skill: AIChatSkillDisplayInfo;
  tone: SkillTraceTone;
}

interface AIChatSkillTracePanelProps {
  invocations: AIChatSkillInvocation[];
  skillDisplayById: AIChatSkillDisplayMap;
}

function getInvocationTone(invocation: AIChatSkillInvocation): SkillTraceTone {
  if (invocation.status === 'loading' || invocation.status === 'running') return 'running';
  if (invocation.status === 'error') return 'error';
  return 'success';
}

function getStatusIcon(tone: SkillTraceTone) {
  if (tone === 'running') return <Loader2 className="size-3.5 animate-spin" />;
  if (tone === 'error') return <AlertCircle className="size-3.5" />;
  return <CheckCircle2 className="size-3.5 text-emerald-600" />;
}

function getDurationText(
  durationMs: number | undefined,
  t: ScopedTranslations<'webapp'>
): string | null {
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs)) return null;
  if (durationMs < 0) return null;
  if (durationMs === 0) return t('consoleChat.skills.trace.values.lessThanOneMillisecond');
  return formatMs(durationMs);
}

function skillTraceDebugRows(
  invocation: AIChatSkillInvocation,
  locale: string,
  t: ScopedTranslations<'webapp'>
) {
  const isExternalApp = isAIChatExternalAppsInvocation(invocation);
  const fallbackToolName =
    getAIChatSkillToolDisplayName(invocation.skill_id, invocation.tool_name, locale) ||
    t('consoleChat.connectedApps.actions.generic');
  const externalToolName = getAIChatExternalInvocationDisplayName(
    invocation,
    locale,
    t,
    fallbackToolName
  );
  const argumentSummary = isExternalApp
    ? {
        type: 'object',
        keys: Object.keys(invocation.arguments ?? {}).filter(
          key => key !== 'argument_labels_i18n' && key !== 'argument_value_labels_i18n'
        ).length,
      }
    : invocation.arguments;
  const reasonCode = invocation.arguments?.reason_code;
  return [
    ['kind', getAIChatInvocationKindLabel(invocation.kind, t)],
    ['skillId', isExternalApp ? null : invocation.skill_id],
    ['toolName', externalToolName ?? fallbackToolName],
    ['path', isExternalApp ? null : invocation.path],
    ['duration', getDurationText(invocation.duration_ms, t)],
    ['arguments', formatAIChatTimelineArgumentSummary(argumentSummary, t)],
    [
      'message',
      localizeAIChatRuntimeMessage(
        invocation.message,
        t,
        undefined,
        invocation.error_code,
        reasonCode
      ),
    ],
    [
      'error',
      localizeAIChatRuntimeMessage(
        invocation.error,
        t,
        undefined,
        invocation.error_code,
        reasonCode
      ),
    ],
  ] as const satisfies ReadonlyArray<readonly [SkillTraceDebugLabel, unknown]>;
}

/**
 * @component AIChatSkillTracePanel
 * @category Feature
 * @status Stable
 * @description Collapsible Codex-style Skill V2 runtime event timeline.
 * @usage Render below assistant message metadata when skill invocations exist.
 * @example
 * <AIChatSkillTracePanel invocations={events} skillDisplayById={skillDisplayById} />
 */
export function AIChatSkillTracePanel({
  invocations,
  skillDisplayById,
}: AIChatSkillTracePanelProps) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(false);

  const events = useMemo<SkillTraceEvent[]>(
    () =>
      invocations.map(invocation => {
        const skillId = invocation.skill_id || t('consoleChat.skills.trace.unknownSkill');
        const baseSkill =
          skillDisplayById[skillId] ?? getFallbackAIChatSkillDisplayInfo(skillId, locale);
        const isExternalApp = isAIChatExternalAppsInvocation(invocation);
        const skill = isExternalApp
          ? { ...baseSkill, label: t('consoleChat.connectedApps.title') }
          : baseSkill;
        const fallbackToolName =
          getAIChatSkillToolDisplayName(invocation.skill_id, invocation.tool_name, locale) ||
          (isExternalApp ? null : invocation.path) ||
          t('consoleChat.skills.trace.unknownTool');
        const toolName =
          getAIChatExternalInvocationDisplayName(invocation, locale, t, fallbackToolName) ??
          fallbackToolName;
        const tone = getInvocationTone(invocation);
        const resultDetail = getAIChatSkillResultDisplay(invocation, locale);
        const reasonCode = invocation.arguments?.reason_code;

        if (invocation.kind === 'skill_load') {
          return {
            invocation,
            skill,
            tone,
            title:
              tone === 'running'
                ? t('consoleChat.skills.trace.loading', { skill: skill.label })
                : tone === 'error'
                  ? t('consoleChat.skills.trace.error', { skill: skill.label })
                  : t('consoleChat.skills.trace.loaded', { skill: skill.label }),
            detail:
              resultDetail ||
              localizeAIChatRuntimeMessage(
                invocation.message,
                t,
                undefined,
                invocation.error_code,
                reasonCode
              ) ||
              localizeAIChatRuntimeMessage(
                invocation.error,
                t,
                undefined,
                invocation.error_code,
                reasonCode
              ) ||
              undefined,
          };
        }

        if (invocation.kind === 'reference_read') {
          return {
            invocation,
            skill,
            tone,
            title: t('consoleChat.skills.trace.referenceRead', {
              skill: skill.label,
              path: isExternalApp
                ? toolName
                : invocation.path || t('consoleChat.skills.trace.unknownReference'),
            }),
            detail:
              resultDetail ||
              localizeAIChatRuntimeMessage(
                invocation.message,
                t,
                undefined,
                invocation.error_code,
                reasonCode
              ) ||
              localizeAIChatRuntimeMessage(
                invocation.error,
                t,
                undefined,
                invocation.error_code,
                reasonCode
              ) ||
              undefined,
          };
        }

        return {
          invocation,
          skill,
          tone,
          title:
            tone === 'running'
              ? t('consoleChat.skills.trace.running', {
                  skill: skill.label,
                  tool: toolName,
                })
              : tone === 'error'
                ? t('consoleChat.skills.trace.error', { skill: skill.label })
                : t('consoleChat.skills.trace.success', {
                    skill: skill.label,
                    tool: toolName,
                  }),
          detail:
            resultDetail ||
            localizeAIChatRuntimeMessage(
              invocation.message,
              t,
              undefined,
              invocation.error_code,
              reasonCode
            ) ||
            localizeAIChatRuntimeMessage(
              invocation.error,
              t,
              undefined,
              invocation.error_code,
              reasonCode
            ) ||
            undefined,
        };
      }),
    [invocations, locale, skillDisplayById, t]
  );

  const summary = useMemo(() => {
    const latestRunning = [...events].reverse().find(event => event.tone === 'running');
    if (latestRunning) return latestRunning;

    const latestError = [...events].reverse().find(event => event.tone === 'error');
    if (latestError) return latestError;

    const latestSuccess = [...events].reverse().find(event => event.tone === 'success');
    if (!latestSuccess) return null;

    if (events.filter(event => event.tone === 'success').length > 1) {
      return {
        ...latestSuccess,
        title: t('consoleChat.skills.trace.summarySuccess', {
          count: events.length,
        }),
      };
    }

    return latestSuccess;
  }, [events, t]);

  if (!summary || events.length === 0) return null;

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen} className="mb-3 max-w-3xl">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className={cn(
            'flex h-8 w-full max-w-xl items-center gap-2 rounded-md border bg-muted/30 px-2.5 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/50',
            summary.tone === 'error' && 'border-destructive/30 bg-destructive/10 text-destructive'
          )}
          aria-expanded={isOpen}
        >
          <span className="shrink-0">{getStatusIcon(summary.tone)}</span>
          <AIChatSkillIcon icon={summary.skill.icon} className="size-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{summary.title}</span>
          {events.length > 1 ? (
            <span className="shrink-0 text-[11px] text-muted-foreground">
              {t('consoleChat.skills.trace.eventCount', { count: events.length })}
            </span>
          ) : null}
          <ChevronDown
            className={cn('size-3.5 shrink-0 transition-transform', isOpen ? 'rotate-180' : '')}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 max-w-xl overflow-hidden rounded-md border bg-background/80 p-2">
          <div className="max-h-56 overflow-y-auto pr-2">
            <div className="space-y-2">
              {events.map((event, index) => {
                const duration = getDurationText(event.invocation.duration_ms, t);
                const detail = event.detail
                  ? sanitizeTimelineDisplayString(
                      event.detail,
                      t('consoleChat.skills.trace.values.hidden'),
                      t('consoleChat.skills.trace.values.truncated')
                    )
                  : null;

                return (
                  <div
                    key={`${event.invocation.kind ?? 'tool_call'}-${event.invocation.skill_id}-${event.invocation.tool_name ?? event.invocation.path ?? ''}-${index}`}
                    className="flex gap-2 text-xs"
                  >
                    <div className="flex items-start">
                      <div
                        className={cn(
                          'flex size-5 items-center justify-center rounded-full border bg-background',
                          event.tone === 'error'
                            ? 'border-destructive/40 text-destructive'
                            : 'border-border text-muted-foreground'
                        )}
                      >
                        {getStatusIcon(event.tone)}
                      </div>
                    </div>
                    <div className="min-w-0 flex-1 pb-1">
                      <div className="flex min-w-0 items-center gap-1.5">
                        <AIChatSkillIcon
                          icon={event.skill.icon}
                          className="size-3.5 shrink-0 text-muted-foreground"
                        />
                        <span className="truncate text-foreground">{event.title}</span>
                        {duration ? (
                          <span className="shrink-0 text-muted-foreground">{duration}</span>
                        ) : null}
                      </div>
                      {detail ? (
                        <div className="mt-1 line-clamp-2 text-muted-foreground">{detail}</div>
                      ) : null}
                      <AIChatSkillResultSummary
                        result={event.invocation.result}
                        skillId={event.invocation.skill_id}
                        toolName={event.invocation.tool_name}
                        className="mt-2"
                      />
                      <dl className="mt-2 grid gap-1 rounded-md bg-muted/30 p-2 text-[11px]">
                        {skillTraceDebugRows(event.invocation, locale, t).map(
                          ([labelKey, value]) => {
                            const formatted = formatAIChatTimelineValue(value, t);
                            if (!formatted) return null;

                            return (
                              <div
                                key={labelKey}
                                className="grid grid-cols-[88px_minmax(0,1fr)] gap-2"
                              >
                                <dt className="text-muted-foreground">
                                  {t(SKILL_TRACE_DEBUG_LABEL_KEYS[labelKey])}
                                </dt>
                                <dd className="min-w-0 whitespace-pre-wrap break-all font-mono text-foreground/80">
                                  {formatted}
                                </dd>
                              </div>
                            );
                          }
                        )}
                      </dl>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
