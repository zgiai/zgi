'use client';

import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { AlertCircle, CheckCircle2, Loader2, Trash2, Upload } from 'lucide-react';
import { toast } from 'sonner';
import {
  usePageContextRegistration,
  type AIChatPageContextItem,
} from '@/components/aichat/page-context';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import {
  getAIChatSkillDisplayInfo,
  getSkillIntegrationRequirements,
  isSkillUserSelectable,
  type AIChatSkillDisplayInfo,
} from '@/components/chat/variants/aichat/skill-display';
import { normalizeAIChatSkillIds } from '@/components/chat/variants/aichat/skill-identity';
import {
  SKILL_CAPABILITY_CATEGORIES,
  SKILL_SCENARIOS,
} from '@/components/chat/variants/aichat/skill-taxonomy';
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
import { Skeleton } from '@/components/ui/skeleton';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Switch } from '@/components/ui/switch';
import { AgentResourceBoundDialog } from '@/components/common/agent-resource-bound-dialog';
import {
  AIChatSkillCatalogFilters,
  type SkillCapabilityFilter,
  type SkillDependencyFilter,
  type SkillScenarioFilter,
  type SkillSourceFilter,
  type SkillStatusFilter,
} from '@/components/dashboard/organization/aichat-skill-catalog-filters';
import {
  useDeleteAIChatSkill,
  useAIChatSkillConfig,
  useAIChatSkills,
  useCancelImportAIChatSkillPreview,
  useConfirmImportAIChatSkill,
  usePreviewImportAIChatSkill,
  useUpdateAIChatSkillConfig,
} from '@/hooks/aichat/use-aichat-skills';
import {
  integrationCatalogItems,
  integrationConnectionItems,
  useIntegrationCatalog,
  useIntegrationConnections,
} from '@/hooks';
import { AICHAT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT, type DashboardSuffix } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type {
  AIChatSkillMetadata,
  AIChatSkillAvailabilityState,
  AIChatImportSkillPreview,
  AIChatSkillConfigUpdateResult,
  AIChatSkillRuntimeType,
  AIChatSkillSource,
} from '@/services/types/aichat';
import type { IntegrationCatalogItem, IntegrationConnection } from '@/services/types/integration';
import type { AgentResourceBoundImpact } from '@/services/types/common';
import { getAgentResourceBoundImpact } from '@/utils/agent-resource-bound';
import { aichatService } from '@/services/aichat.service';
import {
  integrationCatalogID,
  resolveProviderHealthState,
} from '@/components/integrations/integration-utils';
import { useIntegrationMetadata } from '@/components/integrations/metadata-i18n';

const SKILL_CARD_GRID_CLASS = 'grid gap-2 sm:grid-cols-2 lg:grid-cols-3';
const SYSTEM_SKILL_NAME_CONFLICT_ERROR =
  'This skill name is reserved by a built-in system skill. Please rename your custom skill and try again.';

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';

const RUNTIME_LABEL_KEYS: Record<AIChatSkillRuntimeType, DashboardSuffix> = {
  tool: 'organization.aichatSkills.runtime.tool',
  prompt: 'organization.aichatSkills.runtime.prompt',
  hybrid: 'organization.aichatSkills.runtime.hybrid',
};

const SCRIPT_STATUS_LABEL_KEYS = {
  runnable: 'organization.aichatSkills.scriptStatus.runnable',
  unsupported: 'organization.aichatSkills.scriptStatus.unsupported',
} as const satisfies Record<string, DashboardSuffix>;

const AVAILABILITY_LABEL_KEYS = {
  ready: 'organization.aichatSkills.availability.ready',
  setup_required: 'organization.aichatSkills.availability.setupRequired',
  no_access: 'organization.aichatSkills.availability.noAccess',
  degraded: 'organization.aichatSkills.availability.degraded',
  unavailable: 'organization.aichatSkills.availability.unavailable',
} as const satisfies Record<AIChatSkillAvailabilityState, DashboardSuffix>;

const AVAILABILITY_VARIANTS = {
  ready: 'success',
  setup_required: 'warning',
  no_access: 'warning',
  degraded: 'warning',
  unavailable: 'destructive',
} as const;

const AUTO_SAVE_LABEL_KEYS = {
  idle: 'organization.aichatSkills.autoSave.ready',
  saving: 'organization.aichatSkills.autoSave.saving',
  saved: 'organization.aichatSkills.autoSave.saved',
  error: 'organization.aichatSkills.autoSave.error',
} as const satisfies Record<SaveStatus, DashboardSuffix>;

function normalizeSkillIds(ids: unknown): string[] {
  return normalizeAIChatSkillIds(ids).sort((a, b) => a.localeCompare(b));
}

function getInitialEnabledSkillIds(
  skills: AIChatSkillMetadata[],
  configIds: string[] | undefined
): string[] {
  const manageableIds = new Set(skills.map(skill => skill.skill_id));
  const ids = configIds
    ? normalizeAIChatSkillIds(configIds).filter(skillId => manageableIds.has(skillId))
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

function getSkillDependencyAvailability(
  skill: AIChatSkillMetadata
): 'ready' | 'unavailable' | 'unknown' {
  if (!isInvalidSkill(skill)) return 'ready';
  return /dependency|依赖/i.test(skill.validation_error ?? '') ? 'unavailable' : 'unknown';
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
    display?.categoryLabel,
    ...(display?.scenarios ?? []),
    ...(display?.tags ?? []),
    ...(display?.integrationRequirements ?? []),
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
}

function resolveSkillAvailability(
  skill: AIChatSkillMetadata,
  catalog: IntegrationCatalogItem[],
  connections: IntegrationConnection[]
): AIChatSkillAvailabilityState {
  if (skill.availability?.state) return skill.availability.state;
  const requirements = getSkillIntegrationRequirements(skill);
  if (requirements.length === 0) return 'ready';

  let result: AIChatSkillAvailabilityState = 'ready';
  for (const integrationId of requirements) {
    const provider = catalog.find(item => integrationCatalogID(item) === integrationId);
    if (!provider || !provider.enabled) return 'unavailable';
    const providerState = resolveProviderHealthState(provider, connections);
    if (providerState === 'unavailable' || providerState === 'unknown') return 'unavailable';
    if (providerState === 'setup_required') result = 'setup_required';
    if (providerState === 'degraded' && result === 'ready') result = 'degraded';
  }
  return result;
}

function filterSkills(
  skills: AIChatSkillMetadata[],
  displays: Record<string, AIChatSkillDisplayInfo>,
  enabledSkillIds: string[],
  searchQuery: string,
  scenarioFilter: SkillScenarioFilter,
  capabilityFilter: SkillCapabilityFilter,
  sourceFilter: SkillSourceFilter,
  statusFilter: SkillStatusFilter,
  dependencyFilter: SkillDependencyFilter
): AIChatSkillMetadata[] {
  const query = searchQuery.trim().toLowerCase();
  const enabledSet = new Set(enabledSkillIds);

  return skills.filter(skill => {
    const display = displays[skill.skill_id];
    if (scenarioFilter !== 'all' && !display?.scenarios.includes(scenarioFilter)) return false;
    if (capabilityFilter !== 'all' && display?.category !== capabilityFilter) return false;
    if (sourceFilter !== 'all' && getSkillSource(skill) !== sourceFilter) return false;
    if (
      dependencyFilter !== 'all' &&
      (display?.dependencyKind ?? 'standalone') !== dependencyFilter
    ) {
      return false;
    }

    const enabled = enabledSet.has(skill.skill_id);
    const invalid = isInvalidSkill(skill);
    if (statusFilter === 'enabled' && (!enabled || invalid)) return false;
    if (statusFilter === 'disabled' && (enabled || invalid)) return false;
    if (statusFilter === 'invalid' && !invalid) return false;

    if (!query) return true;
    return getFilterSearchText(skill, display).includes(query);
  });
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
  availability: AIChatSkillAvailabilityState;
  integrationNames: string[];
  enabled: boolean;
  disabled: boolean;
  onToggle: (skillId: string, enabled: boolean) => void;
  onSelect: (skill: AIChatSkillMetadata) => void;
}

/**
 * @component AIChatSkillCard
 * @category Feature
 * @status Stable
 * @description Card item for scanning, enabling, and managing one AIChat Skill.
 * @usage Render within the organization AIChat Skill settings grid.
 * @example
 * <AIChatSkillCard skill={skill} display={display} enabled={true} disabled={false} onToggle={onToggle} onSelect={onSelect} />
 */
function AIChatSkillCard({
  skill,
  display,
  availability,
  integrationNames,
  enabled,
  disabled,
  onToggle,
  onSelect,
}: AIChatSkillCardProps) {
  const t = useT('dashboard');
  const invalid = isInvalidSkill(skill);
  const externalDependency = display.dependencyKind === 'external_integration';
  const dependencyReady = availability === 'ready';

  return (
    <article
      className={cn(
        'group flex min-h-[72px] items-center gap-2.5 rounded-md border border-border bg-card p-2.5 transition-[border-color,background-color,opacity] hover:border-primary/25 hover:bg-muted/20',
        disabled || invalid ? 'opacity-75' : ''
      )}
    >
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center gap-2.5 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        onClick={() => onSelect(skill)}
      >
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground transition-colors',
            enabled && !invalid ? 'border-primary/25 bg-primary/10 text-primary' : ''
          )}
        >
          <AIChatSkillIcon icon={display.icon} className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-semibold text-foreground">{display.label}</h3>
            {invalid ? (
              <Badge variant="destructive" className="h-5 shrink-0 rounded-md px-1.5 font-normal">
                {t('organization.aichatSkills.status.invalid')}
              </Badge>
            ) : null}
            {externalDependency ? (
              <Badge
                variant={AVAILABILITY_VARIANTS[availability]}
                className="h-5 shrink-0 rounded-md px-1.5 font-normal"
              >
                {t(AVAILABILITY_LABEL_KEYS[availability])}
              </Badge>
            ) : null}
          </div>
          <p className="mt-0.5 line-clamp-1 text-xs leading-5 text-muted-foreground">
            {display.description}
          </p>
          {externalDependency && integrationNames.length > 0 ? (
            <p className="truncate text-xs leading-5 text-muted-foreground">
              {t('organization.aichatSkills.dependency.usesExternal')}: {integrationNames.join(', ')}
            </p>
          ) : null}
        </div>
      </button>
      <Switch
        checked={enabled}
        disabled={disabled || invalid || (externalDependency && !dependencyReady)}
        aria-label={t('organization.aichatSkills.toggleAria', { skill: display.label })}
        onCheckedChange={checked => onToggle(skill.skill_id, checked)}
      />
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
        'inline-flex h-7 items-center gap-1.5 text-xs',
        isError ? 'font-medium text-destructive' : 'text-muted-foreground'
      )}
    >
      <Icon className={cn('size-3.5', isSaving ? 'animate-spin' : '')} />
      {t(AUTO_SAVE_LABEL_KEYS[status])}
    </span>
  );
}

interface UseAIChatSkillConfigPersistenceOptions {
  initialEnabledSkillIds: string[];
  isLoading: boolean;
  save: (
    enabledSkillIds: string[],
    impact?: AgentResourceBoundImpact
  ) => Promise<AIChatSkillConfigUpdateResult>;
  onConfirmationRequired: (impact: AgentResourceBoundImpact, requestedSkillIds: string[]) => void;
  onError: (error: unknown, requestedSkillIds: string[]) => boolean;
}

function useAIChatSkillConfigPersistence({
  initialEnabledSkillIds,
  isLoading,
  save,
  onConfirmationRequired,
  onError,
}: UseAIChatSkillConfigPersistenceOptions) {
  const [enabledSkillIds, setEnabledSkillIds] = useState<string[]>([]);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  const confirmedSkillIdsRef = useRef<string[]>([]);
  const hasHydratedRef = useRef(false);

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

  const saveEnabledSkillIds = useCallback(
    async (requested: string[], impact?: AgentResourceBoundImpact) => {
      if (isLoading) return false;
      const requestedSkillIds = normalizeSkillIds(requested);
      const requestedKey = getSkillIdsKey(requestedSkillIds);
      if (!impact && requestedKey === getSkillIdsKey(confirmedSkillIdsRef.current)) return true;

      setSaveStatus('saving');
      try {
        const result = await save(requestedSkillIds, impact);
        if (!result.applied) {
          onConfirmationRequired(result.impact, requestedSkillIds);
          setSaveStatus('idle');
          return false;
        }
        const savedSkillIds = normalizeSkillIds(result.enabled_skill_ids);
        confirmedSkillIdsRef.current = savedSkillIds;
        setEnabledSkillIds(savedSkillIds);
        setSaveStatus(getSkillIdsKey(savedSkillIds) === requestedKey ? 'saved' : 'idle');
        return true;
      } catch (error) {
        setSaveStatus(onError(error, requestedSkillIds) ? 'idle' : 'error');
        return false;
      }
    },
    [isLoading, onConfirmationRequired, onError, save]
  );

  return {
    enabledSkillIds,
    saveStatus,
    saveEnabledSkillIds,
  };
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
  const tCommon = useT('common');
  const { locale } = useLocale();
  const integrationMetadata = useIntegrationMetadata();
  const queryClient = useQueryClient();
  const { data: skills = [], isLoading: isLoadingSkills, isError } = useAIChatSkills();
  const { data: config, isLoading: isLoadingConfig } = useAIChatSkillConfig();
  const integrationCatalogQuery = useIntegrationCatalog(true, 'organization');
  const integrationConnectionsQuery = useIntegrationConnections({ page: 1, limit: 100 });
  const updateConfig = useUpdateAIChatSkillConfig();
  const updateSkillConfig = updateConfig.mutateAsync;
  const previewImportSkill = usePreviewImportAIChatSkill();
  const confirmImportSkill = useConfirmImportAIChatSkill();
  const cancelImportPreview = useCancelImportAIChatSkillPreview();
  const deleteSkill = useDeleteAIChatSkill();
  const [searchQuery, setSearchQuery] = useState('');
  const [scenarioFilter, setScenarioFilter] = useState<SkillScenarioFilter>('all');
  const [capabilityFilter, setCapabilityFilter] = useState<SkillCapabilityFilter>('all');
  const [sourceFilter, setSourceFilter] = useState<SkillSourceFilter>('all');
  const [statusFilter, setStatusFilter] = useState<SkillStatusFilter>('all');
  const [dependencyFilter, setDependencyFilter] = useState<SkillDependencyFilter>('all');
  const [selectedSkill, setSelectedSkill] = useState<AIChatSkillMetadata | null>(null);
  const [skillToDelete, setSkillToDelete] = useState<AIChatSkillMetadata | null>(null);
  const [bindingImpact, setBindingImpact] = useState<AgentResourceBoundImpact | null>(null);
  const [isCheckingDeleteImpact, setIsCheckingDeleteImpact] = useState(false);
  const [skillConfigBindingConflict, setSkillConfigBindingConflict] = useState<{
    impact: AgentResourceBoundImpact;
    requestedSkillIds: string[];
  } | null>(null);
  const [importPreview, setImportPreview] = useState<AIChatImportSkillPreview | null>(null);
  const [isImportPreviewOpen, setIsImportPreviewOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const importConfirmedRef = useRef(false);
  const manageableSkills = useMemo(
    () => skills.filter(skill => isSkillUserSelectable(skill)),
    [skills]
  );
  const integrationCatalog = integrationCatalogItems(integrationCatalogQuery.data?.data);
  const integrationConnections = integrationConnectionItems(integrationConnectionsQuery.data?.data);

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
  const skillAvailability = useMemo(
    () =>
      manageableSkills.reduce<Record<string, AIChatSkillAvailabilityState>>((map, skill) => {
        map[skill.skill_id] = resolveSkillAvailability(
          skill,
          integrationCatalog,
          integrationConnections
        );
        return map;
      }, {}),
    [integrationCatalog, integrationConnections, manageableSkills]
  );
  const integrationNames = new Map(
    integrationCatalog.map(item => [
      integrationCatalogID(item),
      integrationMetadata.providerName(item),
    ])
  );

  const isLoading = isLoadingSkills || isLoadingConfig;
  const isImporting = previewImportSkill.isPending || confirmImportSkill.isPending;
  const saveSkillConfig = useCallback(
    async (requestedSkillIds: string[], impact?: AgentResourceBoundImpact) => {
      const response = await updateSkillConfig({
        payload: {
          enabled_skill_ids: requestedSkillIds,
          agent_binding_action: impact ? 'retain_suspended' : undefined,
          impact_token: impact?.impact_token,
        },
        silent: true,
      });
      if (response.data.applied) {
        const savedSkillIds = normalizeSkillIds(response.data.enabled_skill_ids);
        queryClient.setQueryData(AICHAT_KEYS.skillConfig(), { enabled_skill_ids: savedSkillIds });
        return { ...response.data, enabled_skill_ids: savedSkillIds };
      }
      return response.data;
    },
    [queryClient, updateSkillConfig]
  );
  const handleSkillConfigError = useCallback(
    (error: unknown, requestedSkillIds: string[]) => {
      const impact = getAgentResourceBoundImpact(error);
      if (impact) {
        setSkillConfigBindingConflict({ impact, requestedSkillIds });
        return true;
      }
      toast.error(t('organization.aichatSkills.messages.saveFailed'));
      return false;
    },
    [t]
  );
  const handleSkillConfigConfirmationRequired = useCallback(
    (impact: AgentResourceBoundImpact, requestedSkillIds: string[]) => {
      setSkillConfigBindingConflict({ impact, requestedSkillIds });
    },
    []
  );
  const { enabledSkillIds, saveStatus, saveEnabledSkillIds } = useAIChatSkillConfigPersistence({
    initialEnabledSkillIds,
    isLoading,
    save: saveSkillConfig,
    onConfirmationRequired: handleSkillConfigConfirmationRequired,
    onError: handleSkillConfigError,
  });
  const isMutating =
    saveStatus === 'saving' ||
    updateConfig.isPending ||
    isImporting ||
    deleteSkill.isPending ||
    isCheckingDeleteImpact;
  const enabledCount = enabledSkillIds.length;
  const availableScenarios = useMemo(
    () =>
      SKILL_SCENARIOS.filter(scenario =>
        manageableSkills.some(skill => skillDisplays[skill.skill_id]?.scenarios.includes(scenario))
      ),
    [manageableSkills, skillDisplays]
  );
  const scenarioScopedSkills = useMemo(
    () =>
      scenarioFilter === 'all'
        ? manageableSkills
        : manageableSkills.filter(skill =>
            skillDisplays[skill.skill_id]?.scenarios.includes(scenarioFilter)
          ),
    [manageableSkills, scenarioFilter, skillDisplays]
  );
  const availableCapabilities = useMemo(
    () =>
      SKILL_CAPABILITY_CATEGORIES.filter(capability =>
        scenarioScopedSkills.some(skill => skillDisplays[skill.skill_id]?.category === capability)
      ),
    [scenarioScopedSkills, skillDisplays]
  );
  const hasActiveFilters =
    searchQuery.trim().length > 0 ||
    scenarioFilter !== 'all' ||
    capabilityFilter !== 'all' ||
    sourceFilter !== 'all' ||
    statusFilter !== 'all' ||
    dependencyFilter !== 'all';
  const filteredSkills = useMemo(
    () =>
      filterSkills(
        manageableSkills,
        skillDisplays,
        enabledSkillIds,
        searchQuery,
        scenarioFilter,
        capabilityFilter,
        sourceFilter,
        statusFilter,
        dependencyFilter
      ),
    [
      capabilityFilter,
      dependencyFilter,
      enabledSkillIds,
      manageableSkills,
      scenarioFilter,
      searchQuery,
      skillDisplays,
      sourceFilter,
      statusFilter,
    ]
  );
  const skillPageContextItems = useMemo<AIChatPageContextItem[]>(() => {
    const enabledSet = new Set(enabledSkillIds);
    const invalidCount = manageableSkills.filter(isInvalidSkill).length;
    const dependencyUnavailableCount = manageableSkills.filter(
      skill => getSkillDependencyAvailability(skill) === 'unavailable'
    ).length;
    const selectedAvailability = selectedSkill
      ? getSkillDependencyAvailability(selectedSkill)
      : null;
    const selectedStatus = selectedSkill
      ? isInvalidSkill(selectedSkill)
        ? 'invalid'
        : enabledSet.has(selectedSkill.skill_id)
          ? 'enabled'
          : 'disabled'
      : null;

    return [
      {
        id: 'console.skills',
        type: 'page',
        title: t('organization.aichatSkills.title'),
        description: t('organization.aichatSkills.description'),
        href: '/console/skills',
        source: 'console-skills',
        status: isError ? 'error' : isLoading ? 'loading' : 'available',
        metadata: {
          page: 'console.skills',
          route: '/console/skills',
          resource_kind: 'page',
          context_ready: !isLoading && !isError,
          manageable_skill_count: manageableSkills.length,
          visible_skill_count: filteredSkills.length,
          enabled_skill_count: enabledSkillIds.length,
          disabled_skill_count: manageableSkills.filter(
            skill => !isInvalidSkill(skill) && !enabledSet.has(skill.skill_id)
          ).length,
          invalid_skill_count: invalidCount,
          dependency_unavailable_count: dependencyUnavailableCount,
          search: searchQuery.trim(),
          scenario_filter: scenarioFilter,
          capability_filter: capabilityFilter,
          source_filter: sourceFilter,
          status_filter: statusFilter,
          dependency_filter: 'all',
          auto_save_status: saveStatus,
          selected_skill_name: selectedSkill?.name ?? null,
          selected_skill_status: selectedStatus,
          selected_skill_dependency_availability: selectedAvailability,
        },
      },
    ];
  }, [
    capabilityFilter,
    enabledSkillIds,
    filteredSkills.length,
    isError,
    isLoading,
    manageableSkills,
    saveStatus,
    scenarioFilter,
    searchQuery,
    selectedSkill,
    sourceFilter,
    statusFilter,
    t,
  ]);

  usePageContextRegistration(skillPageContextItems, {
    scopeId: 'console-skills',
    replace: true,
    priority: 80,
    visibility: 'current',
  });

  useEffect(() => {
    if (capabilityFilter !== 'all' && !availableCapabilities.includes(capabilityFilter)) {
      setCapabilityFilter('all');
    }
  }, [availableCapabilities, capabilityFilter]);

  const handleToggle = (skillId: string, enabled: boolean) => {
    const next = new Set(enabledSkillIds);
    if (enabled) {
      next.add(skillId);
    } else {
      next.delete(skillId);
    }
    void saveEnabledSkillIds(Array.from(next));
  };

  const handleConfirmRetainSuspended = async () => {
    if (!skillConfigBindingConflict) return;
    const { impact, requestedSkillIds } = skillConfigBindingConflict;
    if (!(await saveEnabledSkillIds(requestedSkillIds, impact))) return;
    setSkillConfigBindingConflict(null);
    toast.success(t('organization.aichatSkills.messages.saved'));
  };

  const handleImportClick = () => {
    fileInputRef.current?.click();
  };

  const importButtons = (
    <Button size="sm" disabled={isMutating} onClick={handleImportClick}>
      {isImporting ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
      {isImporting
        ? t('organization.aichatSkills.actions.importing')
        : t('organization.aichatSkills.actions.import')}
    </Button>
  );

  const handleScenarioChange = (value: SkillScenarioFilter) => {
    setScenarioFilter(value);
    setCapabilityFilter('all');
  };

  const handleClearFilters = () => {
    setSearchQuery('');
    setScenarioFilter('all');
    setCapabilityFilter('all');
    setSourceFilter('all');
    setStatusFilter('all');
    setDependencyFilter('all');
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

  const handleConfirmDelete = async (impact?: AgentResourceBoundImpact) => {
    if (!skillToDelete) return;
    try {
      await deleteSkill.mutateAsync({
        id: skillToDelete.skill_id,
        confirmation: impact
          ? { agent_binding_action: 'unbind', impact_token: impact.impact_token }
          : undefined,
      });
      if (selectedSkill?.skill_id === skillToDelete.skill_id) setSelectedSkill(null);
      setSkillToDelete(null);
      setBindingImpact(null);
    } catch (error) {
      const nextImpact = getAgentResourceBoundImpact(error);
      if (nextImpact) setBindingImpact(nextImpact);
    }
  };

  const handleRequestDelete = async (skill: AIChatSkillMetadata) => {
    if (isCheckingDeleteImpact) return;
    setIsCheckingDeleteImpact(true);
    try {
      const response = await aichatService.previewSkillDeleteImpact(skill.skill_id);
      setSkillToDelete(skill);
      if (response.data) setBindingImpact(response.data);
    } catch {
      toast.error(tCommon('agentResourceBound.previewFailed'));
    } finally {
      setIsCheckingDeleteImpact(false);
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
        <div className="space-y-2.5">
          <AIChatSkillCatalogFilters
            locale={locale}
            availableScenarios={availableScenarios}
            availableCapabilities={availableCapabilities}
            scenario={scenarioFilter}
            capability={capabilityFilter}
            source={sourceFilter}
            status={statusFilter}
            dependency={dependencyFilter}
            searchQuery={searchQuery}
            hasActiveFilters={hasActiveFilters}
            onScenarioChange={handleScenarioChange}
            onCapabilityChange={setCapabilityFilter}
            onSourceChange={setSourceFilter}
            onStatusChange={setStatusFilter}
            onDependencyChange={setDependencyFilter}
            onSearchQueryChange={setSearchQuery}
            onClearFilters={handleClearFilters}
          />

          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>
                {t('organization.aichatSkills.filters.visibleCount', {
                  count: filteredSkills.length,
                })}
              </span>
              <span aria-hidden="true">·</span>
              <span>{t('organization.aichatSkills.enabledCount', { count: enabledCount })}</span>
              <span aria-hidden="true">·</span>
              <AutoSaveStatusIndicator status={saveStatus} />
            </div>
            <div className="flex flex-wrap items-center gap-2 sm:justify-end">{importButtons}</div>
          </div>

          {filteredSkills.length > 0 ? (
            <div className={SKILL_CARD_GRID_CLASS}>
              {filteredSkills.map(skill => (
                <AIChatSkillCard
                  key={skill.skill_id}
                  skill={skill}
                  display={skillDisplays[skill.skill_id]}
                  availability={skillAvailability[skill.skill_id] ?? 'unavailable'}
                  integrationNames={(
                    skillDisplays[skill.skill_id]?.integrationRequirements ?? []
                  ).map(
                    integrationId =>
                      integrationNames.get(integrationId) ??
                      integrationMetadata.providerName(
                        integrationId,
                        t('organization.aichatSkills.dependency.unknownExternalApp')
                      )
                  )}
                  enabled={enabledSkillIds.includes(skill.skill_id)}
                  disabled={isMutating}
                  onToggle={handleToggle}
                  onSelect={setSelectedSkill}
                />
              ))}
            </div>
          ) : (
            <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <span>{t('organization.aichatSkills.filters.empty')}</span>
                <Button variant="ghost" size="sm" onClick={handleClearFilters}>
                  {t('organization.aichatSkills.actions.clearFilters')}
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      <Sheet
        open={Boolean(selectedSkill)}
        onOpenChange={open => {
          if (!open) setSelectedSkill(null);
        }}
      >
        <SheetContent className="w-full overflow-y-auto p-0 sm:max-w-md">
          {selectedSkill && skillDisplays[selectedSkill.skill_id] ? (
            <>
              <SheetHeader className="border-b px-6 py-5 pr-12 text-left">
                <div className="flex items-start gap-3">
                  <div className="flex size-11 shrink-0 items-center justify-center rounded-lg border bg-primary/10 text-primary">
                    <AIChatSkillIcon
                      icon={skillDisplays[selectedSkill.skill_id].icon}
                      className="size-5"
                    />
                  </div>
                  <div className="min-w-0">
                    <SheetTitle className="truncate text-base">
                      {skillDisplays[selectedSkill.skill_id].label}
                    </SheetTitle>
                    <SheetDescription className="mt-1">{selectedSkill.skill_id}</SheetDescription>
                  </div>
                </div>
              </SheetHeader>

              <div className="space-y-6 px-6 py-5">
                <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
                  <div>
                    <p className="text-sm font-medium">
                      {t('organization.aichatSkills.details.availability')}
                    </p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {t('organization.aichatSkills.details.availabilityDescription')}
                    </p>
                  </div>
                  <Switch
                    checked={enabledSkillIds.includes(selectedSkill.skill_id)}
                    disabled={
                      isMutating ||
                      isInvalidSkill(selectedSkill) ||
                      (skillDisplays[selectedSkill.skill_id].dependencyKind ===
                        'external_integration' &&
                        (skillAvailability[selectedSkill.skill_id] ?? 'unavailable') !== 'ready')
                    }
                    aria-label={t('organization.aichatSkills.toggleAria', {
                      skill: skillDisplays[selectedSkill.skill_id].label,
                    })}
                    onCheckedChange={checked => handleToggle(selectedSkill.skill_id, checked)}
                  />
                </div>

                <section>
                  <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('organization.aichatSkills.details.description')}
                  </h3>
                  <p className="mt-2 text-sm leading-6 text-foreground">
                    {skillDisplays[selectedSkill.skill_id].description}
                  </p>
                </section>

                {skillDisplays[selectedSkill.skill_id].whenToUse ? (
                  <section>
                    <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      {t('organization.aichatSkills.details.whenToUse')}
                    </h3>
                    <p className="mt-2 text-sm leading-6 text-foreground">
                      {skillDisplays[selectedSkill.skill_id].whenToUse}
                    </p>
                  </section>
                ) : null}

                <section>
                  <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('organization.aichatSkills.details.information')}
                  </h3>
                  <dl className="mt-2 divide-y rounded-lg border">
                    <div className="flex items-center justify-between gap-4 px-3 py-2.5 text-sm">
                      <dt className="text-muted-foreground">
                        {t('organization.aichatSkills.filters.capabilityLabel')}
                      </dt>
                      <dd className="text-right font-medium">
                        {skillDisplays[selectedSkill.skill_id].categoryLabel}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-4 px-3 py-2.5 text-sm">
                      <dt className="text-muted-foreground">
                        {t('organization.aichatSkills.details.source')}
                      </dt>
                      <dd className="text-right font-medium">
                        {t(
                          getSkillSource(selectedSkill) === 'custom'
                            ? 'organization.aichatSkills.source.custom'
                            : 'organization.aichatSkills.source.system'
                        )}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-4 px-3 py-2.5 text-sm">
                      <dt className="text-muted-foreground">
                        {t('organization.aichatSkills.details.runtime')}
                      </dt>
                      <dd className="text-right font-medium">
                        {t(RUNTIME_LABEL_KEYS[selectedSkill.runtime_type])}
                      </dd>
                    </div>
                  </dl>
                </section>

                {skillDisplays[selectedSkill.skill_id].tags.length > 0 ? (
                  <section>
                    <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      {t('organization.aichatSkills.details.tags')}
                    </h3>
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {skillDisplays[selectedSkill.skill_id].tags.map(tag => (
                        <Badge key={tag} variant="subtle" className="rounded-md font-normal">
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  </section>
                ) : null}

                {isInvalidSkill(selectedSkill) && selectedSkill.validation_error ? (
                  <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm leading-5 text-destructive">
                    {selectedSkill.validation_error}
                  </div>
                ) : null}

                {getSkillSource(selectedSkill) === 'custom' ? (
                  <div className="border-t pt-4">
                    <Button
                      variant="ghost"
                      className="text-destructive hover:text-destructive"
                      disabled={isMutating}
                      onClick={() => void handleRequestDelete(selectedSkill)}
                    >
                      <Trash2 className="size-4" />
                      {t('organization.aichatSkills.actions.delete')}
                    </Button>
                  </div>
                ) : null}
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        variant="danger"
        open={Boolean(skillToDelete) && !bindingImpact}
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
        onConfirm={() => void handleConfirmDelete()}
        loading={deleteSkill.isPending}
      />

      <AgentResourceBoundDialog
        open={Boolean(skillConfigBindingConflict)}
        impact={skillConfigBindingConflict?.impact}
        loading={updateConfig.isPending}
        description={tCommon('agentResourceBound.retainSuspendedDescription', {
          count: skillConfigBindingConflict?.impact.agents.length ?? 0,
        })}
        warningTitle={tCommon('agentResourceBound.retainSuspendedWarningTitle')}
        warningDescription={tCommon('agentResourceBound.retainSuspendedWarningDescription')}
        actionLabel={tCommon('agentResourceBound.retainSuspendedConfirm')}
        onOpenChange={open => {
          if (!open) setSkillConfigBindingConflict(null);
        }}
        onConfirm={() => void handleConfirmRetainSuspended()}
      />

      <AgentResourceBoundDialog
        open={Boolean(bindingImpact)}
        impact={bindingImpact}
        loading={deleteSkill.isPending}
        onOpenChange={open => {
          if (!open) {
            setBindingImpact(null);
            setSkillToDelete(null);
          }
        }}
        onConfirm={() => {
          if (bindingImpact) void handleConfirmDelete(bindingImpact);
        }}
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
