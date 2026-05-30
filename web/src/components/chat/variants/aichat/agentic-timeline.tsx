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
  getAIChatSkillResultDisplay,
  getAIChatSkillToolDisplayName,
  getAIChatUserMemoryMutationTitle,
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
  'prose prose-sm min-w-0 max-w-full dark:prose-invert sm:pr-4 md:pr-6 lg:pr-8 xl:pr-9';

const TRANSIENT_PROGRESS_TEXT_KEYS = [
  'consoleChat.skills.agentic.thinking',
  'consoleChat.skills.agentic.organizing',
  'consoleChat.skills.agentic.preparing',
  'consoleChat.skills.agentic.checkingTools',
] as const;

interface AIChatAgenticTimelineProps {
  timeline: AIChatAgenticTimelineItem[];
  skillDisplayById: AIChatSkillDisplayMap;
  defaultOpen?: boolean;
  showMemoryKey?: boolean;
}

interface SkillTimelineViewModel {
  item: Extract<AIChatAgenticTimelineItem, { type: 'skill_event' }>;
  title: string;
  detail?: string;
  skill: AIChatSkillDisplayInfo;
  tone: TimelineTone;
}

type MemoryTimelineItem = Extract<AIChatAgenticTimelineItem, { type: 'memory_event' }>;

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
    if (tone === 'running') {
      return t('consoleChat.skills.agentic.loadingSkill', { skill: skill.label });
    }
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

function memoryEventContent(item: MemoryTimelineItem): string {
  return (item.event.content ?? item.event.content_preview ?? '').trim();
}

function memoryEventTitle(item: MemoryTimelineItem, locale: string, showMemoryKey: boolean): string {
  return getAIChatUserMemoryMutationTitle(item.event.action, locale, {
    content: item.event.content_preview || item.event.content,
    entryId: item.event.entry_id ?? (showMemoryKey ? item.event.key : undefined),
  });
}

function MemoryTimelineRow({
  item,
  showMemoryKey,
}: {
  item: MemoryTimelineItem;
  showMemoryKey: boolean;
}) {
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(false);
  const content = memoryEventContent(item);
  const canExpand = Boolean(
    content ||
      (showMemoryKey && item.event.key) ||
      item.event.category ||
      item.event.memory_type
  );

  return (
    <div className="rounded-md border border-emerald-500/20 bg-emerald-500/5 text-xs text-foreground">
      <button
        type="button"
        className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left"
        onClick={() => canExpand && setIsOpen(open => !open)}
        aria-expanded={isOpen}
      >
        <span className="flex size-5 shrink-0 items-center justify-center rounded-full border border-emerald-500/30 bg-background text-emerald-600">
          <CheckCircle2 className="size-3.5" />
        </span>
        <span className="min-w-0 flex-1 truncate">
          {memoryEventTitle(item, locale, showMemoryKey)}
        </span>
        {showMemoryKey && item.event.key ? (
          <span className="max-w-32 shrink-0 truncate rounded border border-emerald-500/20 bg-background/70 px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
            {item.event.key}
          </span>
        ) : null}
        {canExpand ? (
          <ChevronDown
            className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', {
              'rotate-180': isOpen,
            })}
          />
        ) : null}
      </button>
      {isOpen ? (
        <div className="space-y-2 border-t border-emerald-500/15 bg-background/70 px-2.5 py-2">
          {content ? (
            <div className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-background p-2 leading-relaxed text-foreground/85">
              {content}
            </div>
          ) : null}
          {item.event.category || item.event.memory_type ? (
            <div className="flex flex-wrap gap-1.5 text-[11px] text-muted-foreground">
              {item.event.category ? (
                <span className="rounded border bg-background/80 px-1.5 py-0.5">
                  {item.event.category}
                </span>
              ) : null}
              {item.event.memory_type ? (
                <span className="rounded border bg-background/80 px-1.5 py-0.5">
                  {item.event.memory_type}
                </span>
              ) : null}
            </div>
          ) : null}
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

function isMemoryEventItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is Extract<AIChatAgenticTimelineItem, { type: 'memory_event' }> {
  return 'type' in item && item.type === 'memory_event';
}

function isTransientProgressItem(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>
) {
  return item.transient === true || Boolean(item.phase && !item.content.trim());
}

function stableIndex(value: string, length: number): number {
  if (length <= 0) return 0;
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  return hash % length;
}

function buildProgressText(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>,
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator
) {
  if (item.phase !== 'tool_planning') {
    if (item.phase === 'planning') {
      return t('consoleChat.skills.agentic.preparingAction');
    }
    return item.content;
  }

  const skill = item.skill_id
    ? skillDisplayById[item.skill_id] ?? getFallbackAIChatSkillDisplayInfo(item.skill_id, locale)
    : null;
  const tool =
    item.skill_id && item.tool_name
      ? getAIChatSkillToolDisplayName(item.skill_id, item.tool_name, locale) || item.tool_name
      : item.tool_name;

  if (skill && tool) {
    return t('consoleChat.skills.agentic.preparingTool', { skill: skill.label, tool });
  }
  if (skill) {
    return t('consoleChat.skills.agentic.preparingSkill', { skill: skill.label });
  }
  return t('consoleChat.skills.agentic.preparingAction');
}

function buildTransientProgressText(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>,
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator
) {
  if (item.phase === 'tool_planning' && (item.skill_id || item.tool_name)) {
    return buildProgressText(item, skillDisplayById, locale, t);
  }
  const key =
    TRANSIENT_PROGRESS_TEXT_KEYS[
      stableIndex(item.event_id ?? item.id, TRANSIENT_PROGRESS_TEXT_KEYS.length)
    ];
  return t(key);
}

export function AIChatAgenticTimeline({
  timeline,
  skillDisplayById,
  defaultOpen = true,
  showMemoryKey = true,
}: AIChatAgenticTimelineProps) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(defaultOpen);

  const events = useMemo(
    () =>
      timeline.map(item => {
        if (item.type === 'progress_text') return item;
        if (item.type === 'intermediate_answer') return item;
        if (item.type === 'memory_event') return item;

        const skillId = item.invocation.skill_id || t('consoleChat.skills.trace.unknownSkill');
        const skill =
          skillDisplayById[skillId] ?? getFallbackAIChatSkillDisplayInfo(skillId, locale);
        const tone = getInvocationTone(item.invocation);

        return {
          item,
          skill,
          tone,
          title: buildSkillTitle(item.invocation, skill, tone, locale, t),
          detail:
            getAIChatSkillResultDisplay(item.invocation, locale) ||
            item.invocation.message ||
            item.invocation.error,
        };
      }),
    [locale, skillDisplayById, t, timeline]
  );

  if (events.length === 0) return null;

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen} className="mb-3 w-full min-w-0 max-w-full">
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
              isTransientProgressItem(item) ? (
                <div
                  key={item.id}
                  className="border-l-2 border-muted-foreground/15 py-0.5 pl-3 text-xs text-muted-foreground/70 animate-pulse"
                >
                  <span>{buildTransientProgressText(item, skillDisplayById, locale, t)}</span>
                </div>
              ) : (
                <div
                  key={item.id}
                  className={cn(
                    assistantMarkdownClassName,
                    'border-l-2 border-muted-foreground/20 pl-3 text-foreground'
                  )}
                >
                  <MarkdownViewer
                    className="md-viewer break-words"
                    content={buildProgressText(item, skillDisplayById, locale, t)}
                    renderIdentity={item.id}
                  />
                </div>
              )
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
                  <MarkdownViewer
                    className="md-viewer break-words"
                    content={item.content}
                    isStreaming={item.status === 'streaming'}
                    renderIdentity={item.answer_id || item.id}
                  />
                </div>
              </div>
            ) : isMemoryEventItem(item) ? (
              <MemoryTimelineRow key={item.id} item={item} showMemoryKey={showMemoryKey} />
            ) : (
              <SkillTimelineRow key={item.item.id} event={item} />
            )
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
