'use client';

import React from 'react';
import { CircleAlert, Clock, Link2, Megaphone, Variable } from 'lucide-react';

import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type { VariableInsertValue } from '@/components/workflow/common/workflow-value-inserter/variable-item';
import { WorkflowValueEditor, type WorkflowValueEditorHandle } from '@/components/workflow/ui';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import {
  getAnnouncementTimeoutMaxDuration,
  normalizeAnnouncementNodeData,
  type AnnouncementNodeData,
  type AnnouncementTimeoutUnit,
} from '../config';

interface AnnouncementManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

function Section({
  title,
  description,
  children,
  className,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('space-y-3', className)}>
      <div className="flex min-w-0 items-center gap-1.5">
        <h3 className="text-sm font-semibold text-foreground">{title}</h3>
        {description ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="inline-flex size-5 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
                aria-label={description}
              >
                <CircleAlert className="size-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="max-w-72 leading-5">{description}</TooltipContent>
          </Tooltip>
        ) : null}
      </div>
      {children}
    </section>
  );
}

function IntroCard() {
  const t = useT('nodes');
  const items = [
    {
      icon: Megaphone,
      title: t('announcement.intro.createLink.title'),
      description: t('announcement.intro.createLink.description'),
    },
    {
      icon: Variable,
      title: t('announcement.intro.variables.title'),
      description: t('announcement.intro.variables.description'),
    },
    {
      icon: Clock,
      title: t('announcement.intro.expiration.title'),
      description: t('announcement.intro.expiration.description'),
    },
    {
      icon: Link2,
      title: t('announcement.intro.outputs.title'),
      description: t('announcement.intro.outputs.description'),
    },
  ];

  return (
    <div className="rounded-lg border bg-muted/40 p-3">
      <div className="text-sm font-semibold text-foreground">{t('announcement.intro.title')}</div>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">
        {t('announcement.intro.description')}
      </p>
      <div className="mt-3 grid gap-2">
        {items.map(item => {
          const Icon = item.icon;
          return (
            <div key={item.title} className="flex gap-2 rounded-md bg-background/70 p-2">
              <div className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                <Icon className="size-3.5" />
              </div>
              <div className="min-w-0">
                <div className="text-xs font-medium text-foreground">{item.title}</div>
                <div className="mt-0.5 text-xs leading-5 text-muted-foreground">
                  {item.description}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function AnnouncementManager({
  id: nodeId,
  className,
  readOnly = false,
}: AnnouncementManagerProps) {
  const t = useT('nodes');
  const titleEditorRef = React.useRef<WorkflowValueEditorHandle>(null);
  const contentEditorRef = React.useRef<WorkflowValueEditorHandle>(null);
  const nodeData = useNodeData<AnnouncementNodeData>(nodeId);
  const updateData = useNodeDataUpdate<AnnouncementNodeData>(nodeId);
  const outputs = useNodeOutputVariables(nodeId);
  const data = React.useMemo(() => normalizeAnnouncementNodeData(nodeData), [nodeData]);
  const [timeoutDurationInput, setTimeoutDurationInput] = React.useState(
    String(data.timeout.duration)
  );

  React.useEffect(() => {
    setTimeoutDurationInput(String(data.timeout.duration));
  }, [data.timeout.duration]);

  const updateAnnouncement = React.useCallback(
    (updater: (current: AnnouncementNodeData) => AnnouncementNodeData) => {
      if (readOnly) return;
      updateData(updater(data));
    },
    [data, readOnly, updateData]
  );

  const handleTitleVariableInsert = React.useCallback(
    (value: VariableInsertValue) => {
      if (readOnly) return;
      const key =
        value.sourceId === 'sys' && value.key.startsWith('sys.') ? value.key.slice(4) : value.key;
      titleEditorRef.current?.insertToken(value.sourceId, key);
      titleEditorRef.current?.focus();
    },
    [readOnly]
  );

  const handleContentVariableInsert = React.useCallback(
    (value: VariableInsertValue) => {
      if (readOnly) return;
      const key =
        value.sourceId === 'sys' && value.key.startsWith('sys.') ? value.key.slice(4) : value.key;
      contentEditorRef.current?.insertToken(value.sourceId, key);
      contentEditorRef.current?.focus();
    },
    [readOnly]
  );

  const updateTimeoutDuration = React.useCallback(
    (raw: string) => {
      setTimeoutDurationInput(raw);
      const value = Number(raw);
      if (!Number.isInteger(value) || value <= 0) return;
      const maxDuration = getAnnouncementTimeoutMaxDuration(data.timeout.unit);
      if (value > maxDuration) return;
      updateAnnouncement(current => ({
        ...current,
        timeout: { ...current.timeout, duration: value },
      }));
    },
    [data.timeout.unit, updateAnnouncement]
  );

  const restoreTimeoutDurationInput = React.useCallback(() => {
    setTimeoutDurationInput(String(data.timeout.duration));
  }, [data.timeout.duration]);

  const applyPreset = React.useCallback(
    (duration: number, unit: AnnouncementTimeoutUnit) => {
      setTimeoutDurationInput(String(duration));
      updateAnnouncement(current => ({
        ...current,
        timeout: { ...current.timeout, duration, unit },
      }));
    },
    [updateAnnouncement]
  );

  const timeoutMaxDuration = getAnnouncementTimeoutMaxDuration(data.timeout.unit);

  return (
    <div className={cn('space-y-5', className)}>
      <IntroCard />

      <Section
        title={t('announcement.section.title')}
        description={t('announcement.sectionHelp.title')}
      >
        <WorkflowValueInserter
          nodeId={nodeId}
          className="w-full"
          onInsert={handleTitleVariableInsert}
          disabled={readOnly}
          defaultCollapsed={false}
        />
        <WorkflowValueEditor
          ref={titleEditorRef}
          value={data.announcement.title}
          onChange={title =>
            updateAnnouncement(current => ({
              ...current,
              announcement: { ...current.announcement, title },
            }))
          }
          readOnly={readOnly}
          nodeId={nodeId}
          placeholder={t('announcement.placeholders.title')}
          editorClassName="min-h-10"
        />
      </Section>

      <Section
        title={t('announcement.section.content')}
        description={t('announcement.sectionHelp.content')}
      >
        <WorkflowValueInserter
          nodeId={nodeId}
          className="w-full"
          onInsert={handleContentVariableInsert}
          disabled={readOnly}
          defaultCollapsed={false}
        />
        <WorkflowValueEditor
          ref={contentEditorRef}
          value={data.announcement.content}
          onChange={content =>
            updateAnnouncement(current => ({
              ...current,
              announcement: { ...current.announcement, content },
            }))
          }
          readOnly={readOnly}
          nodeId={nodeId}
          placeholder={t('announcement.placeholders.content')}
          editorClassName="min-h-[160px]"
        />
      </Section>

      <Section
        title={t('announcement.section.timeout')}
        description={t('announcement.sectionHelp.timeout')}
      >
        <div className="grid grid-cols-[1fr_120px] gap-2">
          <Input
            type="number"
            min={1}
            max={timeoutMaxDuration}
            step={1}
            value={timeoutDurationInput}
            disabled={readOnly}
            onChange={event => updateTimeoutDuration(event.target.value)}
            onBlur={restoreTimeoutDurationInput}
            error={
              timeoutDurationInput.trim() !== '' &&
              Number(timeoutDurationInput) > timeoutMaxDuration
            }
            errorText={
              timeoutDurationInput.trim() !== '' &&
              Number(timeoutDurationInput) > timeoutMaxDuration
                ? t('announcement.validation.timeoutDurationTooLong')
                : undefined
            }
          />
          <Select
            value={data.timeout.unit}
            disabled={readOnly}
            onValueChange={value => {
              const nextUnit = value as AnnouncementTimeoutUnit;
              const nextMax = getAnnouncementTimeoutMaxDuration(nextUnit);
              const nextDuration = Math.min(data.timeout.duration, nextMax);
              setTimeoutDurationInput(String(nextDuration));
              updateAnnouncement(current => ({
                ...current,
                timeout: { ...current.timeout, duration: nextDuration, unit: nextUnit },
              }));
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="hour">{t('announcement.timeout.hour')}</SelectItem>
              <SelectItem value="day">{t('announcement.timeout.day')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid grid-cols-3 gap-2">
          <button
            type="button"
            disabled={readOnly}
            className="rounded-md border px-3 py-2 text-xs hover:bg-muted disabled:opacity-50"
            onClick={() => applyPreset(1, 'day')}
          >
            {t('announcement.presets.oneDay')}
          </button>
          <button
            type="button"
            disabled={readOnly}
            className="rounded-md border px-3 py-2 text-xs hover:bg-muted disabled:opacity-50"
            onClick={() => applyPreset(3, 'day')}
          >
            {t('announcement.presets.threeDays')}
          </button>
          <button
            type="button"
            disabled={readOnly}
            className="rounded-md border px-3 py-2 text-xs hover:bg-muted disabled:opacity-50"
            onClick={() => applyPreset(7, 'day')}
          >
            {t('announcement.presets.oneWeek')}
          </button>
        </div>
      </Section>

      <Section title={t('common.outputVariables')}>
        <OutputVariablesView variables={outputs} />
      </Section>
    </div>
  );
}

export default AnnouncementManager;
