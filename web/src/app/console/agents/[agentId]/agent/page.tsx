'use client';

import { use, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createAgentDraftTransport, useAIChatController } from '@/components/chat';
import {
  getAIChatSkillDisplayInfo,
  isHiddenSystemSkill,
} from '@/components/chat/variants/aichat/skill-display';
import type {
  ModelSelectorParameterValue,
  ModelSelectorValue,
} from '@/components/common/model-selector';
import {
  AgentRuntimeHeader,
  AgentRuntimeLoadingState,
  AgentRuntimeMemoryValuesDialog,
  AgentRuntimeOrchestrationPanel,
  AgentRuntimePreviewPanel,
  AgentRuntimePromptPanel,
  AgentRuntimeVersionPopover,
  AgentRuntimeSkillDialog,
  pickAgentInitials,
  toModelParams,
  useAgentRuntimeDraftPersistence,
  useAgentRuntimeLeaveGuard,
  validateAgentMemorySlots,
  type AgentConfigSection,
  type AgentRuntimeDraftPersistenceSnapshot,
  type AgentPublishedVersionListItem,
} from '@/components/agents/agent-runtime';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import { useAgent, useAgentConfig, usePublishAgent } from '@/hooks/agent/use-agents';
import { useAIChatSkills } from '@/hooks/aichat/use-aichat-skills';
import { useDatasets } from '@/hooks/dataset/use-datasets';
import { useMediaQuery } from '@/hooks/use-media-query';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useAutoProfile } from '@/hooks/use-profile';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type {
  AgentMemorySlotConfig,
  AgentRuntimeConfig,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentRuntimePageProps {
  params: Promise<{ agentId: string }>;
}

function getSaveText(
  t: ReturnType<typeof useT<'agents.agentRuntime'>>,
  saveState: ReturnType<typeof useAgentRuntimeDraftPersistence>['saveState'],
  lastSavedAt: number | null
) {
  if (saveState === 'saving') return t('saveState.saving');
  if (saveState === 'previewing') return t('saveState.previewing');
  if (saveState === 'dirty') return t('saveState.dirty');
  if (saveState === 'error') return t('saveState.error');
  if (lastSavedAt) {
    return t('saveState.savedAt', {
      time: new Date(lastSavedAt * 1000).toLocaleTimeString(undefined, { hour12: false }),
    });
  }
  return t('saveState.saved');
}

interface VersionPreviewBackup {
  payload: UpdateAgentRuntimeConfigRequest;
  persistence: AgentRuntimeDraftPersistenceSnapshot;
}

export default function AgentRuntimePage({ params }: AgentRuntimePageProps) {
  const { agentId } = use(params);
  const queryClient = useQueryClient();
  const { locale } = useLocale();
  const t = useT('agents.agentRuntime');
  const { agent, isLoading: isAgentLoading } = useAgent(agentId);
  const { data: profile } = useAutoProfile({ staleTime: 1_800_000 });
  const { data: configResponse, isLoading: isConfigLoading } = useAgentConfig(agentId);
  const { data: allSkills = [], isLoading: isSkillsLoading } = useAIChatSkills();
  const { pages: datasetPages, isLoading: isDatasetsLoading } = useDatasets({ limit: 100 });
  const publishAgent = usePublishAgent();
  const config = configResponse?.data;
  const agentDetail = agent?.data;
  const defaultHomeTitle = agentDetail?.name?.trim() || t('defaultHomeTitle');
  const defaultInputPlaceholder = t('defaultInputPlaceholder');

  const draftTransport = useMemo(() => createAgentDraftTransport(agentId), [agentId]);
  const chatController = useAIChatController({ transport: draftTransport });
  const initChatController = chatController.init;

  const [systemPrompt, setSystemPrompt] = useState('');
  const [modelValue, setModelValue] = useState<ModelSelectorParameterValue>({
    provider: '',
    model: '',
    params: {},
  });
  const [selectedSkillIds, setSelectedSkillIds] = useState<string[]>([]);
  const [agentMemoryEnabled, setAgentMemoryEnabled] = useState(false);
  const [agentMemorySlots, setAgentMemorySlots] = useState<AgentMemorySlotConfig[]>([]);
  const [fileUploadEnabled, setFileUploadEnabled] = useState(false);
  const [homeTitle, setHomeTitle] = useState(defaultHomeTitle);
  const [inputPlaceholder, setInputPlaceholder] = useState(defaultInputPlaceholder);
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const [knowledgeDatasetIds, setKnowledgeDatasetIds] = useState<string[]>([]);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [promptOptimizerOpen, setPromptOptimizerOpen] = useState(false);
  const [memoryValuesOpen, setMemoryValuesOpen] = useState(false);
  const [skillSearch, setSkillSearch] = useState('');
  const [showSelectedSkillsOnly, setShowSelectedSkillsOnly] = useState(false);
  const [isGeneratingSuggestions, setIsGeneratingSuggestions] = useState(false);
  const [publishedVersionsOpen, setPublishedVersionsOpen] = useState(false);
  const [previewSheetOpen, setPreviewSheetOpen] = useState(false);
  const isTwoXlViewport = useMediaQuery('(min-width: 1536px)');
  const [publishedVersions, setPublishedVersions] = useState<AgentPublishedVersionListItem[]>([]);
  const [isLoadingVersions, setIsLoadingVersions] = useState(false);
  const [isRollingBackVersion, setIsRollingBackVersion] = useState(false);
  const [selectedPublishedVersionId, setSelectedPublishedVersionId] = useState('');
  const [isVersionPreviewing, setIsVersionPreviewing] = useState(false);
  const [openSections, setOpenSections] = useState<Record<AgentConfigSection, boolean>>({
    experience: true,
    model: true,
    skills: true,
    knowledge: true,
    files: true,
    memory: true,
  });
  const hydratedAgentIdRef = useRef<string | null>(null);
  const versionPreviewBackupRef = useRef<VersionPreviewBackup | null>(null);

  const selectableSkills = useMemo(
    () =>
      allSkills.filter(skill => {
        if (isHiddenSystemSkill(skill.skill_id)) return false;
        const callers = skill.supported_callers ?? [];
        return callers.length === 0 || callers.includes('agent');
      }),
    [allSkills]
  );
  const availableDatasets = useMemo(() => datasetPages.flat(), [datasetPages]);
  const selectableSkillIds = useMemo(
    () => new Set(selectableSkills.map(skill => skill.skill_id)),
    [selectableSkills]
  );
  const normalizedSelectedSkillIds = useMemo(
    () => selectedSkillIds.filter(id => selectableSkillIds.has(id)),
    [selectableSkillIds, selectedSkillIds]
  );
  const selectedSkills = useMemo(
    () =>
      normalizedSelectedSkillIds
        .map(id => selectableSkills.find(skill => skill.skill_id === id))
        .filter((skill): skill is AIChatSkillMetadata => Boolean(skill)),
    [normalizedSelectedSkillIds, selectableSkills]
  );
  const dialogSkills = useMemo(() => {
    const query = skillSearch.trim().toLowerCase();
    return selectableSkills
      .filter(skill => {
        const display = getAIChatSkillDisplayInfo(skill, locale);
        const checked = normalizedSelectedSkillIds.includes(skill.skill_id);
        if (showSelectedSkillsOnly && !checked) return false;
        if (!query) return true;
        return [skill.skill_id, display.label, display.description, skill.description]
          .filter(Boolean)
          .some(value => String(value).toLowerCase().includes(query));
      })
      .sort((left, right) => {
        const leftChecked = normalizedSelectedSkillIds.includes(left.skill_id);
        const rightChecked = normalizedSelectedSkillIds.includes(right.skill_id);
        if (leftChecked !== rightChecked) return leftChecked ? -1 : 1;
        const leftDisplay = getAIChatSkillDisplayInfo(left, locale);
        const rightDisplay = getAIChatSkillDisplayInfo(right, locale);
        return leftDisplay.label.localeCompare(rightDisplay.label, locale);
      });
  }, [locale, normalizedSelectedSkillIds, selectableSkills, showSelectedSkillsOnly, skillSearch]);
  const agentMemorySlotValidationErrors = useMemo(
    () => validateAgentMemorySlots(agentMemorySlots),
    [agentMemorySlots]
  );
  const hasAgentMemorySlotErrors =
    agentMemoryEnabled && agentMemorySlotValidationErrors.some(Boolean);

  const modelSelectorValue = useMemo(
    () => ({
      provider: modelValue.provider,
      model: modelValue.model,
      params: modelValue.params,
    }),
    [modelValue]
  );
  const currentPayload = useMemo<UpdateAgentRuntimeConfigRequest>(
    () => ({
      system_prompt: systemPrompt,
      model_provider: modelValue.provider,
      model: modelValue.model,
      model_parameters: modelValue.params,
      enabled_skill_ids: normalizedSelectedSkillIds,
      use_memory: false,
      agent_memory_enabled: agentMemoryEnabled,
      agent_memory_slots: agentMemorySlots.slice(0, 5).map((slot, index) => ({
        ...slot,
        description: slot.description.slice(0, 200),
        max_chars: 2000,
        sort_order: index,
      })),
      file_upload_enabled: fileUploadEnabled,
      home_title: homeTitle.trim() || defaultHomeTitle,
      input_placeholder: inputPlaceholder.trim() || defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: suggestedQuestions
        .map(item => item.trim())
        .filter(Boolean)
        .slice(0, 6),
      knowledge_dataset_ids: knowledgeDatasetIds,
      knowledge_retrieval_config: {},
    }),
    [
      defaultHomeTitle,
      defaultInputPlaceholder,
      fileUploadEnabled,
      agentMemoryEnabled,
      agentMemorySlots,
      homeTitle,
      inputPlaceholder,
      modelValue,
      normalizedSelectedSkillIds,
      suggestedQuestions,
      knowledgeDatasetIds,
      systemPrompt,
    ]
  );
  const agentHomeBrand = useMemo(
    () => (
      <div className="flex size-16 items-center justify-center rounded-2xl border border-primary/30 bg-primary/10 text-xl font-semibold text-primary shadow-sm">
        {agentDetail?.icon_type === 'image' && agentDetail.icon_url ? (
          <img src={agentDetail.icon_url} alt="" className="size-full rounded-2xl object-cover" />
        ) : (
          pickAgentInitials(agentDetail?.name)
        )}
      </div>
    ),
    [agentDetail?.icon_type, agentDetail?.icon_url, agentDetail?.name]
  );

  useEffect(() => {
    initChatController(null);
  }, [initChatController]);

  useEffect(() => {
    if (isTwoXlViewport && previewSheetOpen) {
      setPreviewSheetOpen(false);
    }
  }, [isTwoXlViewport, previewSheetOpen]);

  const applyRuntimePayload = useCallback((payload: UpdateAgentRuntimeConfigRequest) => {
    setSystemPrompt(payload.system_prompt);
    setModelValue({
      provider: payload.model_provider,
      model: payload.model,
      params: toModelParams(payload.model_parameters),
    });
    setSelectedSkillIds(payload.enabled_skill_ids);
    setAgentMemoryEnabled(payload.agent_memory_enabled ?? false);
    setAgentMemorySlots(payload.agent_memory_slots ?? []);
    setFileUploadEnabled(payload.file_upload_enabled);
    setHomeTitle(payload.home_title);
    setInputPlaceholder(payload.input_placeholder);
    setSuggestedQuestions(payload.suggested_questions);
    setKnowledgeDatasetIds(payload.knowledge_dataset_ids ?? []);
  }, []);

  const payloadFromRuntimeConfig = useCallback(
    (runtimeConfig: AgentRuntimeConfig): UpdateAgentRuntimeConfigRequest => ({
      system_prompt: runtimeConfig.system_prompt ?? '',
      model_provider: runtimeConfig.model_provider ?? '',
      model: runtimeConfig.model ?? '',
      model_parameters: toModelParams(runtimeConfig.model_parameters),
      enabled_skill_ids: runtimeConfig.enabled_skill_ids ?? [],
      use_memory: runtimeConfig.use_memory ?? false,
      agent_memory_enabled: runtimeConfig.agent_memory_enabled ?? false,
      agent_memory_slots: runtimeConfig.agent_memory_slots ?? [],
      file_upload_enabled: runtimeConfig.file_upload_enabled ?? false,
      home_title:
        !runtimeConfig.home_title || runtimeConfig.home_title === 'title'
          ? defaultHomeTitle
          : runtimeConfig.home_title,
      input_placeholder: runtimeConfig.input_placeholder ?? defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: runtimeConfig.suggested_questions ?? [],
      knowledge_dataset_ids: runtimeConfig.knowledge_dataset_ids ?? [],
      knowledge_retrieval_config: runtimeConfig.knowledge_retrieval_config ?? {},
    }),
    [defaultHomeTitle, defaultInputPlaceholder]
  );

  const saveRuntimePayload = useCallback(
    async (payload: UpdateAgentRuntimeConfigRequest) => {
      let slotsResponse: Awaited<ReturnType<typeof agentService.updateAgentMemorySlots>> | null =
        null;
      const payloadMemorySlotErrors = validateAgentMemorySlots(
        payload.agent_memory_slots ?? []
      ).some(Boolean);
      if (payload.agent_memory_enabled || !payloadMemorySlotErrors) {
        slotsResponse = await agentService.updateAgentMemorySlots(
          agentId,
          payload.agent_memory_slots ?? []
        );
      }
      const response = await agentService.updateAgentConfig(agentId, payload);
      const updatedAt = response.data.updated_at ?? Math.floor(Date.now() / 1000);
      const savedPayload = {
        ...payload,
        agent_memory_slots:
          slotsResponse?.data.slots ??
          response.data.agent_memory_slots ??
          payload.agent_memory_slots,
      };

      queryClient.setQueryData(AGENT_KEYS.config(agentId), {
        ...response,
        data: {
          ...response.data,
          agent_memory_enabled: payload.agent_memory_enabled,
          agent_memory_slots: savedPayload.agent_memory_slots,
        },
      });
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(agentId) });

      return {
        savedPayload,
        updatedAt,
      };
    },
    [agentId, queryClient]
  );

  const {
    saveState,
    lastSavedAt,
    isDirty,
    isSaving,
    saveNow,
    markHydrated,
    markServerSaved,
    setPreviewing,
    getSnapshot,
    restoreSnapshot,
  } = useAgentRuntimeDraftPersistence({
    currentPayload,
    enabled: !isVersionPreviewing,
    canSave: () => !hasAgentMemorySlotErrors,
    savePayload: saveRuntimePayload,
    onSaveCommitted: result => {
      setAgentMemorySlots(result.savedPayload.agent_memory_slots ?? []);
    },
    onSaveFailed: (error, options) => {
      if (!options.silent) {
        toast.error(getErrorMessage(error) || t('toasts.saveFailedDraftKept'));
      }
    },
  });

  useEffect(() => {
    if (!config || !agentDetail) return;
    if (hydratedAgentIdRef.current === agentId) return;

    const nextPayload = payloadFromRuntimeConfig(config);
    applyRuntimePayload(nextPayload);
    hydratedAgentIdRef.current = agentId;
    markHydrated(nextPayload, config.updated_at ?? null);
  }, [agentDetail, agentId, applyRuntimePayload, config, markHydrated, payloadFromRuntimeConfig]);

  const handleModelChange = useCallback((value: ModelSelectorValue) => {
    setModelValue(current => ({
      provider: value.provider,
      model: value.model,
      params: current.params,
    }));
  }, []);

  const handleToggleSkill = useCallback((skillId: string, checked: boolean) => {
    setSelectedSkillIds(current =>
      checked ? Array.from(new Set([...current, skillId])) : current.filter(id => id !== skillId)
    );
  }, []);

  const handleToggleKnowledgeDataset = useCallback((datasetId: string, checked: boolean) => {
    setKnowledgeDatasetIds(current =>
      checked
        ? Array.from(new Set([...current, datasetId]))
        : current.filter(id => id !== datasetId)
    );
  }, []);

  const handleGenerateSuggestedQuestions = useCallback(async () => {
    if (isGeneratingSuggestions) return;
    setIsGeneratingSuggestions(true);
    try {
      const skills = selectedSkills.map(skill => {
        const display = getAIChatSkillDisplayInfo(skill, locale);
        return {
          id: skill.skill_id,
          name: display.label,
          description: display.description || skill.description || '',
        };
      });
      const response = await agentService.generateSuggestedQuestions(agentId, {
        locale,
        count: 6,
        provider: modelValue.provider,
        model: modelValue.model,
        system_prompt: systemPrompt,
        home_title: homeTitle,
        existing_questions: currentPayload.suggested_questions,
        skills,
        knowledge_refs: [],
      });
      const generated = (response.data.questions ?? [])
        .map(item => item.text.trim())
        .filter(Boolean)
        .slice(0, 6);
      if (generated.length === 0) {
        toast.info(t('toasts.noGeneratedSuggestions'));
        return;
      }
      setSuggestedQuestions(generated);
      toast.success(t('toasts.suggestionsGenerated'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('toasts.generateSuggestionsFailed'));
    } finally {
      setIsGeneratingSuggestions(false);
    }
  }, [
    agentId,
    currentPayload.suggested_questions,
    homeTitle,
    isGeneratingSuggestions,
    locale,
    modelValue.model,
    modelValue.provider,
    selectedSkills,
    systemPrompt,
    t,
  ]);

  const handleCopyWebAppUrl = useCallback(async () => {
    const webAppID = agentDetail?.web_app_id;
    if (!webAppID) {
      toast.error(t('toasts.noWebAppUrl'));
      return;
    }
    try {
      await navigator.clipboard.writeText(`${window.location.origin}/webapp/${webAppID}/chat`);
      toast.success(t('toasts.webAppLinkCopied'));
    } catch {
      toast.error(t('toasts.webAppLinkCopyFailed'));
    }
  }, [agentDetail?.web_app_id, t]);

  const handleOpenPublishedVersions = useCallback(async () => {
    setPublishedVersionsOpen(true);
    setIsLoadingVersions(true);
    try {
      const response = await agentService.getPublishedVersions(agentId);
      setPublishedVersions(response.data.data ?? []);
    } catch (error) {
      toast.error(getErrorMessage(error) || t('toasts.loadPublishedVersionsFailed'));
    } finally {
      setIsLoadingVersions(false);
    }
  }, [agentId, t]);

  const handleCancelVersionPreview = useCallback(() => {
    const backup = versionPreviewBackupRef.current;
    if (backup) {
      applyRuntimePayload(backup.payload);
      restoreSnapshot(backup.persistence);
    }
    versionPreviewBackupRef.current = null;
    setSelectedPublishedVersionId('');
    setIsVersionPreviewing(false);
  }, [applyRuntimePayload, restoreSnapshot]);

  const handlePublishedVersionsOpenChange = useCallback(
    (open: boolean) => {
      if (open) {
        void handleOpenPublishedVersions();
        return;
      }
      handleCancelVersionPreview();
      setPublishedVersionsOpen(false);
    },
    [handleCancelVersionPreview, handleOpenPublishedVersions]
  );

  const handleSelectPublishedVersion = useCallback(
    (versionId: string) => {
      const version = publishedVersions.find(item => item.id === versionId);
      if (!version) return;
      if (!versionPreviewBackupRef.current) {
        versionPreviewBackupRef.current = {
          payload: currentPayload,
          persistence: getSnapshot(),
        };
      }
      const nextPayload = payloadFromRuntimeConfig(version.config_snapshot);
      applyRuntimePayload(nextPayload);
      setSelectedPublishedVersionId(version.id);
      setIsVersionPreviewing(true);
      setPreviewing();
    },
    [
      applyRuntimePayload,
      currentPayload,
      getSnapshot,
      payloadFromRuntimeConfig,
      publishedVersions,
      setPreviewing,
    ]
  );

  const handleConfirmVersionRollback = useCallback(async () => {
    if (!selectedPublishedVersionId || isRollingBackVersion) return;
    setIsRollingBackVersion(true);
    try {
      const response = await agentService.rollbackPublishedVersion(agentId, {
        version_id: selectedPublishedVersionId,
      });
      const nextPayload = payloadFromRuntimeConfig(response.data);
      applyRuntimePayload(nextPayload);
      markServerSaved(nextPayload, response.data.updated_at ?? Math.floor(Date.now() / 1000));
      queryClient.setQueryData(AGENT_KEYS.config(agentId), response);
      versionPreviewBackupRef.current = null;
      setIsVersionPreviewing(false);
      setSelectedPublishedVersionId('');
      setPublishedVersionsOpen(false);
      toast.success(t('toasts.rollbackSuccess'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('toasts.rollbackFailed'));
    } finally {
      setIsRollingBackVersion(false);
    }
  }, [
    applyRuntimePayload,
    agentId,
    isRollingBackVersion,
    markServerSaved,
    payloadFromRuntimeConfig,
    queryClient,
    selectedPublishedVersionId,
    t,
  ]);

  const handleApplyOptimizedPrompt = useCallback((payload: { text: string }) => {
    setSystemPrompt(payload.text);
    setPromptOptimizerOpen(false);
  }, []);

  const handleManualSave = useCallback(async () => {
    if (hasAgentMemorySlotErrors) {
      toast.error(t('toasts.fixMemorySlotsBeforeSave'));
      return;
    }
    const saved = await saveNow({ silent: false, force: true });
    if (saved) {
      toast.success(t('toasts.saveSuccess'));
    }
  }, [hasAgentMemorySlotErrors, saveNow, t]);

  const handlePublish = useCallback(async () => {
    if (hasAgentMemorySlotErrors) {
      toast.error(t('toasts.fixMemorySlotsBeforePublish'));
      return;
    }
    const saved = await saveNow({ silent: true, force: true });
    if (saved) {
      publishAgent.mutate({ agentId });
    }
  }, [agentId, hasAgentMemorySlotErrors, publishAgent, saveNow, t]);

  const handleSaveBeforeLeave = useCallback(() => {
    if (hasAgentMemorySlotErrors) {
      toast.error(t('toasts.fixMemorySlotsBeforeSave'));
      return Promise.resolve(false);
    }
    return saveNow({ silent: false, force: true });
  }, [hasAgentMemorySlotErrors, saveNow, t]);

  const leaveGuardNode = useAgentRuntimeLeaveGuard({
    enabled: !isVersionPreviewing,
    hasUnsavedChanges: isDirty,
    isSaving,
    onSave: handleSaveBeforeLeave,
  });

  const webAppUrl = agentDetail?.web_app_id ? `/webapp/${agentDetail.web_app_id}/chat` : '';

  if (isAgentLoading || isConfigLoading) {
    return <AgentRuntimeLoadingState />;
  }

  const renderPreviewPanel = (surfaceMode: 'inline' | 'sheet' = 'inline') => (
    <AgentRuntimePreviewPanel
      controller={chatController}
      modelSelectorValue={modelSelectorValue}
      useMemory={false}
      fileUploadEnabled={fileUploadEnabled}
      suggestions={currentPayload.suggested_questions}
      inputPlaceholder={currentPayload.input_placeholder}
      homeBrand={agentHomeBrand}
      homeTitle={currentPayload.home_title || defaultHomeTitle}
      surfaceMode={surfaceMode}
      onOpenMemoryValues={() => setMemoryValuesOpen(true)}
      onModelChange={handleModelChange}
      onClose={() => setPreviewSheetOpen(false)}
    />
  );

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-background">
      {leaveGuardNode}
      <AgentRuntimeHeader
        agentId={agentId}
        agent={agentDetail}
        saveState={saveState}
        saveText={getSaveText(t, saveState, lastSavedAt)}
        isDirty={isDirty}
        isPublishing={publishAgent.isPending}
        disablePrimaryActions={isVersionPreviewing}
        webAppUrl={webAppUrl}
        versionControl={
          <AgentRuntimeVersionPopover
            open={publishedVersionsOpen}
            isLoading={isLoadingVersions}
            isRollingBack={isRollingBackVersion}
            isPreviewing={isVersionPreviewing}
            versions={publishedVersions}
            selectedVersionId={selectedPublishedVersionId}
            onOpenChange={handlePublishedVersionsOpenChange}
            onSelectVersion={handleSelectPublishedVersion}
            onCancelPreview={() => {
              handleCancelVersionPreview();
              setPublishedVersionsOpen(false);
            }}
            onConfirmRollback={() => void handleConfirmVersionRollback()}
          />
        }
        showPreviewAction
        isPreviewOpen={previewSheetOpen}
        onSave={handleManualSave}
        onPublish={handlePublish}
        onCopyWebAppUrl={handleCopyWebAppUrl}
        onTogglePreview={() => setPreviewSheetOpen(current => !current)}
        onOpenPublishedVersions={handleOpenPublishedVersions}
      />

      <div className="min-h-0 flex-1 overflow-y-auto lg:overflow-hidden">
        <div className="grid min-h-full grid-cols-1 lg:h-full lg:min-h-0 lg:grid-cols-[minmax(320px,1fr)_minmax(360px,1fr)] lg:divide-x 2xl:grid-cols-[minmax(320px,0.95fr)_minmax(320px,0.95fr)_minmax(440px,1.2fr)]">
          <div className="h-[45vh] min-h-[360px] border-b lg:h-full lg:min-h-0 lg:border-b-0">
            <AgentRuntimePromptPanel
              className="h-full"
              systemPrompt={systemPrompt}
              onChangeSystemPrompt={setSystemPrompt}
              onOpenOptimizer={() => setPromptOptimizerOpen(true)}
            />
          </div>
          <div className="min-h-0 lg:h-full">
            <AgentRuntimeOrchestrationPanel
              className="min-h-0 lg:h-full"
              scrollAreaClassName="overflow-visible lg:overflow-hidden"
              scrollViewportClassName="h-auto w-full rounded-[inherit] lg:h-full"
              locale={locale}
              openSections={openSections}
              modelValue={modelValue}
              homeTitle={homeTitle}
              inputPlaceholder={inputPlaceholder}
              selectedSkills={selectedSkills}
              normalizedSelectedSkillIds={normalizedSelectedSkillIds}
              selectableSkillsCount={selectableSkills.length}
              isSkillsLoading={isSkillsLoading}
              isSkillConfigLoading={false}
              isDatasetsLoading={isDatasetsLoading}
              availableDatasets={availableDatasets}
              selectedKnowledgeDatasetIds={knowledgeDatasetIds}
              suggestedQuestions={suggestedQuestions}
              isGeneratingSuggestions={isGeneratingSuggestions}
              systemPrompt={systemPrompt}
              fileUploadEnabled={fileUploadEnabled}
              agentMemoryEnabled={agentMemoryEnabled}
              agentMemorySlots={agentMemorySlots}
              agentMemorySlotValidationErrors={agentMemorySlotValidationErrors}
              defaultHomeTitle={defaultHomeTitle}
              defaultInputPlaceholder={defaultInputPlaceholder}
              onToggleSection={section =>
                setOpenSections(current => ({ ...current, [section]: !current[section] }))
              }
              onChangeModelValue={setModelValue}
              onChangeHomeTitle={setHomeTitle}
              onChangeInputPlaceholder={setInputPlaceholder}
              onOpenSkillDialog={() => setSkillDialogOpen(true)}
              onToggleSkill={handleToggleSkill}
              onToggleKnowledgeDataset={handleToggleKnowledgeDataset}
              onGenerateSuggestedQuestions={() => void handleGenerateSuggestedQuestions()}
              onChangeSuggestedQuestions={setSuggestedQuestions}
              onChangeFileUploadEnabled={setFileUploadEnabled}
              onChangeAgentMemoryEnabled={setAgentMemoryEnabled}
              onChangeAgentMemorySlots={setAgentMemorySlots}
            />
          </div>
        <div className="hidden min-w-0 overflow-hidden 2xl:flex">{renderPreviewPanel()}</div>
        </div>
      </div>

      {!isTwoXlViewport ? (
        <Sheet open={previewSheetOpen} onOpenChange={setPreviewSheetOpen}>
          <SheetContent
            side="right"
            showClose={false}
            className="flex h-full min-h-0 w-[min(720px,100vw)] max-w-none flex-col p-0 sm:max-w-none"
          >
            <SheetTitle className="sr-only">{t('preview.title')}</SheetTitle>
            <SheetDescription className="sr-only">{t('preview.description')}</SheetDescription>
            {renderPreviewPanel('sheet')}
          </SheetContent>
        </Sheet>
      ) : null}

      <PromptOptimizerDialog
        open={promptOptimizerOpen}
        onOpenChange={setPromptOptimizerOpen}
        initialPrompt={systemPrompt}
        sourceLabel={t('prompt.optimizerSourceLabel')}
        sourceHelpText={t('prompt.optimizerSourceHelp')}
        sourceResetLabel={t('prompt.optimizerSourceReset')}
        initialModel={
          modelValue.model
            ? {
                provider: modelValue.provider,
                model: modelValue.model,
              }
            : null
        }
        applyLabel={t('prompt.optimizerApply')}
        onApplyResult={handleApplyOptimizedPrompt}
      />

      <AgentRuntimeSkillDialog
        open={skillDialogOpen}
        locale={locale}
        selectableSkillsCount={selectableSkills.length}
        dialogSkills={dialogSkills}
        normalizedSelectedSkillIds={normalizedSelectedSkillIds}
        skillSearch={skillSearch}
        showSelectedSkillsOnly={showSelectedSkillsOnly}
        onOpenChange={setSkillDialogOpen}
        onChangeSkillSearch={setSkillSearch}
        onChangeShowSelectedSkillsOnly={setShowSelectedSkillsOnly}
        onToggleSkill={handleToggleSkill}
      />

      <AgentRuntimeMemoryValuesDialog
        agentId={agentId}
        open={memoryValuesOpen}
        defaultUserId={profile?.id}
        onOpenChange={setMemoryValuesOpen}
      />
    </div>
  );
}
