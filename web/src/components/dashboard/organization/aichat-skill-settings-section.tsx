'use client';

import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { AlertCircle, CheckCircle2, Loader2, Trash2, Upload, Wrench } from 'lucide-react';
import { toast } from 'sonner';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import {
  getAIChatSkillDisplayInfo,
  isHiddenSystemSkill,
  type AIChatSkillDisplayInfo,
} from '@/components/chat/variants/aichat/skill-display';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SearchInput } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  useDeleteAIChatSkill,
  useAIChatSkillConfig,
  useAIChatSkills,
  useCancelImportAIChatSkillPreview,
  useConfirmImportAIChatSkill,
  usePreviewImportAIChatSkill,
  useUpdateAIChatSkillConfig,
} from '@/hooks/aichat/use-aichat-skills';
import { AICHAT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT, type DashboardSuffix } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type {
  AIChatSkillMetadata,
  AIChatImportSkillPreview,
  AIChatSkillRuntimeType,
  AIChatSkillSource,
} from '@/services/types/aichat';

const AUTO_SAVE_DELAY_MS = 450;
const SKILL_CARD_GRID_CLASS =
  'grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4';
const SYSTEM_SKILL_NAME_CONFLICT_ERROR =
  'This skill name is reserved by a built-in system skill. Please rename your custom skill and try again.';

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';
type RuntimeFilter = 'all' | AIChatSkillRuntimeType;
type StatusFilter = 'all' | 'enabled' | 'disabled' | 'invalid';

const RUNTIME_LABEL_KEYS: Record<AIChatSkillRuntimeType, DashboardSuffix> = {
  tool: 'organization.aichatSkills.runtime.tool',
  prompt: 'organization.aichatSkills.runtime.prompt',
  hybrid: 'organization.aichatSkills.runtime.hybrid',
};

const STATUS_LABEL_KEYS = {
  enabled: 'organization.aichatSkills.status.enabled',
  disabled: 'organization.aichatSkills.status.disabled',
  invalid: 'organization.aichatSkills.status.invalid',
} as const satisfies Record<string, DashboardSuffix>;

const SCRIPT_STATUS_LABEL_KEYS = {
  runnable: 'organization.aichatSkills.scriptStatus.runnable',
  unsupported: 'organization.aichatSkills.scriptStatus.unsupported',
} as const satisfies Record<string, DashboardSuffix>;

const AUTO_SAVE_LABEL_KEYS = {
  idle: 'organization.aichatSkills.autoSave.ready',
  saving: 'organization.aichatSkills.autoSave.saving',
  saved: 'organization.aichatSkills.autoSave.saved',
  error: 'organization.aichatSkills.autoSave.error',
} as const satisfies Record<SaveStatus, DashboardSuffix>;

function normalizeSkillIds(ids: string[]): string[] {
  return Array.from(new Set(ids.map(id => id.trim().toLowerCase()).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b)
  );
}

function getInitialEnabledSkillIds(
  skills: AIChatSkillMetadata[],
  configIds: string[] | undefined
): string[] {
  const manageableIds = new Set(skills.map(skill => skill.skill_id.trim().toLowerCase()));
  const ids = configIds
    ? configIds.filter(skillId => manageableIds.has(skillId.trim().toLowerCase()))
    : skills.filter(skill => skill.enabled).map(skill => skill.skill_id);
  return normalizeSkillIds(ids);
}

function getSkillIdsKey(ids: string[]): string {
  return normalizeSkillIds(ids).join('\u0000');
}

function getSkillSource(skill: AIChatSkillMetadata): AIChatSkillSource {
  return skill.source ?? 'system';
}

function isInvalidSkill(skill: AIChatSkillMetadata): boolean {
  return skill.status === 'invalid';
}

function getScriptStatusLabelKey(skill: AIChatSkillMetadata): DashboardSuffix | null {
  if (!skill.has_scripts) return null;
  return skill.scripts_supported
    ? SCRIPT_STATUS_LABEL_KEYS.runnable
    : SCRIPT_STATUS_LABEL_KEYS.unsupported;
}

function getFilterSearchText(
  skill: AIChatSkillMetadata,
  display: AIChatSkillDisplayInfo | undefined
): string {
  return [
    skill.skill_id,
    skill.name,
    skill.description,
    skill.when_to_use,
    display?.label,
    display?.description,
    display?.whenToUse,
    ...(display?.tags ?? []),
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
}

function filterSkills(
  skills: AIChatSkillMetadata[],
  displays: Record<string, AIChatSkillDisplayInfo>,
  enabledSkillIds: string[],
  searchQuery: string,
  runtimeFilter: RuntimeFilter,
  statusFilter: StatusFilter
): AIChatSkillMetadata[] {
  const query = searchQuery.trim().toLowerCase();
  const enabledSet = new Set(enabledSkillIds);

  return skills.filter(skill => {
    if (runtimeFilter !== 'all' && skill.runtime_type !== runtimeFilter) return false;

    const enabled = enabledSet.has(skill.skill_id);
    const invalid = isInvalidSkill(skill);
    if (statusFilter === 'enabled' && (!enabled || invalid)) return false;
    if (statusFilter === 'disabled' && (enabled || invalid)) return false;
    if (statusFilter === 'invalid' && !invalid) return false;

    if (!query) return true;
    return getFilterSearchText(skill, displays[skill.skill_id]).includes(query);
  });
}

function formatTabCount(filteredCount: number, totalCount: number, hasActiveFilters: boolean) {
  return hasActiveFilters ? `${filteredCount}/${totalCount}` : String(totalCount);
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value >= 10 || unitIndex === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}

function previewFiles(preview: AIChatImportSkillPreview | null) {
  return preview?.files ?? [];
}

function previewReferences(preview: AIChatImportSkillPreview | null) {
  return preview?.references ?? [];
}

function previewWarnings(preview: AIChatImportSkillPreview | null) {
  return preview?.warnings ?? [];
}

function previewValidationErrors(
  preview: AIChatImportSkillPreview | null,
  localize: (key: DashboardSuffix) => string
) {
  return (preview?.validation_errors ?? []).map(error =>
    error === SYSTEM_SKILL_NAME_CONFLICT_ERROR
      ? localize('organization.aichatSkills.importPreview.systemSkillNameConflict')
      : error
  );
}

interface AIChatSkillCardProps {
  skill: AIChatSkillMetadata;
  display: AIChatSkillDisplayInfo;
  enabled: boolean;
  disabled: boolean;
  onToggle: (skillId: string, enabled: boolean) => void;
  onDelete: (skill: AIChatSkillMetadata) => void;
}

/**
 * @component AIChatSkillCard
 * @category Feature
 * @status Stable
 * @description Card item for scanning, enabling, and managing one AIChat Skill.
 * @usage Render within the organization AIChat Skill settings grid.
 * @example
 * <AIChatSkillCard skill={skill} display={display} enabled={true} disabled={false} onToggle={onToggle} onDelete={onDelete} />
 */
function AIChatSkillCard({
  skill,
  display,
  enabled,
  disabled,
  onToggle,
  onDelete,
}: AIChatSkillCardProps) {
  const t = useT('dashboard');
  const runtimeLabel = t(RUNTIME_LABEL_KEYS[skill.runtime_type]);
  const isCustom = getSkillSource(skill) === 'custom';
  const invalid = isInvalidSkill(skill);
  const scriptStatusLabelKey = getScriptStatusLabelKey(skill);

  return (
    <article
      className={cn(
        'flex h-full flex-col rounded-md border border-border bg-card p-3.5 shadow-sm transition-colors hover:border-primary/25',
        disabled || invalid ? 'opacity-75' : ''
      )}
    >
      <div className="flex items-start gap-3">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground">
          <AIChatSkillIcon icon={display.icon} className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="truncate text-sm font-semibold text-foreground">{display.label}</h3>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">{skill.skill_id}</p>
            </div>
            <Switch
              checked={enabled}
              disabled={disabled || invalid}
              aria-label={t('organization.aichatSkills.toggleAria', { skill: display.label })}
              onCheckedChange={checked => onToggle(skill.skill_id, checked)}
            />
          </div>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        <Badge variant="outline" className="rounded-md font-normal">
          {runtimeLabel}
        </Badge>
        <Badge
          variant={invalid ? 'destructive' : enabled ? 'success' : 'subtle'}
          className="rounded-md font-normal"
        >
          {t(
            invalid
              ? STATUS_LABEL_KEYS.invalid
              : enabled
                ? STATUS_LABEL_KEYS.enabled
                : STATUS_LABEL_KEYS.disabled
          )}
        </Badge>
        {scriptStatusLabelKey ? (
          <Badge
            variant={skill.scripts_supported ? 'success' : 'warning'}
            className="rounded-md font-normal"
          >
            {t(scriptStatusLabelKey)}
          </Badge>
        ) : null}
      </div>

      <p className="mt-2.5 line-clamp-2 min-h-10 text-sm leading-5 text-muted-foreground">
        {display.description}
      </p>

      {invalid && skill.validation_error ? (
        <div className="mt-2.5 rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs leading-5 text-destructive">
          {skill.validation_error}
        </div>
      ) : null}

      {display.tags.length > 0 ? (
        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {display.tags.map(tag => (
            <Badge key={tag} variant="subtle" className="rounded-md font-normal">
              {tag}
            </Badge>
          ))}
        </div>
      ) : null}

      {isCustom ? (
        <div className="mt-auto flex justify-end pt-3">
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            disabled={disabled}
            onClick={() => onDelete(skill)}
          >
            <Trash2 className="size-4" />
            {t('organization.aichatSkills.actions.delete')}
          </Button>
        </div>
      ) : null}
    </article>
  );
}

interface AutoSaveStatusIndicatorProps {
  status: SaveStatus;
}

/**
 * @component AutoSaveStatusIndicator
 * @category Feature
 * @status Stable
 * @description Lightweight status label for organization Skill auto-save progress.
 * @usage Render near the enabled count on the AIChat Skill settings page.
 * @example
 * <AutoSaveStatusIndicator status="saved" />
 */
function AutoSaveStatusIndicator({ status }: AutoSaveStatusIndicatorProps) {
  const t = useT('dashboard');
  const isSaving = status === 'saving';
  const isError = status === 'error';
  const Icon = isSaving ? Loader2 : isError ? AlertCircle : CheckCircle2;

  return (
    <span
      className={cn(
        'inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium',
        isError
          ? 'border-destructive/30 bg-destructive/10 text-destructive'
          : 'border-border bg-background text-muted-foreground'
      )}
    >
      <Icon className={cn('size-3.5', isSaving ? 'animate-spin' : '')} />
      {t(AUTO_SAVE_LABEL_KEYS[status])}
    </span>
  );
}

interface UseAIChatSkillConfigAutosaveOptions {
  initialEnabledSkillIds: string[];
  isLoading: boolean;
  save: (enabledSkillIds: string[]) => Promise<string[]>;
  onError: (error: unknown) => void;
}

function useAIChatSkillConfigAutosave({
  initialEnabledSkillIds,
  isLoading,
  save,
  onError,
}: UseAIChatSkillConfigAutosaveOptions) {
  const [enabledSkillIds, setEnabledSkillIds] = useState<string[]>([]);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  const confirmedSkillIdsRef = useRef<string[]>([]);
  const hasHydratedRef = useRef(false);
  const saveRef = useRef(save);
  const onErrorRef = useRef(onError);
  const draftKey = useMemo(() => getSkillIdsKey(enabledSkillIds), [enabledSkillIds]);
  const confirmedKey = getSkillIdsKey(confirmedSkillIdsRef.current);
  const latestDraftKeyRef = useRef(draftKey);

  latestDraftKeyRef.current = draftKey;

  useEffect(() => {
    saveRef.current = save;
  }, [save]);

  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  useEffect(() => {
    const normalizedInitialSkillIds = normalizeSkillIds(initialEnabledSkillIds);
    const initialKey = getSkillIdsKey(normalizedInitialSkillIds);
    const currentConfirmedKey = getSkillIdsKey(confirmedSkillIdsRef.current);

    if (hasHydratedRef.current && initialKey === currentConfirmedKey) return;

    hasHydratedRef.current = true;
    confirmedSkillIdsRef.current = normalizedInitialSkillIds;
    setEnabledSkillIds(normalizedInitialSkillIds);
    setSaveStatus('idle');
  }, [initialEnabledSkillIds]);

  useEffect(() => {
    if (isLoading || draftKey === confirmedKey) return;

    setSaveStatus('saving');

    const timeout = window.setTimeout(async () => {
      const requestedSkillIds = normalizeSkillIds(enabledSkillIds);
      const requestedKey = getSkillIdsKey(requestedSkillIds);

      try {
        const savedSkillIds = normalizeSkillIds(await saveRef.current(requestedSkillIds));
        const savedKey = getSkillIdsKey(savedSkillIds);

        if (latestDraftKeyRef.current !== requestedKey) return;

        confirmedSkillIdsRef.current = savedSkillIds;
        setEnabledSkillIds(current =>
          getSkillIdsKey(current) === requestedKey ? savedSkillIds : current
        );
        setSaveStatus(savedKey === requestedKey ? 'saved' : 'idle');
      } catch (error) {
        if (latestDraftKeyRef.current !== requestedKey) return;

        setEnabledSkillIds(confirmedSkillIdsRef.current);
        setSaveStatus('error');
        onErrorRef.current(error);
      }
    }, AUTO_SAVE_DELAY_MS);

    return () => {
      window.clearTimeout(timeout);
    };
  }, [confirmedKey, draftKey, enabledSkillIds, isLoading]);

  return {
    enabledSkillIds,
    setEnabledSkillIds,
    saveStatus,
    resetToServerValue: () => setEnabledSkillIds(confirmedSkillIdsRef.current),
  };
}

interface SkillFilterToolbarProps {
  searchQuery: string;
  runtimeFilter: RuntimeFilter;
  statusFilter: StatusFilter;
  hasActiveFilters: boolean;
  onSearchQueryChange: (value: string) => void;
  onRuntimeFilterChange: (value: RuntimeFilter) => void;
  onStatusFilterChange: (value: StatusFilter) => void;
  onClearFilters: () => void;
}

/**
 * @component SkillFilterToolbar
 * @category Feature
 * @status Stable
 * @description Search and filter controls for larger AIChat Skill catalogs.
 * @usage Render above Skill tab content to narrow the visible Skill cards.
 * @example
 * <SkillFilterToolbar searchQuery="" runtimeFilter="all" statusFilter="all" hasActiveFilters={false} onSearchQueryChange={setSearchQuery} onRuntimeFilterChange={setRuntimeFilter} onStatusFilterChange={setStatusFilter} onClearFilters={clearFilters} />
 */
function SkillFilterToolbar({
  searchQuery,
  runtimeFilter,
  statusFilter,
  hasActiveFilters,
  onSearchQueryChange,
  onRuntimeFilterChange,
  onStatusFilterChange,
  onClearFilters,
}: SkillFilterToolbarProps) {
  const t = useT('dashboard');

  return (
    <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
      <SearchInput
        value={searchQuery}
        onChange={event => onSearchQueryChange(event.target.value)}
        placeholder={t('organization.aichatSkills.filters.searchPlaceholder')}
        aria-label={t('organization.aichatSkills.filters.searchAria')}
        className="rounded-md lg:w-[360px]"
      />
      <div className="grid gap-2 sm:grid-cols-2 lg:flex lg:shrink-0">
        <Select
          value={runtimeFilter}
          onValueChange={value => onRuntimeFilterChange(value as RuntimeFilter)}
        >
          <SelectTrigger
            className="rounded-md bg-background lg:w-40"
            aria-label={t('organization.aichatSkills.filters.runtimeAria')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('organization.aichatSkills.filters.allRuntime')}</SelectItem>
            <SelectItem value="tool">{t(RUNTIME_LABEL_KEYS.tool)}</SelectItem>
            <SelectItem value="prompt">{t(RUNTIME_LABEL_KEYS.prompt)}</SelectItem>
            <SelectItem value="hybrid">{t(RUNTIME_LABEL_KEYS.hybrid)}</SelectItem>
          </SelectContent>
        </Select>

        <Select
          value={statusFilter}
          onValueChange={value => onStatusFilterChange(value as StatusFilter)}
        >
          <SelectTrigger
            className="rounded-md bg-background lg:w-40"
            aria-label={t('organization.aichatSkills.filters.statusAria')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('organization.aichatSkills.filters.allStatus')}</SelectItem>
            <SelectItem value="enabled">{t(STATUS_LABEL_KEYS.enabled)}</SelectItem>
            <SelectItem value="disabled">{t(STATUS_LABEL_KEYS.disabled)}</SelectItem>
            <SelectItem value="invalid">{t(STATUS_LABEL_KEYS.invalid)}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {hasActiveFilters ? (
        <Button variant="ghost" size="sm" onClick={onClearFilters}>
          {t('organization.aichatSkills.actions.clearFilters')}
        </Button>
      ) : null}
    </div>
  );
}

interface SkillImportPreviewDialogProps {
  preview: AIChatImportSkillPreview | null;
  open: boolean;
  loading: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

function SkillImportPreviewDialog({
  preview,
  open,
  loading,
  onOpenChange,
  onConfirm,
}: SkillImportPreviewDialogProps) {
  const t = useT('dashboard');
  const skill = preview?.skill;
  const canImport = Boolean(preview?.can_import && preview.import_id);
  const files = previewFiles(preview);
  const references = previewReferences(preview);
  const warnings = previewWarnings(preview);
  const validationErrors = previewValidationErrors(preview, t);
  const existingSkillName =
    preview?.existing_skill?.name || preview?.existing_skill?.skill_id || skill?.skill_id || '';
  const scriptStatusLabelKey = skill ? getScriptStatusLabelKey(skill) : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('organization.aichatSkills.importPreview.title')}</DialogTitle>
          <DialogDescription>
            {t('organization.aichatSkills.importPreview.description')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {skill ? (
            <div className="rounded-md border p-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <h3 className="truncate text-sm font-semibold text-foreground">
                    {skill.name || skill.skill_id}
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground">{skill.skill_id}</p>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  <Badge variant="outline" className="w-fit rounded-md font-normal">
                    {t(RUNTIME_LABEL_KEYS[skill.runtime_type])}
                  </Badge>
                  {scriptStatusLabelKey ? (
                    <Badge
                      variant={skill.scripts_supported ? 'success' : 'warning'}
                      className="w-fit rounded-md font-normal"
                    >
                      {t(scriptStatusLabelKey)}
                    </Badge>
                  ) : null}
                </div>
              </div>
              <p className="mt-3 text-sm leading-5 text-muted-foreground">{skill.description}</p>
            </div>
          ) : null}

          {preview ? (
            <div className="grid gap-2 sm:grid-cols-3">
              <div className="rounded-md border p-3">
                <p className="text-xs text-muted-foreground">
                  {t('organization.aichatSkills.importPreview.fileCount')}
                </p>
                <p className="mt-1 text-sm font-medium">{preview.file_count}</p>
              </div>
              <div className="rounded-md border p-3">
                <p className="text-xs text-muted-foreground">
                  {t('organization.aichatSkills.importPreview.totalSize')}
                </p>
                <p className="mt-1 text-sm font-medium">{formatBytes(preview.total_size)}</p>
              </div>
              <div className="rounded-md border p-3">
                <p className="text-xs text-muted-foreground">
                  {t('organization.aichatSkills.importPreview.references')}
                </p>
                <p className="mt-1 text-sm font-medium">{references.length}</p>
              </div>
            </div>
          ) : null}

          {preview?.will_overwrite ? (
            <div className="rounded-md border border-amber-300/70 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200">
              <div className="flex gap-2">
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <div className="min-w-0">
                  <p className="font-medium">
                    {t('organization.aichatSkills.importPreview.overwriteTitle')}
                  </p>
                  <p className="mt-1 leading-5">
                    {t('organization.aichatSkills.importPreview.overwriteDescription', {
                      skill: existingSkillName,
                    })}
                  </p>
                </div>
              </div>
            </div>
          ) : null}

          {warnings.length ? (
            <div className="rounded-md border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200">
              {warnings.map(warning => (
                <p key={warning}>{warning}</p>
              ))}
            </div>
          ) : null}

          {validationErrors.length ? (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              {validationErrors.map(error => (
                <p key={error}>{error}</p>
              ))}
            </div>
          ) : null}

          {files.length ? (
            <div className="max-h-44 overflow-auto rounded-md border">
              {files.slice(0, 40).map(file => (
                <div
                  key={file.path}
                  className="flex items-center justify-between gap-3 border-b px-3 py-2 text-xs last:border-b-0"
                >
                  <span className="min-w-0 truncate text-muted-foreground">{file.path}</span>
                  <span className="shrink-0 text-muted-foreground">{formatBytes(file.size)}</span>
                </div>
              ))}
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" disabled={loading} onClick={() => onOpenChange(false)}>
            {t('organization.aichatSkills.importPreview.cancel')}
          </Button>
          <Button disabled={!canImport || loading} onClick={onConfirm}>
            {loading ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
            {loading
              ? t('organization.aichatSkills.actions.importing')
              : preview?.will_overwrite
                ? t('organization.aichatSkills.importPreview.confirmOverwrite')
                : t('organization.aichatSkills.importPreview.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * @component AIChatSkillSettingsSection
 * @category Feature
 * @status Stable
 * @description Organization-level AIChat Skill catalog and auto-saving enablement settings.
 * @usage Render inside organization management to manage AIChat skills.
 * @example
 * <AIChatSkillSettingsSection />
 */
export function AIChatSkillSettingsSection() {
  const t = useT('dashboard');
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const { data: skills = [], isLoading: isLoadingSkills, isError } = useAIChatSkills();
  const { data: config, isLoading: isLoadingConfig } = useAIChatSkillConfig();
  const updateConfig = useUpdateAIChatSkillConfig();
  const updateSkillConfig = updateConfig.mutateAsync;
  const previewImportSkill = usePreviewImportAIChatSkill();
  const confirmImportSkill = useConfirmImportAIChatSkill();
  const cancelImportPreview = useCancelImportAIChatSkillPreview();
  const deleteSkill = useDeleteAIChatSkill();
  const [searchQuery, setSearchQuery] = useState('');
  const [runtimeFilter, setRuntimeFilter] = useState<RuntimeFilter>('all');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [activeTab, setActiveTab] = useState<'system' | 'custom'>('system');
  const [skillToDelete, setSkillToDelete] = useState<AIChatSkillMetadata | null>(null);
  const [importPreview, setImportPreview] = useState<AIChatImportSkillPreview | null>(null);
  const [isImportPreviewOpen, setIsImportPreviewOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const importConfirmedRef = useRef(false);
  const manageableSkills = useMemo(
    () => skills.filter(skill => !isHiddenSystemSkill(skill.skill_id)),
    [skills]
  );

  const initialEnabledSkillIds = useMemo(
    () => getInitialEnabledSkillIds(manageableSkills, config?.enabled_skill_ids),
    [config?.enabled_skill_ids, manageableSkills]
  );

  const skillDisplays = useMemo(
    () =>
      manageableSkills.reduce<Record<string, AIChatSkillDisplayInfo>>((map, skill) => {
        map[skill.skill_id] = getAIChatSkillDisplayInfo(skill, locale);
        return map;
      }, {}),
    [locale, manageableSkills]
  );

  const isLoading = isLoadingSkills || isLoadingConfig;
  const isImporting = previewImportSkill.isPending || confirmImportSkill.isPending;
  const isMutating = updateConfig.isPending || isImporting || deleteSkill.isPending;
  const saveSkillConfig = useCallback(
    async (requestedSkillIds: string[]) => {
      const response = await updateSkillConfig({
        payload: {
          enabled_skill_ids: requestedSkillIds,
        },
        silent: true,
      });
      const savedSkillIds = normalizeSkillIds(response.data?.enabled_skill_ids ?? requestedSkillIds);
      queryClient.setQueryData(AICHAT_KEYS.skillConfig(), { enabled_skill_ids: savedSkillIds });
      return savedSkillIds;
    },
    [queryClient, updateSkillConfig]
  );
  const handleAutosaveError = useCallback(
    (error: unknown) => {
      toast.error(
        error instanceof Error ? error.message : t('organization.aichatSkills.messages.saveFailed')
      );
    },
    [t]
  );
  const { enabledSkillIds, setEnabledSkillIds, saveStatus } = useAIChatSkillConfigAutosave(
    {
      initialEnabledSkillIds,
      isLoading,
      save: saveSkillConfig,
      onError: handleAutosaveError,
    }
  );
  const enabledCount = enabledSkillIds.length;
  const systemSkills = useMemo(
    () => manageableSkills.filter(skill => getSkillSource(skill) === 'system'),
    [manageableSkills]
  );
  const customSkills = useMemo(
    () => manageableSkills.filter(skill => getSkillSource(skill) === 'custom'),
    [manageableSkills]
  );
  const hasActiveFilters =
    searchQuery.trim().length > 0 || runtimeFilter !== 'all' || statusFilter !== 'all';
  const filteredSystemSkills = useMemo(
    () =>
      filterSkills(
        systemSkills,
        skillDisplays,
        enabledSkillIds,
        searchQuery,
        runtimeFilter,
        statusFilter
      ),
    [enabledSkillIds, runtimeFilter, searchQuery, skillDisplays, statusFilter, systemSkills]
  );
  const filteredCustomSkills = useMemo(
    () =>
      filterSkills(
        customSkills,
        skillDisplays,
        enabledSkillIds,
        searchQuery,
        runtimeFilter,
        statusFilter
      ),
    [customSkills, enabledSkillIds, runtimeFilter, searchQuery, skillDisplays, statusFilter]
  );

  const handleToggle = (skillId: string, enabled: boolean) => {
    setEnabledSkillIds(current => {
      const next = new Set(current);
      if (enabled) {
        next.add(skillId);
      } else {
        next.delete(skillId);
      }
      return normalizeSkillIds(Array.from(next));
    });
  };

  const handleImportClick = () => {
    fileInputRef.current?.click();
  };

  const importButton =
    activeTab === 'custom' && customSkills.length > 0 ? (
      <Button size="sm" disabled={isMutating} onClick={handleImportClick}>
        {isImporting ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
        {isImporting
          ? t('organization.aichatSkills.actions.importing')
          : t('organization.aichatSkills.actions.import')}
      </Button>
    ) : null;

  const handleClearFilters = () => {
    setSearchQuery('');
    setRuntimeFilter('all');
    setStatusFilter('all');
  };

  const handleImportFile = async (file: File) => {
    if (!file.name.toLowerCase().endsWith('.zip')) {
      toast.error(t('organization.aichatSkills.messages.zipRequired'));
      return;
    }

    try {
      const response = await previewImportSkill.mutateAsync(file);
      importConfirmedRef.current = false;
      setImportPreview(response.data);
      setIsImportPreviewOpen(true);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    } catch {
      // The mutation hook owns user-facing error feedback.
    }
  };

  const handleFileInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    void handleImportFile(file);
    event.target.value = '';
  };

  const handleImportPreviewOpenChange = (open: boolean) => {
    setIsImportPreviewOpen(open);
    if (!open && !confirmImportSkill.isPending) {
      const importId = importPreview?.import_id;
      if (importId && !importConfirmedRef.current) {
        cancelImportPreview.mutate(importId);
      }
      importConfirmedRef.current = false;
      setImportPreview(null);
    }
  };

  const handleConfirmImport = async () => {
    if (!importPreview?.import_id) return;
    try {
      await confirmImportSkill.mutateAsync({
        import_id: importPreview.import_id,
        overwrite_confirmed: Boolean(importPreview.will_overwrite),
      });
      importConfirmedRef.current = true;
      setIsImportPreviewOpen(false);
      setImportPreview(null);
    } catch {
      // The mutation hook owns user-facing error feedback.
    }
  };

  const handleConfirmDelete = async () => {
    if (!skillToDelete) return;
    try {
      await deleteSkill.mutateAsync(skillToDelete.skill_id);
      setSkillToDelete(null);
    } catch {
      // The mutation hook owns user-facing error feedback.
    }
  };

  return (
    <div className="space-y-4">
      <input
        ref={fileInputRef}
        type="file"
        accept=".zip,application/zip,application/x-zip-compressed"
        className="hidden"
        onChange={handleFileInputChange}
      />

      {isLoading ? (
        <div className="space-y-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <Skeleton className="h-10 w-full rounded-md sm:w-80" />
            <div className="flex gap-2">
              <Skeleton className="h-8 w-24 rounded-md" />
              <Skeleton className="h-8 w-28 rounded-md" />
            </div>
          </div>
          {Array.from({ length: 2 }).map((_, sectionIndex) => (
            <div key={sectionIndex} className="space-y-3">
              <Skeleton className="h-14 rounded-md" />
              <div className={SKILL_CARD_GRID_CLASS}>
                {Array.from({ length: 3 }).map((_, index) => (
                  <Skeleton key={index} className="h-42 rounded-md" />
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : isError ? (
        <div className="rounded-md border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
          {t('organization.aichatSkills.loadFailed')}
        </div>
      ) : skills.length === 0 ? (
        <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('organization.aichatSkills.empty')}
        </div>
      ) : manageableSkills.length === 0 ? (
        <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('organization.aichatSkills.empty')}
        </div>
      ) : (
        <Tabs
          value={activeTab}
          onValueChange={value => setActiveTab(value as 'system' | 'custom')}
          className="space-y-3"
        >
          <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="space-y-3">
              <TabsList className="grid h-auto w-full grid-cols-2 justify-start rounded-md p-1 sm:w-[380px]">
                <TabsTrigger value="system" className="h-9 gap-2 rounded-md px-3">
                  <Wrench className="size-4" />
                  {t('organization.aichatSkills.tabs.system')}
                  <Badge variant="subtle" className="rounded-md">
                    {formatTabCount(
                      filteredSystemSkills.length,
                      systemSkills.length,
                      hasActiveFilters
                    )}
                  </Badge>
                </TabsTrigger>
                <TabsTrigger value="custom" className="h-9 gap-2 rounded-md px-3">
                  <Upload className="size-4" />
                  {t('organization.aichatSkills.tabs.custom')}
                  <Badge variant="subtle" className="rounded-md">
                    {formatTabCount(
                      filteredCustomSkills.length,
                      customSkills.length,
                      hasActiveFilters
                    )}
                  </Badge>
                </TabsTrigger>
              </TabsList>
              <SkillFilterToolbar
                searchQuery={searchQuery}
                runtimeFilter={runtimeFilter}
                statusFilter={statusFilter}
                hasActiveFilters={hasActiveFilters}
                onSearchQueryChange={setSearchQuery}
                onRuntimeFilterChange={setRuntimeFilter}
                onStatusFilterChange={setStatusFilter}
                onClearFilters={handleClearFilters}
              />
            </div>
            <div className="flex flex-wrap items-center gap-2 lg:justify-end">
              <Badge variant="secondary" className="h-8 rounded-md">
                <Wrench className="size-4" />
                {t('organization.aichatSkills.enabledCount', { count: enabledCount })}
              </Badge>
              <AutoSaveStatusIndicator status={saveStatus} />
              {importButton}
            </div>
          </div>

          <TabsContent value="system" className="mt-0">
            <div className="space-y-3">
              {filteredSystemSkills.length > 0 ? (
                <div className={SKILL_CARD_GRID_CLASS}>
                  {filteredSystemSkills.map(skill => (
                    <AIChatSkillCard
                      key={skill.skill_id}
                      skill={skill}
                      display={skillDisplays[skill.skill_id]}
                      enabled={enabledSkillIds.includes(skill.skill_id)}
                      disabled={isMutating}
                      onToggle={handleToggle}
                      onDelete={setSkillToDelete}
                    />
                  ))}
                </div>
              ) : systemSkills.length > 0 ? (
                <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <span>{t('organization.aichatSkills.filters.empty')}</span>
                    <Button variant="ghost" size="sm" onClick={handleClearFilters}>
                      {t('organization.aichatSkills.actions.clearFilters')}
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                  {t('organization.aichatSkills.sections.system.empty')}
                </div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="custom" className="mt-0">
            <div className="space-y-3">
              {filteredCustomSkills.length > 0 ? (
                <div className={SKILL_CARD_GRID_CLASS}>
                  {filteredCustomSkills.map(skill => (
                    <AIChatSkillCard
                      key={skill.skill_id}
                      skill={skill}
                      display={skillDisplays[skill.skill_id]}
                      enabled={enabledSkillIds.includes(skill.skill_id)}
                      disabled={isMutating}
                      onToggle={handleToggle}
                      onDelete={setSkillToDelete}
                    />
                  ))}
                </div>
              ) : customSkills.length > 0 ? (
                <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <span>{t('organization.aichatSkills.filters.empty')}</span>
                    <Button variant="ghost" size="sm" onClick={handleClearFilters}>
                      {t('organization.aichatSkills.actions.clearFilters')}
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="rounded-md border border-dashed bg-muted/20 p-8">
                  <div className="mx-auto flex max-w-md flex-col items-center text-center">
                    <div className="flex size-11 items-center justify-center rounded-md border bg-background text-muted-foreground">
                      <Upload className="size-5" />
                    </div>
                    <h3 className="mt-4 text-sm font-medium text-foreground">
                      {t('organization.aichatSkills.sections.custom.emptyTitle')}
                    </h3>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                      {t('organization.aichatSkills.sections.custom.emptyDescription')}
                    </p>
                    <Button
                      className="mt-5"
                      size="sm"
                      disabled={isMutating}
                      onClick={handleImportClick}
                    >
                      {isImporting ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Upload className="size-4" />
                      )}
                      {isImporting
                        ? t('organization.aichatSkills.actions.importing')
                        : t('organization.aichatSkills.actions.import')}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </TabsContent>
        </Tabs>
      )}

      <ConfirmDialog
        variant="danger"
        open={Boolean(skillToDelete)}
        onOpenChange={open => {
          if (!open) setSkillToDelete(null);
        }}
        title={t('organization.aichatSkills.deleteConfirm.title')}
        description={t('organization.aichatSkills.deleteConfirm.description', {
          skill: skillToDelete?.name || skillToDelete?.skill_id || '',
        })}
        confirmText={
          deleteSkill.isPending
            ? t('organization.aichatSkills.actions.deleting')
            : t('organization.aichatSkills.deleteConfirm.confirm')
        }
        cancelText={t('organization.aichatSkills.deleteConfirm.cancel')}
        onConfirm={handleConfirmDelete}
        loading={deleteSkill.isPending}
      />

      <SkillImportPreviewDialog
        preview={importPreview}
        open={isImportPreviewOpen}
        loading={confirmImportSkill.isPending}
        onOpenChange={handleImportPreviewOpenChange}
        onConfirm={handleConfirmImport}
      />
    </div>
  );
}
