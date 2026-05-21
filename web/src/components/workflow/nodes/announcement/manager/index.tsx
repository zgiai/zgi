'use client';

import React from 'react';

import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type { VariableInsertValue } from '@/components/workflow/common/workflow-value-inserter/variable-item';
import { WorkflowValueEditor, type WorkflowValueEditorHandle } from '@/components/workflow/ui';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
  children,
  className,
}: {
  title: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('space-y-3', className)}>
      <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      {children}
    </section>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return <Label className="text-xs font-medium text-muted-foreground">{children}</Label>;
}

export function AnnouncementManager({
  id: nodeId,
  className,
  readOnly = false,
}: AnnouncementManagerProps) {
  const t = useT('nodes');
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
      <Section title={t('announcement.section.content')}>
        <WorkflowValueInserter
          nodeId={nodeId}
          className="w-full"
          onInsert={handleContentVariableInsert}
          disabled={readOnly}
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

      <Section title={t('announcement.section.timeout')}>
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

      <FieldLabel>{t('announcement.hint.publicLink')}</FieldLabel>
    </div>
  );
}

export default AnnouncementManager;
