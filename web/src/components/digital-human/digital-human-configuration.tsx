'use client';

import type { FileUploadSelection } from '@/services/upload.service';
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  CircleHelp,
  Loader2,
  MessageSquareText,
  Pencil,
  Plus,
  Search,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { toast } from 'sonner';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { DigitalHumanResourceBindings } from '@/components/digital-human/digital-human-resource-bindings';
import {
  DigitalHumanMemorySettings,
  memorySettingsOf,
  withMemorySettings,
} from '@/components/digital-human/digital-human-memory-instructions';
import {
  DigitalHumanSetupChecklist,
  useDigitalHumanSetupStatus,
} from '@/components/digital-human/digital-human-setup-checklist';
import { DigitalHumanWorkJournal } from '@/components/digital-human/digital-human-work-journal';
import { appendWorkJournalPrompt } from '@/components/digital-human/digital-human-work-journal-model';
import { DigitalHumanDraftPreview } from '@/components/digital-human/digital-human-draft-preview';
import { useDigitalHumanDraftPersistence } from '@/components/digital-human/use-digital-human-draft-persistence';
import {
  useDigitalHumanDraftStore,
  useRegisterDigitalHumanDraftEditor,
} from '@/components/digital-human/digital-human-draft-store';
import { Badge } from '@/components/ui/badge';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import { Switch } from '@/components/ui/switch';
import { PolicyFileUploadSettings } from '@/components/chat/file-upload-settings';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  useDigitalHuman,
  useDigitalHumanMutations,
  useDigitalHumanDraftBindings,
  useDigitalHumanTemplates,
} from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useAIChatSkills } from '@/hooks/aichat/use-aichat-skills';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { Checkbox } from '@/components/ui/checkbox';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  CreateDigitalHumanPersonaTemplateRequest,
  DigitalHumanTemplate,
  UpdateDigitalHumanDraftRequest,
} from '@/services/types/digital-human';

export type DigitalHumanConfigurationPanel = 'response';

const CONFIGURATION_TWO_COLUMN_MIN_WIDTH = 760;
const CONFIGURATION_INLINE_PREVIEW_MIN_WIDTH = 1280;

function useMeasuredWidth() {
  const ref = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const measure = () => setWidth(Math.floor(node.getBoundingClientRect().width));
    measure();
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure);
      return () => window.removeEventListener('resize', measure);
    }
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  return { ref, width };
}

export function DigitalHumanConfigurationEditor({
  id,
  initialPanel,
}: {
  id: string;
  initialPanel?: DigitalHumanConfigurationPanel | null;
}) {
  const detailQuery = useDigitalHuman(id);
  const detail = detailQuery.data?.data;
  const resetVersion = useDigitalHumanDraftStore(state => state.resetVersions[id] ?? 0);
  if (detailQuery.isLoading || !detail) {
    return <DigitalHumanConfigurationLoading />;
  }
  return (
    <ConfigurationEditor
      id={id}
      initialPanel={initialPanel}
      key={`${id}:${initialPanel ?? 'root'}:${resetVersion}`}
    />
  );
}

function DigitalHumanConfigurationLoading() {
  return (
    <div className="grid min-h-full lg:grid-cols-2 lg:divide-x">
      <div className="space-y-3 p-5">
        <div className="h-5 w-28 animate-pulse rounded bg-muted" />
        <div className="h-4 w-64 animate-pulse rounded bg-muted/70" />
        <div className="mt-5 h-56 animate-pulse rounded-xl bg-muted/50" />
      </div>
      <div className="p-5">
        <div className="h-[420px] animate-pulse rounded-xl bg-muted/40" />
      </div>
    </div>
  );
}

type EditableDigitalHumanDraft = Omit<UpdateDigitalHumanDraftRequest, 'expected_lock_version'>;

interface IdentityEditorSnapshot {
  draft: EditableDigitalHumanDraft;
}

function ConfigurationEditor({
  id,
  initialPanel,
}: {
  id: string;
  initialPanel?: DigitalHumanConfigurationPanel | null;
}) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const detail = useDigitalHuman(id).data?.data;
  const mutations = useDigitalHumanMutations(id);
  const versionPreview = useDigitalHumanDraftStore(state => state.versionPreviews[id]);
  const availableModels = useAvailableModels({ use_case: 'agent-runtime' });
  const [identityName, setIdentityName] = useState(
    detail?.draft.identity_name || detail?.definition.name || ''
  );
  const [persona, setPersona] = useState(detail?.draft.persona ?? '');
  const [goals, setGoals] = useState<string[]>(() => {
    const initialGoals = (detail?.draft.goals ?? []).map(item => item.trim()).filter(Boolean);
    return initialGoals.length > 0 ? initialGoals : [''];
  });
  const [rules, setRules] = useState<string[]>(() => {
    const initialRules = (detail?.draft.rules ?? []).map(item => item.trim()).filter(Boolean);
    return initialRules.length > 0 ? initialRules : [''];
  });
  const [model, setModel] = useState(detail?.draft.default_model ?? '');
  const [memorySettings, setMemorySettings] = useState(() =>
    memorySettingsOf(detail?.draft.memory_policy)
  );
  const draftBindings = useDigitalHumanDraftBindings(id).data?.data?.items;
  const setupStatus = useDigitalHumanSetupStatus(id);
  const draftSkillIds = useMemo(
    () =>
      (draftBindings ?? [])
        .filter(binding => binding.binding_type === 'skill')
        .map(binding => binding.resource_id),
    [draftBindings]
  );
  const [executionEnabled, setExecutionEnabled] = useState(
    detail?.draft.execution_runtime?.enabled ?? false
  );
  const executionRuntimeCapability = detail?.execution_runtime_capability ?? null;
  const canEnableExecution = executionRuntimeCapability?.can_enable ?? true;
  const [fileUploadPolicy, setFileUploadPolicy] = useState<FileUploadSelection | null>(detail?.draft.file_upload_policy ?? null);
  const { ref: bodyRef, width: bodyWidth } = useMeasuredWidth();
  const twoColumns = bodyWidth >= CONFIGURATION_TWO_COLUMN_MIN_WIDTH;
  // The draft preview sits beside the editor on wide screens, in a sheet otherwise.
  // While a published version is shown it stays in place, paused, so the page
  // keeps its shape and the preview conversation is still there afterwards.
  const previewAvailable = permissions.hasPermission('digital_human.update');
  const inlinePreview = previewAvailable && bodyWidth >= CONFIGURATION_INLINE_PREVIEW_MIN_WIDTH;
  const [previewOpen, setPreviewOpen] = useState(false);
  const renderPreview = (surface: 'inline' | 'sheet'): ReactNode => (
    <div className={cn('flex min-h-0 min-w-0 overflow-hidden', surface === 'inline' && 'h-full')}>
      <DigitalHumanDraftPreview
        id={id}
        name={identityName.trim()}
        model={model}
        fileUploadPolicy={fileUploadPolicy}
        surfaceMode={surface}
        pausedVersion={versionPreview ? (versionPreview.version.version_number ?? '') : undefined}
        onClose={() => setPreviewOpen(false)}
        beforeSend={() => manualSave()}
      />
    </div>
  );
  // Old links to the reply settings land on the section that now holds them.
  useEffect(() => {
    if (initialPanel !== 'response') return;
    const timer = window.setTimeout(
      () => document.getElementById('configuration-conversation')?.scrollIntoView({ block: 'center' }),
      300
    );
    return () => window.clearTimeout(timer);
  }, [initialPanel]);
  const models = availableModels.models;
  const canUpdate = permissions.hasPermission('digital_human.update');
  const editingEnabled = canUpdate && !versionPreview;
  const normalizedGoals = goals.map(item => item.trim()).filter(Boolean);
  const normalizedRules = rules.map(item => item.trim()).filter(Boolean);
  const draftLockVersionRef = useRef(detail?.draft.lock_version ?? 0);
  const initialSnapshotRef = useRef<IdentityEditorSnapshot>({
    draft: {
      identity_name: detail?.draft.identity_name || detail?.definition.name || '',
      persona: detail?.draft.persona ?? '',
      goals: detail?.draft.goals ?? [],
      rules: detail?.draft.rules ?? [],
      default_model: detail?.draft.default_model ?? '',
      memory_policy: withMemorySettings(
        detail?.draft.memory_policy,
        memorySettingsOf(detail?.draft.memory_policy)
      ),
      execution_runtime: { enabled: executionEnabled },
      file_upload_policy: fileUploadPolicy,
      trigger_policy: {
        ...(detail?.draft.trigger_policy ?? {}),
        manual: detail?.draft.trigger_policy?.manual ?? true,
      },
    },
  });
  const savedDraftSignatureRef = useRef(serializeEditorPart(initialSnapshotRef.current.draft));

  useEffect(() => {
    if (!detail) return;
    draftLockVersionRef.current = detail.draft.lock_version;
  }, [detail]);

  const currentSnapshot: IdentityEditorSnapshot = {
    draft: {
      identity_name: identityName.trim(),
      persona: persona.trim(),
      goals: normalizedGoals,
      rules: normalizedRules,
      default_model: model,
      memory_policy: withMemorySettings(detail?.draft.memory_policy, memorySettings),
      execution_runtime: { enabled: executionEnabled },
      file_upload_policy: fileUploadPolicy,
      trigger_policy: {
        ...(detail?.draft.trigger_policy ?? {}),
        manual: detail?.draft.trigger_policy?.manual ?? true,
      },
    },
  };
  const persistence = useDigitalHumanDraftPersistence({
    currentPayload: currentSnapshot,
    initialPayload: initialSnapshotRef.current,
    enabled: editingEnabled && Boolean(detail),
    canSave: () => Boolean(identityName.trim() && persona.trim() && model.trim()),
    savePayload: async snapshot => {
      const draftSignature = serializeEditorPart(snapshot.draft);
      const draftChanged = draftSignature !== savedDraftSignatureRef.current;
      if (draftChanged) {
        const draftResult = await mutations.updateDraft.mutateAsync({
          digitalHumanId: id,
          request: {
            expected_lock_version: draftLockVersionRef.current,
            ...snapshot.draft,
          },
        });
        draftLockVersionRef.current = draftResult.data.lock_version;
        savedDraftSignatureRef.current = draftSignature;
      }
      return { savedPayload: snapshot, updatedAt: Date.now() };
    },
  });

  const manualSave = async () => {
    if (versionPreview) return false;
    if (!identityName.trim() || !persona.trim() || !model.trim()) {
      toast.error(t('errors.required'));
      return false;
    }
    return persistence.saveNow({ force: true });
  };
  useRegisterDigitalHumanDraftEditor(id, {
    canUpdate: editingEnabled,
    saveState: persistence.saveState,
    lastSavedAt: persistence.lastSavedAt,
    isDirty: persistence.isDirty,
    isSaving: persistence.isSaving,
    saveNow: manualSave,
  });

  if (!detail) return null;
  const previewVersion = versionPreview?.version;
  const displayedExecutionEnabled = previewVersion
    ? (previewVersion.execution_runtime?.enabled ?? false)
    : executionEnabled;
  const executionUnavailable =
    executionRuntimeCapability != null &&
    !executionRuntimeCapability.available &&
    !displayedExecutionEnabled;
  const displayedIdentityName = previewVersion?.identity_name ?? identityName;
  const displayedPersona = previewVersion?.persona ?? persona;
  const displayedGoals = previewVersion
    ? previewVersion.goals.length > 0
      ? previewVersion.goals
      : ['']
    : goals;
  const displayedRules = previewVersion
    ? previewVersion.rules.length > 0
      ? previewVersion.rules
      : ['']
    : rules;
  const displayedModel = previewVersion?.default_model ?? model;
  const selectedModel = models.find(item => item.model === displayedModel);
  const modelValue: ModelSelectorValue = {
    provider: selectedModel?.provider ?? '',
    model: selectedModel?.model ?? '',
  };
  const modelUnavailable = Boolean(displayedModel && !selectedModel && !availableModels.isLoading);
  const isInitialSetup = !detail.published;
  const initialSetupChecks = [
    { label: t('fields.identityName'), ready: Boolean(identityName.trim()) },
    { label: t('fields.persona'), ready: Boolean(persona.trim()) },
    // What the template requires counts too: publishing waits for it.
    ...(setupStatus?.items ?? []).map(item => ({
      label: item.kind === 'skill' ? t('setup.shortSkill') : t('setup.shortConnector'),
      ready: item.met,
    })),
  ];
  const initialSetupReadyCount = initialSetupChecks.filter(check => check.ready).length;
  return (
    <div className="flex min-h-full min-w-0 flex-col bg-background lg:h-full lg:min-h-0">
      {versionPreview ? (
        <section className="shrink-0 border-b border-primary/20 bg-primary/[0.04] px-4 py-3 md:px-5">
          <p className="text-sm text-primary">
            {t('versionHistory.previewBanner', {
              version: versionPreview.version.version_number ?? '',
            })}
          </p>
        </section>
      ) : isInitialSetup ? (
        <section className="shrink-0 border-b bg-primary/[0.035] px-4 py-3 md:px-5">
          <div className="mx-auto flex max-w-[1440px] flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="flex min-w-0 items-start gap-3">
              <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Sparkles className="size-4" />
              </span>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold">{t('initialSetup.title')}</h2>
                <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                  {t('initialSetup.description')}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 pl-11 text-xs xl:shrink-0 xl:pl-0">
              {initialSetupChecks.map(check => (
                <span
                  key={check.label}
                  className={cn(
                    'inline-flex items-center gap-1.5',
                    check.ready ? 'text-success' : 'text-muted-foreground'
                  )}
                >
                  {check.ready ? (
                    <CheckCircle2 className="size-3.5" />
                  ) : (
                    <CircleDashed className="size-3.5" />
                  )}
                  {check.label}
                </span>
              ))}
              <span className="text-muted-foreground">
                {t('initialSetup.progress', {
                  ready: initialSetupReadyCount,
                  total: initialSetupChecks.length,
                })}
              </span>
            </div>
          </div>
        </section>
      ) : null}
      <div
        ref={bodyRef}
        className={cn('min-h-0 min-w-0 flex-1', twoColumns ? 'overflow-hidden' : 'overflow-y-auto')}
      >
        <div
          className={cn(
            'grid min-h-full min-w-0 grid-cols-1',
            twoColumns && 'h-full min-h-0 grid-cols-[minmax(0,1fr)_minmax(0,1fr)] divide-x',
            inlinePreview && 'grid-cols-[minmax(0,0.95fr)_minmax(0,0.95fr)_minmax(0,1.2fr)]'
          )}
        >
          {/* Who the digital human is: the counterpart of an Agent's prompt. */}
          <section
            id="configuration-identity"
            className={cn('min-w-0 border-b', twoColumns && 'h-full min-h-0 overflow-y-auto border-b-0')}
          >
            <div className="flex min-h-12 items-center justify-between gap-3 border-b px-4 py-2 md:px-5">
              <div className="min-w-0">
                <h2 className="text-sm font-semibold">{t('settings.behaviorTitle')}</h2>
                <p className="truncate text-xs text-muted-foreground">{t('settings.behaviorDescription')}</p>
              </div>
              {editingEnabled ? (
                <PersonaTemplateActions
                  identityName={displayedIdentityName}
                  persona={displayedPersona}
                  goals={displayedGoals.map(item => item.trim()).filter(Boolean)}
                  rules={displayedRules.map(item => item.trim()).filter(Boolean)}
                  memoryInstructions={memorySettings.instructions}
                  skillIds={draftSkillIds}
                  onApply={template => {
                    // A template sets the name only when it has one of its own.
                    if (template.identity_name?.trim()) setIdentityName(template.identity_name.trim());
                    setPersona(template.persona);
                    setGoals(template.goals.length > 0 ? template.goals : ['']);
                    setRules(template.rules.length > 0 ? template.rules : ['']);
                    if (template.memory_instructions.trim()) {
                      setMemorySettings(current => ({
                        ...current,
                        instructions: template.memory_instructions.trim(),
                      }));
                    }
                    // Skills are bindings with their own authorization, so a
                    // template only suggests the ones not bound yet.
                    const missing = template.skills
                      .map(skill => skill.id)
                      .filter(id => !draftSkillIds.includes(id));
                    if (missing.length > 0) {
                      toast.info(t('personaTemplates.suggestSkills', { skills: missing.join(', ') }));
                    }
                  }}
                />
              ) : null}
            </div>
            <div className="space-y-5 p-4 md:p-5">
              <Field
                label={t('fields.identityName')}
                description={t('settings.identityNameDescription')}
                required
              >
                <Input
                  value={displayedIdentityName}
                  onChange={event => setIdentityName(event.target.value)}
                  disabled={!editingEnabled}
                  placeholder={t('settings.identityNamePlaceholder')}
                  aria-label={t('fields.identityName')}
                  aria-required="true"
                />
              </Field>
              <Field
                label={t('fields.persona')}
                description={t('settings.personaDescription')}
                required
              >
                <Textarea
                  rows={7}
                  value={displayedPersona}
                  onChange={event => setPersona(event.target.value)}
                  disabled={!editingEnabled}
                  placeholder={t('settings.personaPlaceholder')}
                  aria-label={t('fields.persona')}
                  aria-required="true"
                />
              </Field>
              {/* What it is for comes before what limits it. */}
              <Field
                label={t('fields.goals')}
                description={t('settings.goalsDescription')}
                action={
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-7 shrink-0 gap-1.5 px-2.5"
                    onClick={() => setGoals([...goals, ''])}
                    disabled={!editingEnabled}
                  >
                    <Plus className="size-3.5" />
                    {t('settings.addGoal')}
                  </Button>
                }
              >
                <RulesEditor
                  rules={displayedGoals}
                  onChange={setGoals}
                  disabled={!editingEnabled}
                  removeLabel={t('settings.removeGoal')}
                  placeholder={t('settings.goalsPlaceholder')}
                />
              </Field>
              <Field
                label={t('fields.rules')}
                description={t('settings.rulesDescription')}
                action={
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-7 shrink-0 gap-1.5 px-2.5"
                    onClick={() => setRules([...rules, ''])}
                    disabled={!editingEnabled}
                  >
                    <Plus className="size-3.5" />
                    {t('settings.addRule')}
                  </Button>
                }
              >
                <RulesEditor
                  rules={displayedRules}
                  onChange={setRules}
                  disabled={!editingEnabled}
                  removeLabel={t('settings.removeRule')}
                  placeholder={t('settings.rulesPlaceholder')}
                />
              </Field>
            </div>
          </section>

          {/* How it works: model, capabilities, conversation, and runtime. */}
          <section
            className={cn('min-w-0 bg-muted/[0.08]', twoColumns && 'h-full min-h-0 overflow-y-auto')}
          >
            <nav
              aria-label={t('configurationHub.sectionNav')}
              className="sticky top-0 z-10 flex min-h-12 flex-wrap items-center gap-1 border-b bg-background px-3 py-2 md:px-4"
            >
              {orchestrationSections.map(section => (
                <Button
                  key={section.id}
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 shrink-0 px-2.5 text-xs"
                  onClick={() =>
                    document.getElementById(section.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
                  }
                >
                  {t(section.label)}
                </Button>
              ))}
              {editingEnabled && detail.published && detail.has_unpublished_changes ? (
                <Badge variant="info" className="ml-auto shrink-0">
                  {t('capabilities.pendingPublish')}
                </Badge>
              ) : null}
            </nav>
            {/* Leave room for the floating preview button. */}
            <div className={cn('space-y-4 p-4 md:p-5', previewAvailable && !inlinePreview && 'pb-20 md:pb-20')}>
              {editingEnabled ? <DigitalHumanSetupChecklist id={id} /> : null}
              <ConfigurationSection id="configuration-model" title={t('settings.modelTitle')} description={t('settings.modelDescription')}>
                <ModelSelector
                  modelType="text-chat"
                  availabilityUseCase="agent-runtime"
                  preferredUseCase="agent"
                  capabilityFilter={{ features_tool_call: true }}
                  modelsOverride={models}
                  value={modelValue}
                  onChange={value => setModel(value.model)}
                  placeholder={t('settings.modelPlaceholder')}
                  disabled={!editingEnabled || availableModels.isLoading}
                  hasError={modelUnavailable}
                  className="w-full"
                />
                {modelUnavailable ? (
                  <div className="mt-2 flex items-start gap-2 rounded-md border border-destructive/20 bg-destructive/5 p-2 text-xs text-destructive">
                    <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                    <span>{t('settings.modelUnavailable')}</span>
                  </div>
                ) : null}
              </ConfigurationSection>

              <ConfigurationSection
                id="configuration-capabilities"
                title={t('capabilities.title')}
                description={editingEnabled ? t('capabilities.draftHint') : undefined}
              >
                <DigitalHumanResourceBindings
                  id={id}
                  codeExecution={{
                    enabled: displayedExecutionEnabled,
                    // Switch it on in the draft, then show where it lives.
                    onEnable:
                      editingEnabled && canEnableExecution
                        ? () => {
                            setExecutionEnabled(true);
                            document
                              .getElementById('configuration-runtime')
                              ?.scrollIntoView({ behavior: 'smooth', block: 'center' });
                          }
                        : undefined,
                  }}
                />
                <DigitalHumanWorkJournal
                  id={id}
                  canEdit={editingEnabled}
                  onAppendPrompt={prompt => {
                    if (editingEnabled) {
                      setPersona(current => appendWorkJournalPrompt(current, prompt));
                    }
                  }}
                />
              </ConfigurationSection>

              <ConfigurationSection
                id="configuration-memory"
                title={t('memoryInstructions.title')}
                description={t('memoryInstructions.description')}
              >
                <DigitalHumanMemorySettings
                  digitalHumanId={id}
                  value={
                    previewVersion ? memorySettingsOf(previewVersion.memory_policy) : memorySettings
                  }
                  onChange={setMemorySettings}
                  disabled={!editingEnabled}
                />
              </ConfigurationSection>

              <ConfigurationSection
                id="configuration-conversation"
                title={t('configurationHub.conversationTitle')}
                description={t('configurationHub.conversationDescription')}
              >
                <PolicyFileUploadSettings
                  value={previewVersion ? previewVersion.file_upload_policy : fileUploadPolicy}
                  onChange={setFileUploadPolicy}
                  disabled={!editingEnabled}
                />
              </ConfigurationSection>

              <ConfigurationSection
                id="configuration-runtime"
                title={t('preferences.runtimeTitle')}
                description={t('preferences.runtimeDescription')}
              >
                <div className="rounded-md border bg-background px-3">
                  <PreferenceSwitch
                    title={t('preferences.linuxEnvironment')}
                    description={t('preferences.linuxEnvironmentDescription')}
                    checked={displayedExecutionEnabled}
                    onCheckedChange={setExecutionEnabled}
                    disabled={!editingEnabled || (!executionEnabled && !canEnableExecution)}
                    compact
                  />
                  {executionUnavailable ? (
                    <p className="-mt-1 pb-3 text-xs text-muted-foreground">
                      {t(linuxEnvironmentUnavailableKey(executionRuntimeCapability?.reason))}
                    </p>
                  ) : null}
                </div>
              </ConfigurationSection>
            </div>
          </section>
          {inlinePreview ? renderPreview('inline') : null}
        </div>
      </div>
      {previewAvailable && !inlinePreview ? (
        <Sheet open={previewOpen} onOpenChange={setPreviewOpen}>
          <SheetContent
            side="right"
            showClose={false}
            className="flex h-full min-h-0 w-[min(720px,100vw)] max-w-none flex-col p-0 sm:max-w-none"
          >
            <SheetTitle className="sr-only">{t('preview.title')}</SheetTitle>
            <SheetDescription className="sr-only">{t('preview.description')}</SheetDescription>
            {renderPreview('sheet')}
          </SheetContent>
        </Sheet>
      ) : null}
      {previewAvailable && !inlinePreview && !previewOpen ? (
        <Button
          type="button"
          size="sm"
          className="fixed bottom-5 right-5 z-20 gap-1.5 shadow-md"
          onClick={() => setPreviewOpen(true)}
        >
          <MessageSquareText className="size-4" />
          {t('preview.open')}
        </Button>
      ) : null}
    </div>
  );
}

const orchestrationSections = [
  { id: 'configuration-model', label: 'settings.modelTitle' },
  { id: 'configuration-capabilities', label: 'capabilities.title' },
  { id: 'configuration-memory', label: 'memoryInstructions.title' },
  { id: 'configuration-conversation', label: 'configurationHub.conversationTitle' },
  { id: 'configuration-runtime', label: 'preferences.runtimeTitle' },
] as const;

function ConfigurationSection({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-14 rounded-xl border bg-background p-4">
      <h3 className="text-sm font-semibold">{title}</h3>
      {description ? <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p> : null}
      <div className="mt-3">{children}</div>
    </section>
  );
}

function RulesEditor({
  rules,
  onChange,
  disabled,
  required,
  removeLabel,
  placeholder,
}: {
  rules: string[];
  onChange: (rules: string[]) => void;
  disabled: boolean;
  required?: boolean;
  removeLabel: string;
  placeholder: string;
}) {
  const focusRule = (editor: HTMLElement | null, index: number) => {
    requestAnimationFrame(() => {
      editor?.querySelectorAll<HTMLInputElement>('input[data-rule-input]')[index]?.focus();
    });
  };
  const updateRule = (index: number, value: string) => {
    onChange(rules.map((rule, ruleIndex) => (ruleIndex === index ? value : rule)));
  };
  const addRule = (afterIndex = rules.length - 1) => {
    const nextRules = [...rules];
    nextRules.splice(afterIndex + 1, 0, '');
    onChange(nextRules);
  };
  const removeRule = (index: number) => {
    const nextRules = rules.filter((_, ruleIndex) => ruleIndex !== index);
    onChange(nextRules.length > 0 ? nextRules : ['']);
  };
  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>, index: number) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      const editor = event.currentTarget.closest<HTMLElement>('[data-rules-editor]');
      addRule(index);
      focusRule(editor, index + 1);
      return;
    }
    if (event.key === 'Backspace' && !rules[index] && rules.length > 1) {
      event.preventDefault();
      const editor = event.currentTarget.closest<HTMLElement>('[data-rules-editor]');
      removeRule(index);
      focusRule(editor, Math.max(0, index - 1));
    }
  };
  const handlePaste = (event: ClipboardEvent<HTMLInputElement>, index: number) => {
    const pastedRules = event.clipboardData
      .getData('text')
      .split(/\r?\n/)
      .map(item => item.trim())
      .filter(Boolean);
    if (pastedRules.length <= 1) return;
    event.preventDefault();
    const editor = event.currentTarget.closest<HTMLElement>('[data-rules-editor]');
    const nextRules = [...rules];
    nextRules.splice(index, 1, ...pastedRules);
    onChange(nextRules);
    focusRule(editor, index + pastedRules.length - 1);
  };

  return (
    <div className="space-y-3" data-rules-editor role="group" aria-required={required || undefined}>
      <div className="space-y-2">
        {rules.map((rule, index) => (
          <div
            key={index}
            className="group flex min-w-0 items-center gap-2 rounded-lg border bg-background p-1.5 transition-colors focus-within:border-primary hover:border-border-strong"
          >
            <span
              className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted/70 text-xs font-medium text-muted-foreground"
              aria-hidden="true"
            >
              {index + 1}
            </span>
            <Input
              data-rule-input
              containerClassName="min-w-0 flex-1"
              className="h-8 border-0 bg-transparent px-1 shadow-none hover:border-0 focus-visible:border-0"
              value={rule}
              onChange={event => updateRule(index, event.target.value)}
              onKeyDown={event => handleKeyDown(event, index)}
              onPaste={event => handlePaste(event, index)}
              disabled={disabled}
              placeholder={placeholder}
              aria-label={`${index + 1}. ${placeholder}`}
            />
            <Button
              type="button"
              variant="ghost"
              size="sm"
              isIcon
              className="size-8 shrink-0 text-muted-foreground opacity-70 hover:text-destructive group-hover:opacity-100 focus-visible:opacity-100"
              onClick={() => removeRule(index)}
              disabled={disabled}
              aria-label={`${removeLabel} ${index + 1}`}
              title={removeLabel}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}

function serializeEditorPart(value: unknown) {
  return JSON.stringify(value);
}

// The server reports why the Linux environment cannot be enabled; each reason
// needs a different administrator action, so it is shown instead of one
// generic sentence.
const linuxEnvironmentUnavailableReasons = [
  'disabled',
  'organization_not_enabled',
  'entry_not_enabled',
  'runtime_unavailable',
] as const;

function linuxEnvironmentUnavailableKey(reason?: string) {
  const known = linuxEnvironmentUnavailableReasons.find(item => item === reason);
  return known
    ? (`preferences.linuxEnvironmentUnavailableReasons.${known}` as const)
    : ('preferences.linuxEnvironmentUnavailable' as const);
}

function PreferenceSwitch({
  title,
  description,
  checked,
  onCheckedChange,
  disabled,
  compact = false,
}: {
  title: string;
  description: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled: boolean;
  compact?: boolean;
}) {
  return (
    <div className={cn('flex items-start justify-between gap-6', compact ? 'py-3' : 'py-5')}>
      <div className="max-w-3xl space-y-1">
        <div className="text-sm font-medium">{title}</div>
        <p className="text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        aria-label={title}
      />
    </div>
  );
}

function PersonaTemplateActions({
  identityName,
  persona,
  goals,
  rules,
  memoryInstructions,
  skillIds,
  onApply,
}: {
  identityName: string;
  persona: string;
  goals: string[];
  rules: string[];
  memoryInstructions: string;
  skillIds: string[];
  onApply: (template: DigitalHumanTemplate) => void;
}) {
  const t = useT('digitalHumans');
  const [libraryOpen, setLibraryOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  return (
    <div className="flex shrink-0 gap-2">
      <Button type="button" variant="outline" size="sm" onClick={() => setLibraryOpen(true)}>
        <Sparkles className="size-4" />
        {t('personaTemplates.use')}
      </Button>
      <Button type="button" variant="ghost" size="sm" onClick={() => setCreateOpen(true)}>
        {t('personaTemplates.save')}
      </Button>
      <PersonaTemplateLibrary
        open={libraryOpen}
        onOpenChange={setLibraryOpen}
        onApply={onApply}
        onCreate={() => setCreateOpen(true)}
      />
      <PersonaTemplateEditor
        open={createOpen}
        onOpenChange={setCreateOpen}
        initial={{
          name: '',
          identity_name: identityName,
          description: '',
          persona,
          goals,
          rules,
          memory_instructions: memoryInstructions,
          skill_ids: skillIds,
        }}
      />
    </div>
  );
}

// The fields a saved template's editor starts from.
type PersonaTemplateDraft = Omit<CreateDigitalHumanPersonaTemplateRequest, never>;

// savedTemplateDraft turns a catalog entry of the workspace back into what
// its editor edits.
function savedTemplateDraft(template: DigitalHumanTemplate): PersonaTemplateDraft {
  return {
    name: template.title,
    identity_name: template.identity_name ?? '',
    description: template.description,
    persona: template.persona,
    goals: template.goals,
    rules: template.rules,
    memory_instructions: template.memory_instructions,
    skill_ids: template.skills.map(skill => skill.id),
  };
}

// The library lists the same catalog as the creation page (ADR 0022): the
// built-in templates, then the workspace's saved ones, which can also be
// edited and deleted here.
function PersonaTemplateLibrary({
  open,
  onOpenChange,
  onApply,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApply: (template: DigitalHumanTemplate) => void;
  onCreate: () => void;
}) {
  const pageSize = 4;
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const catalogQuery = useDigitalHumanTemplates({ locale, search: search.trim() }, open);
  const mutations = useDigitalHumanMutations();
  const [editingTemplate, setEditingTemplate] = useState<DigitalHumanTemplate | null>(null);
  const allTemplates = catalogQuery.data?.data?.templates ?? [];
  const total = allTemplates.length;
  const savedCount = allTemplates.filter(template => template.source === 'workspace').length;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const templates = allTemplates.slice((page - 1) * pageSize, page * pageSize);
  useEffect(() => {
    if (catalogQuery.data && !catalogQuery.isLoading && page > totalPages) {
      setPage(totalPages);
    }
  }, [page, catalogQuery.data, catalogQuery.isLoading, totalPages]);
  const apply = (template: DigitalHumanTemplate) => {
    onApply(template);
    onOpenChange(false);
  };
  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="h-[min(640px,calc(100vh-2rem))] max-w-3xl overflow-hidden">
          <DialogHeader>
            <DialogTitle>{t('personaTemplates.title')}</DialogTitle>
            <DialogDescription>{t('personaTemplates.description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden pb-6 pt-2">
            <Input
              value={search}
              onChange={event => {
                setSearch(event.target.value);
                setPage(1);
              }}
              placeholder={t('personaTemplates.search')}
              leftIcon={<Search className="size-4" />}
            />
            <div className="grid min-h-0 flex-1 auto-rows-[minmax(176px,1fr)] gap-3 overflow-y-auto pr-1 sm:grid-cols-2">
              {templates.map(template => (
                <div
                  key={`${template.source}:${template.id}`}
                  className="flex min-h-44 flex-col rounded-xl border bg-card p-4 transition-colors hover:border-border-strong"
                >
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 truncate font-medium">{template.title}</span>
                    {template.source === 'builtin' ? (
                      <Badge variant="subtle" className="shrink-0">
                        {t('personaTemplates.builtin')}
                      </Badge>
                    ) : null}
                  </div>
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                    {template.description || template.goals[0] || template.persona}
                  </p>
                  <div className="mt-3 line-clamp-3 text-sm">{template.persona}</div>
                  <div className="mt-auto flex justify-end gap-1 pt-4">
                    {template.source === 'workspace' ? (
                      <>
                        <ConfirmDialog
                          title={t('personaTemplates.deleteTitle')}
                          description={t('personaTemplates.deleteDescription', {
                            name: template.title,
                          })}
                          confirmText={t('common.delete')}
                          cancelText={t('common.cancel')}
                          variant="danger"
                          loading={mutations.deletePersonaTemplate.isPending}
                          onConfirm={() =>
                            void mutations.deletePersonaTemplate.mutateAsync({
                              templateId: template.id,
                              expectedRevision: template.revision ?? 0,
                            })
                          }
                          trigger={
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              className="text-muted-foreground hover:text-destructive"
                            >
                              <Trash2 className="size-4" />
                              {t('common.delete')}
                            </Button>
                          }
                        />
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setEditingTemplate(template);
                            onOpenChange(false);
                          }}
                        >
                          <Pencil className="size-4" />
                          {t('common.edit')}
                        </Button>
                      </>
                    ) : null}
                    <ConfirmDialog
                      title={t('personaTemplates.applyTitle')}
                      description={t('personaTemplates.applyDescription', { name: template.title })}
                      confirmText={t('personaTemplates.apply')}
                      cancelText={t('common.cancel')}
                      onConfirm={() => apply(template)}
                      trigger={
                        <Button type="button" size="sm">
                          {t('personaTemplates.apply')}
                        </Button>
                      }
                    />
                  </div>
                </div>
              ))}
              {catalogQuery.isLoading ? (
                <div className="col-span-full flex min-h-full items-center justify-center text-sm text-muted-foreground">
                  <Loader2 className="mr-2 size-4 animate-spin" />
                  {t('personaTemplates.loading')}
                </div>
              ) : null}
              {!catalogQuery.isLoading && templates.length === 0 ? (
                <div className="col-span-full flex min-h-full flex-col items-center justify-center rounded-2xl border border-dashed bg-muted/20 px-6 py-12 text-center">
                  <span className="flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                    <Sparkles className="size-5" />
                  </span>
                  <p className="mt-4 text-sm font-medium text-foreground">
                    {t('personaTemplates.noResultsTitle')}
                  </p>
                  <p className="mt-1 max-w-md text-sm leading-6 text-muted-foreground">
                    {t('personaTemplates.noResultsDescription')}
                  </p>
                </div>
              ) : null}
            </div>
            {!catalogQuery.isLoading && savedCount === 0 && !search.trim() ? (
              <div className="flex shrink-0 items-center justify-between gap-3 rounded-lg bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                <span>{t('personaTemplates.emptyDescription')}</span>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    onOpenChange(false);
                    onCreate();
                  }}
                >
                  <Plus className="size-4" />
                  {t('personaTemplates.createFirst')}
                </Button>
              </div>
            ) : null}
            {!catalogQuery.isLoading && totalPages > 1 ? (
              <div className="shrink-0 border-t pt-4">
                <Pagination
                  currentPage={page}
                  totalPages={totalPages}
                  total={total}
                  pageSize={pageSize}
                  onPageChange={setPage}
                  showJump={false}
                  renderInfo={(start, end, totalItems) =>
                    t('personaTemplates.paginationInfo', {
                      start,
                      end,
                      total: totalItems,
                    })
                  }
                />
              </div>
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <PersonaTemplateEditor
        open={Boolean(editingTemplate)}
        onOpenChange={nextOpen => {
          if (!nextOpen) setEditingTemplate(null);
        }}
        template={editingTemplate}
        initial={editingTemplate ? savedTemplateDraft(editingTemplate) : null}
      />
    </>
  );
}

// Saves the current draft, or edits a saved template. Besides what is shown,
// a saved template keeps the draft's memory instructions and bound skills
// (ADR 0022); connector connections stay with each DigitalHuman.
function PersonaTemplateEditor({
  open,
  onOpenChange,
  initial,
  template,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial: PersonaTemplateDraft | null;
  template?: DigitalHumanTemplate | null;
}) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanMutations();
  const nameInputRef = useRef<HTMLInputElement>(null);
  const [name, setName] = useState('');
  const [identityName, setIdentityName] = useState('');
  const [description, setDescription] = useState('');
  const [persona, setPersona] = useState('');
  const [goals, setGoals] = useState<string[]>(['']);
  const [rules, setRules] = useState<string[]>(['']);
  const { locale } = useLocale();
  // The skills the template carries: the draft's by default, each removable.
  const [skillIds, setSkillIds] = useState<string[]>([]);
  const skillsQuery = useAIChatSkills({ enabled: open });
  const skillLabel = (id: string) => {
    const skill = (skillsQuery.data ?? []).find(item => item.skill_id === id);
    return skill ? getAIChatSkillDisplayInfo(skill, locale).label : id;
  };
  useEffect(() => {
    if (!open || !initial) return;
    setName(initial.name);
    setIdentityName(initial.identity_name);
    setDescription(initial.description);
    setPersona(initial.persona);
    setGoals(initial.goals.length > 0 ? initial.goals : ['']);
    setRules(initial.rules.length > 0 ? initial.rules : ['']);
    setSkillIds([...initial.skill_ids]);
    // Seeding happens once per opening; later renders pass a fresh object.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, template]);
  const save = async () => {
    if (!name.trim() || !persona.trim()) {
      toast.error(t('errors.required'));
      return;
    }
    const payload: CreateDigitalHumanPersonaTemplateRequest = {
      name: name.trim(),
      identity_name: identityName.trim(),
      description: description.trim(),
      persona: persona.trim(),
      goals: goals.map(item => item.trim()).filter(Boolean),
      rules: rules.map(item => item.trim()).filter(Boolean),
      memory_instructions: initial?.memory_instructions.trim() ?? '',
      skill_ids: skillIds,
    };
    if (template) {
      await mutations.updatePersonaTemplate.mutateAsync({
        templateId: template.id,
        request: { expected_revision: template.revision ?? 0, ...payload },
      });
    } else {
      await mutations.createPersonaTemplate.mutateAsync(payload);
    }
    onOpenChange(false);
  };
  const pending = template
    ? mutations.updatePersonaTemplate.isPending
    : mutations.createPersonaTemplate.isPending;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        onOpenAutoFocus={event => {
          event.preventDefault();
          requestAnimationFrame(() => nameInputRef.current?.focus());
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {template ? t('personaTemplates.editTitle') : t('personaTemplates.save')}
          </DialogTitle>
          <DialogDescription>
            {template
              ? t('personaTemplates.editDescription')
              : t('personaTemplates.saveDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <Field
            label={t('personaTemplates.templateName')}
            description={t('personaTemplates.templateNameDescription')}
            required
          >
            <Input
              ref={nameInputRef}
              value={name}
              onChange={event => setName(event.target.value)}
              aria-label={t('personaTemplates.templateName')}
              aria-required="true"
            />
          </Field>
          <Field
            label={t('personaTemplates.templateDescription')}
            description={t('personaTemplates.templateDescriptionHint')}
          >
            <Input
              value={description}
              onChange={event => setDescription(event.target.value)}
              aria-label={t('personaTemplates.templateDescription')}
            />
          </Field>
          <Field
            label={t('fields.identityName')}
            description={t('personaTemplates.identityNameDescription')}
          >
            <Input
              value={identityName}
              onChange={event => setIdentityName(event.target.value)}
              aria-label={t('fields.identityName')}
            />
          </Field>
          <Field
            label={t('fields.persona')}
            description={t('settings.personaDescription')}
            required
          >
            <Textarea
              rows={5}
              value={persona}
              onChange={event => setPersona(event.target.value)}
              aria-label={t('fields.persona')}
              aria-required="true"
            />
          </Field>
          <Field
            label={t('fields.rules')}
            description={t('settings.rulesDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-7 shrink-0 gap-1.5 px-2.5"
                onClick={() => setRules([...rules, ''])}
              >
                <Plus className="size-3.5" />
                {t('settings.addRule')}
              </Button>
            }
          >
            <RulesEditor
              rules={rules}
              onChange={setRules}
              disabled={false}
              removeLabel={t('settings.removeRule')}
              placeholder={t('settings.rulesPlaceholder')}
            />
          </Field>
          <Field
            label={t('fields.goals')}
            description={t('settings.goalsDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-7 shrink-0 gap-1.5 px-2.5"
                onClick={() => setGoals([...goals, ''])}
              >
                <Plus className="size-3.5" />
                {t('settings.addGoal')}
              </Button>
            }
          >
            <RulesEditor
              rules={goals}
              onChange={setGoals}
              disabled={false}
              removeLabel={t('settings.removeGoal')}
              placeholder={t('settings.goalsPlaceholder')}
            />
          </Field>
          {initial && initial.skill_ids.length > 0 ? (
            <Field
              label={t('personaTemplates.skillsLabel')}
              description={t('personaTemplates.skillsDescription')}
            >
              <div className="space-y-1.5">
                {initial.skill_ids.map(id => {
                  const deleted =
                    skillsQuery.isSuccess &&
                    !(skillsQuery.data ?? []).some(item => item.skill_id === id);
                  return (
                    <label key={id} className="flex items-center gap-2 text-sm">
                      <Checkbox
                        checked={skillIds.includes(id)}
                        onCheckedChange={checked =>
                          setSkillIds(current =>
                            checked === true
                              ? [...new Set([...current, id])]
                              : current.filter(value => value !== id)
                          )
                        }
                      />
                      <span className="min-w-0 truncate">{skillLabel(id)}</span>
                      {deleted ? (
                        <span className="text-xs text-warning">
                          {t('personaTemplates.skillDeleted')}
                        </span>
                      ) : null}
                    </label>
                  );
                })}
              </div>
            </Field>
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t('personaTemplates.snapshotHint', {
              goalCount: goals.filter(item => item.trim()).length,
              ruleCount: rules.filter(item => item.trim()).length,
              skillCount: skillIds.length,
            })}
            {initial?.memory_instructions.trim() ? ` ${t('personaTemplates.keepsMemory')}` : ''}
          </p>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => void save()} disabled={pending}>
            {pending ? <Loader2 className="size-4 animate-spin" /> : null}
            {template ? t('common.save') : t('personaTemplates.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  description,
  required,
  action,
  children,
}: {
  label: string;
  description: string;
  required?: boolean;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5 text-sm font-medium">
      <div className="flex min-h-7 items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-1.5">
          <span>{label}</span>
          {required ? (
            <span className="self-start text-sm leading-none text-destructive" aria-hidden="true">
              *
            </span>
          ) : null}
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="inline-flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                aria-label={description}
              >
                <CircleHelp className="size-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="max-w-72 leading-5">{description}</TooltipContent>
          </Tooltip>
        </div>
        {action}
      </div>
      {children}
    </div>
  );
}
