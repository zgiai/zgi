'use client';

import { useMemo, useState } from 'react';
import { AlertCircle, CheckCircle2, ChevronDown, Loader2 } from 'lucide-react';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { useT } from '@/i18n/translations';
import type { ScopedTranslations } from '@/i18n/translations';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import type { AIChatSkillInvocation } from '@/services/types/aichat';
import type { AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat';
import {
  getAIChatSkillToolDisplayName,
  getFallbackAIChatSkillDisplayInfo,
  type AIChatSkillDisplayInfo,
  type AIChatSkillDisplayMap,
} from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';

type TimelineTone = 'running' | 'success' | 'error';
type TimelineDebugLabel = keyof typeof TIMELINE_DEBUG_LABEL_KEYS;
type WebappTranslator = ScopedTranslations<'webapp'>;

const TIMELINE_DEBUG_LABEL_KEYS = {
  kind: 'consoleChat.skills.trace.debug.kind',
  skillId: 'consoleChat.skills.trace.debug.skillId',
  toolName: 'consoleChat.skills.trace.debug.toolName',
  path: 'consoleChat.skills.trace.debug.path',
  duration: 'consoleChat.skills.trace.debug.duration',
  arguments: 'consoleChat.skills.trace.debug.arguments',
  message: 'consoleChat.skills.trace.debug.message',
  error: 'consoleChat.skills.trace.debug.error',
} as const;

const assistantMarkdownClassName =
  'prose prose-sm max-w-none dark:prose-invert sm:pr-4 md:pr-6 lg:pr-8 xl:pr-9';

interface AIChatAgenticTimelineProps {
  timeline: AIChatAgenticTimelineItem[];
  skillDisplayById: AIChatSkillDisplayMap;
  defaultOpen?: boolean;
}

interface SkillTimelineViewModel {
  item: Extract<AIChatAgenticTimelineItem, { type: 'skill_event' }>;
  title: string;
  detail?: string;
  skill: AIChatSkillDisplayInfo;
  tone: TimelineTone;
}

function getInvocationTone(invocation: AIChatSkillInvocation): TimelineTone {
  if (invocation.status === 'loading' || invocation.status === 'running') return 'running';
  if (invocation.status === 'error' || invocation.status === 'blocked') return 'error';
  return 'success';
}

function getStatusIcon(tone: TimelineTone) {
  if (tone === 'running') return <Loader2 className="size-3.5 animate-spin" />;
  if (tone === 'error') return <AlertCircle className="size-3.5" />;
  return <CheckCircle2 className="size-3.5 text-emerald-600" />;
}

function getDurationText(durationMs: number | undefined): string | null {
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs)) return null;
  if (durationMs < 0) return null;
  if (durationMs === 0) return '<1ms';
  return `${durationMs}ms`;
}

function formatDebugValue(value: unknown): string | null {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function timelineDebugRows(invocation: AIChatSkillInvocation, locale: string) {
  return [
    ['kind', invocation.kind],
    ['skillId', invocation.skill_id],
    ['toolName', getAIChatSkillToolDisplayName(invocation.skill_id, invocation.tool_name, locale)],
    ['path', invocation.path],
    ['duration', getDurationText(invocation.duration_ms)],
    ['arguments', invocation.arguments],
    ['message', invocation.message],
    ['error', invocation.error],
  ] as const satisfies ReadonlyArray<readonly [TimelineDebugLabel, unknown]>;
}

function buildSkillTitle(
  invocation: AIChatSkillInvocation,
  skill: AIChatSkillDisplayInfo,
  tone: TimelineTone,
  locale: string,
  t: WebappTranslator
): string {
  const toolName =
    getAIChatSkillToolDisplayName(invocation.skill_id, invocation.tool_name, locale) ||
    invocation.path ||
    t('consoleChat.skills.trace.unknownTool');

  if (invocation.kind === 'skill_load') {
    if (tone === 'running') return t('consoleChat.skills.agentic.loadingSkill', { skill: skill.label });
    if (tone === 'error') return t('consoleChat.skills.agentic.loadFailed', { skill: skill.label });
    return t('consoleChat.skills.agentic.loadedSkill', { skill: skill.label });
  }

  if (invocation.kind === 'reference_read') {
    return t('consoleChat.skills.agentic.referenceRead', {
      skill: skill.label,
      path: invocation.path || t('consoleChat.skills.trace.unknownReference'),
    });
  }

  if (tone === 'running') {
    return t('consoleChat.skills.agentic.callingTool', {
      skill: skill.label,
      tool: toolName,
    });
  }
  if (tone === 'error') {
    return t('consoleChat.skills.agentic.toolFailed', {
      skill: skill.label,
      tool: toolName,
    });
  }
  return t('consoleChat.skills.agentic.toolSucceeded', {
    skill: skill.label,
    tool: toolName,
  });
}

function SkillTimelineRow({
  event,
}: {
  event: SkillTimelineViewModel;
}) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(false);
  const duration = getDurationText(event.item.invocation.duration_ms);

  return (
    <div
      className={cn(
        'rounded-md border bg-background/80 text-xs',
        event.tone === 'error' ? 'border-destructive/30' : 'border-border'
      )}
    >
      <button
        type="button"
        className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left"
        onClick={() => setIsOpen(open => !open)}
        aria-expanded={isOpen}
      >
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full border bg-background',
            event.tone === 'error'
              ? 'border-destructive/40 text-destructive'
              : 'border-border text-muted-foreground'
          )}
        >
          {getStatusIcon(event.tone)}
        </span>
        <AIChatSkillIcon
          icon={event.skill.icon}
          className="size-3.5 shrink-0 text-muted-foreground"
        />
        <span className="min-w-0 flex-1 truncate text-foreground">{event.title}</span>
        {duration ? <span className="shrink-0 text-muted-foreground">{duration}</span> : null}
        <ChevronDown
          className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', {
            'rotate-180': isOpen,
          })}
        />
      </button>
      {isOpen ? (
        <div className="border-t bg-muted/20 px-2.5 py-2">
          {event.detail ? (
            <div className="mb-2 whitespace-pre-wrap break-words text-muted-foreground">
              {event.detail}
            </div>
          ) : null}
          <dl className="grid gap-1 rounded-md bg-background/80 p-2 text-[11px]">
            {timelineDebugRows(event.item.invocation, locale).map(([labelKey, value]) => {
              const formatted = formatDebugValue(value);
              if (!formatted) return null;

              return (
                <div key={labelKey} className="grid grid-cols-[88px_minmax(0,1fr)] gap-2">
                  <dt className="text-muted-foreground">
                    {t(TIMELINE_DEBUG_LABEL_KEYS[labelKey])}
                  </dt>
                  <dd className="min-w-0 max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-foreground/80">
                    {formatted}
                  </dd>
                </div>
              );
            })}
          </dl>
        </div>
      ) : null}
    </div>
  );
}

function isProgressTextItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }> {
  return 'type' in item && item.type === 'progress_text';
}

function isIntermediateAnswerItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is Extract<AIChatAgenticTimelineItem, { type: 'intermediate_answer' }> {
  return 'type' in item && item.type === 'intermediate_answer';
}

export function AIChatAgenticTimeline({
  timeline,
  skillDisplayById,
  defaultOpen = true,
}: AIChatAgenticTimelineProps) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(defaultOpen);

  const events = useMemo(
    () =>
      timeline.map(item => {
        if (item.type === 'progress_text') return item;
        if (item.type === 'intermediate_answer') return item;

        const skillId = item.invocation.skill_id || t('consoleChat.skills.trace.unknownSkill');
        const skill =
          skillDisplayById[skillId] ?? getFallbackAIChatSkillDisplayInfo(skillId, locale);
        const tone = getInvocationTone(item.invocation);

        return {
          item,
          skill,
          tone,
          title: buildSkillTitle(item.invocation, skill, tone, locale, t),
          detail: item.invocation.message || item.invocation.error,
        };
      }),
    [locale, skillDisplayById, t, timeline]
  );

  if (events.length === 0) return null;

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen} className="mb-3 max-w-3xl">
      <div className="mb-2 flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 text-xs text-muted-foreground"
          asChild
        >
          <CollapsibleTrigger>
            <ChevronDown
              className={cn('size-3.5 transition-transform', { 'rotate-180': isOpen })}
            />
            {isOpen
              ? t('consoleChat.skills.agentic.hideProcess')
              : t('consoleChat.skills.agentic.showProcess')}
          </CollapsibleTrigger>
        </Button>
        <span className="text-[11px] text-muted-foreground">
          {t('consoleChat.skills.trace.eventCount', { count: events.length })}
        </span>
      </div>
      <CollapsibleContent>
        <div className="space-y-2">
          {events.map(item =>
            isProgressTextItem(item) ? (
              <div
                key={item.id}
                className={cn(
                  assistantMarkdownClassName,
                  'border-l-2 border-muted-foreground/20 pl-3 text-foreground'
                )}
              >
                <MarkdownViewer className="md-viewer break-words" content={item.content} />
              </div>
            ) : isIntermediateAnswerItem(item) ? (
              <div key={item.id} className="space-y-1.5 border-l-2 border-muted-foreground/20 pl-3">
                {item.title || item.status === 'streaming' ? (
                  <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                    {item.status === 'streaming' ? (
                      <Loader2 className="size-3 animate-spin" />
                    ) : null}
                    {item.title ? <span>{item.title}</span> : null}
                  </div>
                ) : null}
                <div className={assistantMarkdownClassName}>
                  <MarkdownViewer className="md-viewer break-words" content={item.content} />
                </div>
              </div>
            ) : (
              <SkillTimelineRow key={item.item.id} event={item} />
            )
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
