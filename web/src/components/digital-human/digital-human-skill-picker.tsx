'use client';

import { useDeferredValue, useMemo, useState } from 'react';
import { AlertCircle, Check, Puzzle, RefreshCw, SearchX, Trash2 } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import { ResourceBindingCard } from '@/components/common/resource-binding';
import {
  ResourceSelectionCardIcon,
  ResourceSelectionDialog,
  ResourceSelectionEmptyState,
  ResourceSelectionGrid,
  ResourceSelectionSkeleton,
} from '@/components/common/resource-selection-dialog';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import {
  SkillAvailabilityNotice,
  type SkillCodeExecution,
} from '@/components/skills/skill-availability-notice';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { DigitalHumanResourceCandidate } from '@/services/types/digital-human';

/**
 * Skill presentation shared by every digital human surface that lets a user
 * pick a skill (the creation form and the draft binding editor).
 *
 * Keeping the cards, the source filter and the client-side matching in one
 * module is what makes both pickers show the same localized label, the same
 * description, and the same search behaviour.
 */

export type DigitalHumanSkillSourceFilter = 'all' | 'system' | 'custom';

export interface DigitalHumanSkillDisplaySource {
  resource_id: string;
  display_name?: string;
  description?: string;
  metadata: Record<string, unknown>;
}

function metadataString(metadata: Record<string, unknown>, key: string) {
  const value = metadata[key];
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function metadataBoolean(metadata: Record<string, unknown>, key: string) {
  return metadata[key] === true;
}

function skillDisplayMetadata(value: unknown) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  return value as AIChatSkillMetadata['display'];
}

/** The workspace source a candidate comes from, used by the source filter. */
export function digitalHumanSkillSource(item: DigitalHumanSkillDisplaySource): 'system' | 'custom' {
  return metadataString(item.metadata, 'source') === 'custom' ? 'custom' : 'system';
}

/** Project a binding candidate onto the shared AIChat skill metadata shape. */
export function digitalHumanSkillToMetadata(item: DigitalHumanSkillDisplaySource): AIChatSkillMetadata {
  const displayName = item.display_name?.trim() || item.resource_id;
  return {
    skill_id: item.resource_id,
    source: digitalHumanSkillSource(item),
    name: displayName,
    description: item.description?.trim() || metadataString(item.metadata, 'description') || '',
    when_to_use: metadataString(item.metadata, 'when_to_use') || '',
    runtime_type: (metadataString(item.metadata, 'runtime_type') ||
      'prompt') as AIChatSkillMetadata['runtime_type'],
    enabled: true,
    display: skillDisplayMetadata(item.metadata.display),
    has_tools: metadataBoolean(item.metadata, 'has_tools'),
    has_references: metadataBoolean(item.metadata, 'has_references'),
    has_scripts: metadataBoolean(item.metadata, 'has_scripts'),
    scripts_supported: metadataBoolean(item.metadata, 'scripts_supported'),
    max_calls_per_turn: 0,
    timeout_seconds: 0,
  };
}

/** Localized label, description, icon and when-to-use copy for one skill. */
export function getDigitalHumanSkillDisplay(item: DigitalHumanSkillDisplaySource, locale: string) {
  return getAIChatSkillDisplayInfo(digitalHumanSkillToMetadata(item), locale);
}

/**
 * Client-side skill matching.
 *
 * Skills are filtered locally because the candidate list is small and already
 * scoped by the server, so search must behave identically on every surface.
 */
export function filterDigitalHumanSkills(
  candidates: readonly DigitalHumanResourceCandidate[],
  options: {
    search: string;
    source: DigitalHumanSkillSourceFilter;
    locale: string;
  }
): DigitalHumanResourceCandidate[] {
  const search = options.search.trim().toLocaleLowerCase();
  return candidates.filter(candidate => {
    if (options.source !== 'all' && digitalHumanSkillSource(candidate) !== options.source) {
      return false;
    }
    if (!search) return true;
    const display = getDigitalHumanSkillDisplay(candidate, options.locale);
    return [
      display.label,
      display.description,
      display.whenToUse,
      candidate.display_name,
      candidate.description,
      candidate.resource_id,
    ].some(value => value?.toLocaleLowerCase().includes(search));
  });
}

/** Source filter control, matching the draft editor's toolbar. */
export function DigitalHumanSkillSourceSelect({
  value,
  onChange,
}: {
  value: DigitalHumanSkillSourceFilter;
  onChange: (value: DigitalHumanSkillSourceFilter) => void;
}) {
  const t = useT('digitalHumans');
  return (
    <Select value={value} onValueChange={next => onChange(next as DigitalHumanSkillSourceFilter)}>
      <SelectTrigger className="h-9 w-full shrink-0 bg-background sm:w-48">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t('bindings.skills.all')}</SelectItem>
        <SelectItem value="system">{t('bindings.skills.system')}</SelectItem>
        <SelectItem value="custom">{t('bindings.skills.custom')}</SelectItem>
      </SelectContent>
    </Select>
  );
}

/**
 * The skill cards themselves.
 *
 * `selectedIds` holds resource ids, which is enough because skills have no
 * parent resource; the cards stay usable while the surrounding surface decides
 * what saving means.
 */
export function DigitalHumanSkillGrid({
  candidates,
  selectedIds,
  onToggle,
  disabled = false,
  locale,
}: {
  candidates: readonly DigitalHumanResourceCandidate[];
  selectedIds: readonly string[];
  onToggle: (candidate: DigitalHumanResourceCandidate) => void;
  disabled?: boolean;
  locale: string;
}) {
  const t = useT('digitalHumans');
  const selected = new Set(selectedIds);

  return (
    <ResourceSelectionGrid>
      {candidates.map(candidate => {
        const isSelected = selected.has(candidate.resource_id);
        const display = getDigitalHumanSkillDisplay(candidate, locale);
        return (
          <button
            key={candidate.resource_id}
            type="button"
            aria-pressed={isSelected}
            disabled={disabled || candidate.available === false}
            className={cn(
              'flex min-h-36 cursor-pointer flex-col rounded-lg border bg-background p-3.5 text-left shadow-sm transition-colors motion-reduce:transition-none',
              isSelected
                ? 'border-primary bg-primary/5 hover:bg-primary/10'
                : 'hover:border-primary/40 hover:bg-muted/20',
              candidate.available === false && 'cursor-not-allowed opacity-60'
            )}
            onClick={() => onToggle(candidate)}
          >
            <span className="flex min-h-8 items-center gap-3">
              <ResourceSelectionCardIcon>
                <AIChatSkillIcon icon={display.icon} />
              </ResourceSelectionCardIcon>
              <span className="flex min-h-8 min-w-0 flex-1 items-center">
                <span className="block truncate text-sm font-semibold">{display.label}</span>
              </span>
              <span
                className={cn(
                  'flex size-5 shrink-0 items-center justify-center rounded-full border',
                  isSelected ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
                )}
              >
                {isSelected ? <Check className="size-3.5" /> : null}
              </span>
            </span>
            <span className="mt-2.5 line-clamp-3 text-xs leading-5 text-muted-foreground">
              {display.description || candidate.description || t('bindings.noDescription')}
            </span>
          </button>
        );
      })}
    </ResourceSelectionGrid>
  );
}

/**
 * One already-chosen skill, rendered with the configuration page's bound
 * resource card so a selection looks the same wherever it is reviewed.
 */
export function DigitalHumanSkillBindingCard({
  item,
  locale,
  removeLabel,
  onRemove,
  disabled = false,
}: {
  item: DigitalHumanSkillDisplaySource;
  locale: string;
  removeLabel: string;
  onRemove: () => void;
  disabled?: boolean;
}) {
  const display = getDigitalHumanSkillDisplay(item, locale);
  return (
    <ResourceBindingCard
      icon={<AIChatSkillIcon icon={display.icon} className="size-4" />}
      title={display.label}
      description={display.description}
      action={
        <Button
          type="button"
          variant="ghost"
          size="sm"
          isIcon
          className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
          aria-label={removeLabel}
          title={removeLabel}
          disabled={disabled}
          onClick={onRemove}
        >
          <Trash2 className="size-4" />
        </Button>
      }
    />
  );
}

/**
 * Dialog body for the skill picker.
 *
 * The draft editor and the creation form render this same body, so loading,
 * failure, empty, no-match and the grid all behave identically.
 */
export function DigitalHumanSkillDialogBody({
  candidates,
  selectedIds,
  onToggle,
  locale,
  loading,
  error,
  search,
  emptyTitle,
  emptyDescription,
  skillNoMatchLabel,
  retryLabel,
  onRetry,
}: {
  candidates: readonly DigitalHumanResourceCandidate[];
  selectedIds: readonly string[];
  onToggle: (candidate: DigitalHumanResourceCandidate) => void;
  locale: string;
  loading: boolean;
  error: boolean;
  search: boolean;
  emptyTitle: string;
  emptyDescription: string;
  skillNoMatchLabel: string;
  retryLabel: string;
  onRetry: () => void;
}) {
  if (loading) return <ResourceSelectionSkeleton />;
  if (error) {
    return (
      <ResourceSelectionEmptyState
        icon={<AlertCircle />}
        title={emptyTitle}
        description={emptyDescription}
        action={
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RefreshCw className="size-4" />
            {retryLabel}
          </Button>
        }
      />
    );
  }
  if (candidates.length === 0) {
    return (
      <ResourceSelectionEmptyState
        icon={search ? <SearchX /> : <Puzzle />}
        title={search ? skillNoMatchLabel : emptyTitle}
        description={emptyDescription}
        variant={search ? 'search' : 'resource'}
      />
    );
  }
  return (
    <DigitalHumanSkillGrid
      candidates={candidates}
      selectedIds={selectedIds}
      onToggle={onToggle}
      locale={locale}
    />
  );
}

/**
 * The Add-Skill dialog: the same shell, copy, search and source filter the
 * configuration page opens, usable while a digital human does not exist yet.
 */
export function DigitalHumanSkillPickerDialog({
  open,
  onOpenChange,
  candidates,
  selectedIds,
  onToggle,
  onSave,
  saving = false,
  locale,
  loading,
  error,
  onRetry,
  expectedIds,
  codeExecution,
  onSkillEnabled,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  candidates: readonly DigitalHumanResourceCandidate[];
  selectedIds: readonly string[];
  onToggle: (candidate: DigitalHumanResourceCandidate) => void;
  onSave: () => void;
  saving?: boolean;
  locale: string;
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  /** Skills a template asks for, reported when missing. */
  expectedIds?: readonly string[];
  codeExecution?: SkillCodeExecution;
  /** Refresh candidates (and select) after a skill was enabled here. */
  onSkillEnabled?: (skillId: string) => void | Promise<unknown>;
}) {
  const t = useT('digitalHumans');
  const [search, setSearch] = useState('');
  const [source, setSource] = useState<DigitalHumanSkillSourceFilter>('all');
  const deferredSearch = useDeferredValue(search.trim());
  const visible = useMemo(
    () => filterDigitalHumanSkills(candidates, { search: deferredSearch, source, locale }),
    [candidates, deferredSearch, source, locale]
  );

  return (
    <ResourceSelectionDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('bindings.sections.skills.pickerTitle')}
      description={t('bindings.skills.dialogDescription')}
      selectedCountLabel={t('bindings.selectedCount', { count: selectedIds.length })}
      search={search}
      searchPlaceholder={t('bindings.skills.searchPlaceholder')}
      toolbar={<DigitalHumanSkillSourceSelect value={source} onChange={setSource} />}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={onSave} disabled={saving}>
            {t('bindings.skills.save')}
          </Button>
        </>
      }
      onChangeSearch={setSearch}
    >
      <div className="mb-3">
        <SkillAvailabilityNotice
          offeredIds={candidates.map(candidate => candidate.resource_id)}
          expectedIds={expectedIds}
          selectedIds={selectedIds}
          codeExecution={codeExecution}
          onEnabled={onSkillEnabled}
        />
      </div>
      <DigitalHumanSkillDialogBody
        candidates={visible}
        selectedIds={selectedIds}
        onToggle={onToggle}
        locale={locale}
        loading={loading}
        error={error}
        search={Boolean(deferredSearch)}
        emptyTitle={t('bindings.noCandidates')}
        emptyDescription={t('bindings.noCandidatesDescription')}
        skillNoMatchLabel={t('bindings.skills.noMatch')}
        retryLabel={t('bindings.retry')}
        onRetry={onRetry}
      />
    </ResourceSelectionDialog>
  );
}
