'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQueries, useQueryClient } from '@tanstack/react-query';
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
import { useAgent, useAgentConfig, usePublishAgent } from '@/hooks/agent/use-agents';
import { useAIChatSkills } from '@/hooks/aichat/use-aichat-skills';
import { useDatasets } from '@/hooks/dataset/use-datasets';
import { useMediaQuery } from '@/hooks/use-media-query';
import { AGENT_KEYS, DATASET_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useAutoProfile } from '@/hooks/use-profile';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import { datasetService } from '@/services';
import { getTemplateAwareCharacterCount } from '@/components/workflow/common/workflow-value-editor/utils/value-transform';
import type {
  AgentDatabaseBinding,
  AgentMemorySlotConfig,
  AgentRuntimeConfig,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { Dataset } from '@/services/types/dataset';
import { getErrorMessage } from '@/utils/error-notifications';
import type { AgentConfigSection, AgentPublishedVersionListItem } from '../types';
import { toModelParams, validateAgentMemorySlots } from '../utils';
import { useAgentRuntimeDraftPersistence } from '../use-agent-runtime-draft-persistence';
import { useAgentRuntimeLeaveGuard } from '../use-agent-runtime-leave-guard';
import { AgentHomeBrand, getAgentRuntimeSaveText, type VersionPreviewBackup } from './page-model-utils';
import { AGENT_SYSTEM_PROMPT_MAX_LENGTH } from '../prompt-limits';

type AgentKnowledgeDataset = Dataset & { load_error?: boolean };

function createAgentKnowledgeDatasetFallback(
  id: string,
  name: string,
  description: string,
  loadError: boolean
): AgentKnowledgeDataset {
  return {
    id,
    name,
    description,
    provider: '',
    data_source_type: '',
    indexing_technique: '',
    word_count: 0,
    created_by: '',
    created_at: '',
    updated_by: null,
    updated_at: '',
    embedding_model: '',
    embedding_model_provider: '',
    embedding_available: false,
    retrieval_config: {
      search_method: 'semantic_search',
      reranking_enable: false,
      top_k: 0,
      score_threshold_enabled: false,
      score_threshold: 0,
    },
    tags: null,
    icon: '',
    icon_type: 'text',
    icon_background: '',
    app_count: 0,
    document_count: 0,
    available_document_count: 0,
    available_segment_count: 0,
    collection_binding_id: null,
    owner: null,
    owner_account: null,
    is_editor: false,
    can_edit: false,
    load_error: loadError,
  };
}

function normalizeAgentDatabaseBindings(bindings: AgentDatabaseBinding[]): AgentDatabaseBinding[] {
  const byDataSource = new Map<string, { readable: Set<string>; writable: Set<string> }>();
  bindings.forEach(binding => {
    const dataSourceId = binding.data_source_id.trim();
    if (!dataSourceId) return;
    const tableIds = binding.table_ids.map(id => id.trim()).filter(Boolean);
    if (tableIds.length === 0) return;
    const existing = byDataSource.get(dataSourceId) ?? {
      readable: new Set<string>(),
      writable: new Set<string>(),
    };
    tableIds.forEach(id => existing.readable.add(id));
    (binding.writable_table_ids ?? [])
      .map(id => id.trim())
      .filter(Boolean)
      .forEach(id => {
        if (tableIds.includes(id)) {
          existing.writable.add(id);
        }
      });
    byDataSource.set(dataSourceId, existing);
  });
  return Array.from(byDataSource.entries())
    .map(([dataSourceId, tables]) => ({
      data_source_id: dataSourceId,
      table_ids: Array.from(tables.readable).sort(),
      writable_table_ids: Array.from(tables.writable).filter(id => tables.readable.has(id)).sort(),
    }))
    .sort((left, right) => left.data_source_id.localeCompare(right.data_source_id));
}

export function useAgentRuntimePageModel(agentId: string) {
  const queryClient = useQueryClient();
  const { locale } = useLocale();
  const t = useT('agents.agentRuntime');
  const { agent, isLoading: isAgentLoading } = useAgent(agentId);
  const { data: profile } = useAutoProfile({ staleTime: 1_800_000 });
  const { data: configResponse, isLoading: isConfigLoading } = useAgentConfig(agentId);
  const { data: allSkills = [], isLoading: isSkillsLoading } = useAIChatSkills();
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
  const [databaseBindings, setDatabaseBindings] = useState<AgentDatabaseBinding[]>([]);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [knowledgeDialogOpen, setKnowledgeDialogOpen] = useState(false);
  const [promptOptimizerOpen, setPromptOptimizerOpen] = useState(false);
  const [memoryValuesOpen, setMemoryValuesOpen] = useState(false);
  const [skillSearch, setSkillSearch] = useState('');
  const [showSelectedSkillsOnly, setShowSelectedSkillsOnly] = useState(false);
  const [knowledgeSearch, setKnowledgeSearch] = useState('');
  const [showSelectedKnowledgeOnly, setShowSelectedKnowledgeOnly] = useState(false);
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
    databases: true,
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
  const selectedDatasetQueries = useQueries({
    queries: knowledgeDatasetIds.map(datasetId => ({
      queryKey: DATASET_KEYS.detail(datasetId),
      queryFn: () => datasetService.getDataset(datasetId),
      enabled: Boolean(datasetId),
      staleTime: 5 * 60 * 1000,
      retry: false,
    })),
  });
  const { pages: knowledgeDialogPages, isLoading: isKnowledgeDialogDatasetsLoading } = useDatasets(
    { keyword: knowledgeSearch.trim(), limit: 50 },
    { enabled: knowledgeDialogOpen }
  );
  const selectedKnowledgeDatasets = useMemo(() => {
    const byID = new Map<string, AgentKnowledgeDataset>();
    selectedDatasetQueries.forEach(query => {
      const dataset = query.data?.data;
      if (dataset?.id) {
        byID.set(dataset.id, dataset);
      }
    });
    knowledgeDialogPages.flat().forEach(dataset => {
      if (knowledgeDatasetIds.includes(dataset.id)) {
        byID.set(dataset.id, dataset);
      }
    });
    return knowledgeDatasetIds.map((id, index) => {
      const dataset = byID.get(id);
      if (dataset) return dataset;
      const query = selectedDatasetQueries[index];
      const hasLoadError = Boolean(query?.isError);
      return createAgentKnowledgeDatasetFallback(
        id,
        t('knowledge.loadFailedName'),
        hasLoadError ? t('knowledge.loadFailedDescription') : '',
        hasLoadError
      );
    });
  }, [knowledgeDatasetIds, knowledgeDialogPages, selectedDatasetQueries, t]);
  const isSelectedDatasetsLoading = selectedDatasetQueries.some(query => query.isLoading);
  const knowledgeDialogDatasets = useMemo(() => {
    const byID = new Map<string, Dataset>();
    selectedKnowledgeDatasets.forEach(dataset => byID.set(dataset.id, dataset));
    knowledgeDialogPages.flat().forEach(dataset => byID.set(dataset.id, dataset));
    return Array.from(byID.values())
      .filter(dataset => !showSelectedKnowledgeOnly || knowledgeDatasetIds.includes(dataset.id))
      .sort((left, right) => {
        const leftChecked = knowledgeDatasetIds.includes(left.id);
        const rightChecked = knowledgeDatasetIds.includes(right.id);
        if (leftChecked !== rightChecked) return leftChecked ? -1 : 1;
        return left.name.localeCompare(right.name, locale);
      });
  }, [
    knowledgeDatasetIds,
    knowledgeDialogPages,
    locale,
    selectedKnowledgeDatasets,
    showSelectedKnowledgeOnly,
  ]);
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
  const systemPromptEffectiveLength = useMemo(
    () => getTemplateAwareCharacterCount(systemPrompt, { templateBlocksEnabled: true }),
    [systemPrompt]
  );
  const isSystemPromptTooLong = systemPromptEffectiveLength > AGENT_SYSTEM_PROMPT_MAX_LENGTH;

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
      database_bindings: databaseBindings,
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
      databaseBindings,
      systemPrompt,
    ]
  );
  const agentHomeBrand = useMemo(
    () => (
      <AgentHomeBrand
        iconType={agentDetail?.icon_type}
        iconUrl={agentDetail?.icon_url}
        name={agentDetail?.name}
      />
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
    setDatabaseBindings(normalizeAgentDatabaseBindings(payload.database_bindings ?? []));
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
      database_bindings: normalizeAgentDatabaseBindings(runtimeConfig.database_bindings ?? []),
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
    canSave: () => !hasAgentMemorySlotErrors && !isSystemPromptTooLong,
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
    if (isSystemPromptTooLong) {
      toast.error(
        t('toasts.systemPromptTooLongBeforeSave', { limit: AGENT_SYSTEM_PROMPT_MAX_LENGTH })
      );
      return;
    }
    const saved = await saveNow({ silent: false, force: true });
    if (saved) {
      toast.success(t('toasts.saveSuccess'));
    }
  }, [hasAgentMemorySlotErrors, isSystemPromptTooLong, saveNow, t]);

  const handlePublish = useCallback(async () => {
    if (hasAgentMemorySlotErrors) {
      toast.error(t('toasts.fixMemorySlotsBeforePublish'));
      return;
    }
    if (isSystemPromptTooLong) {
      toast.error(
        t('toasts.systemPromptTooLongBeforePublish', { limit: AGENT_SYSTEM_PROMPT_MAX_LENGTH })
      );
      return;
    }
    const saved = await saveNow({ silent: true, force: true });
    if (saved) {
      publishAgent.mutate({ agentId });
    }
  }, [agentId, hasAgentMemorySlotErrors, isSystemPromptTooLong, publishAgent, saveNow, t]);

  const handleSaveBeforeLeave = useCallback(() => {
    if (hasAgentMemorySlotErrors) {
      toast.error(t('toasts.fixMemorySlotsBeforeSave'));
      return Promise.resolve(false);
    }
    if (isSystemPromptTooLong) {
      toast.error(
        t('toasts.systemPromptTooLongBeforeSave', { limit: AGENT_SYSTEM_PROMPT_MAX_LENGTH })
      );
      return Promise.resolve(false);
    }
    return saveNow({ silent: false, force: true });
  }, [hasAgentMemorySlotErrors, isSystemPromptTooLong, saveNow, t]);

  const leaveGuardNode = useAgentRuntimeLeaveGuard({
    enabled: !isVersionPreviewing,
    hasUnsavedChanges: isDirty,
    isSaving,
    onSave: handleSaveBeforeLeave,
  });

  const webAppUrl = agentDetail?.web_app_id ? `/webapp/${agentDetail.web_app_id}/chat` : '';

  return {
    agentId,
    locale,
    t,
    isLoading: isAgentLoading || isConfigLoading,
    leaveGuardNode,
    isTwoXlViewport,
    previewSheetOpen,
    setPreviewSheetOpen,
    header: {
      agentId,
      agent: agentDetail,
      saveState,
      saveText: getAgentRuntimeSaveText(t, saveState, lastSavedAt),
      isDirty,
      isPublishing: publishAgent.isPending,
      disablePrimaryActions: isVersionPreviewing,
      webAppUrl,
      showPreviewAction: true,
      isPreviewOpen: previewSheetOpen,
      onSave: handleManualSave,
      onPublish: handlePublish,
      onCopyWebAppUrl: handleCopyWebAppUrl,
      onTogglePreview: () => setPreviewSheetOpen(current => !current),
      onOpenPublishedVersions: handleOpenPublishedVersions,
    },
    version: {
      open: publishedVersionsOpen,
      isLoading: isLoadingVersions,
      isRollingBack: isRollingBackVersion,
      isPreviewing: isVersionPreviewing,
      versions: publishedVersions,
      selectedVersionId: selectedPublishedVersionId,
      onOpenChange: handlePublishedVersionsOpenChange,
      onSelectVersion: handleSelectPublishedVersion,
      onCancelPreview: () => {
        handleCancelVersionPreview();
        setPublishedVersionsOpen(false);
      },
      onConfirmRollback: () => void handleConfirmVersionRollback(),
    },
    prompt: {
      systemPrompt,
      selectedKnowledgeDatasets,
      selectedSkills,
      onChangeSystemPrompt: setSystemPrompt,
      onOpenOptimizer: () => setPromptOptimizerOpen(true),
    },
    orchestration: {
      locale,
      openSections,
      modelValue,
      homeTitle,
      inputPlaceholder,
      selectedSkills,
      normalizedSelectedSkillIds,
      selectableSkillsCount: selectableSkills.length,
      isSkillsLoading,
      isSkillConfigLoading: false,
      isDatasetsLoading: isSelectedDatasetsLoading,
      selectedKnowledgeDatasets,
      selectedKnowledgeDatasetIds: knowledgeDatasetIds,
      databaseBindings,
      suggestedQuestions,
      isGeneratingSuggestions,
      systemPrompt,
      fileUploadEnabled,
      agentMemoryEnabled,
      agentMemorySlots,
      agentMemorySlotValidationErrors,
      defaultHomeTitle,
      defaultInputPlaceholder,
      onToggleSection: (section: AgentConfigSection) =>
        setOpenSections(current => ({ ...current, [section]: !current[section] })),
      onChangeModelValue: setModelValue,
      onChangeHomeTitle: setHomeTitle,
      onChangeInputPlaceholder: setInputPlaceholder,
      onOpenSkillDialog: () => setSkillDialogOpen(true),
      onOpenKnowledgeDialog: () => setKnowledgeDialogOpen(true),
      onToggleSkill: handleToggleSkill,
      onToggleKnowledgeDataset: handleToggleKnowledgeDataset,
      onChangeDatabaseBindings: setDatabaseBindings,
      onGenerateSuggestedQuestions: () => void handleGenerateSuggestedQuestions(),
      onChangeSuggestedQuestions: setSuggestedQuestions,
      onChangeFileUploadEnabled: setFileUploadEnabled,
      onChangeAgentMemoryEnabled: setAgentMemoryEnabled,
      onChangeAgentMemorySlots: setAgentMemorySlots,
    },
    preview: {
      controller: chatController,
      modelSelectorValue,
      useMemory: false,
      fileUploadEnabled,
      suggestions: currentPayload.suggested_questions,
      inputPlaceholder: currentPayload.input_placeholder,
      homeBrand: agentHomeBrand,
      homeTitle: currentPayload.home_title || defaultHomeTitle,
      onOpenMemoryValues: () => setMemoryValuesOpen(true),
      onModelChange: handleModelChange,
      onClose: () => setPreviewSheetOpen(false),
    },
    dialogs: {
      promptOptimizer: {
        open: promptOptimizerOpen,
        onOpenChange: setPromptOptimizerOpen,
        initialPrompt: systemPrompt,
        sourceLabel: t('prompt.optimizerSourceLabel'),
        sourceHelpText: t('prompt.optimizerSourceHelp'),
        sourceResetLabel: t('prompt.optimizerSourceReset'),
        initialModel: modelValue.model
          ? {
              provider: modelValue.provider,
              model: modelValue.model,
            }
          : null,
        applyLabel: t('prompt.optimizerApply'),
        onApplyResult: handleApplyOptimizedPrompt,
      },
      skill: {
        open: skillDialogOpen,
        locale,
        selectableSkillsCount: selectableSkills.length,
        dialogSkills,
        normalizedSelectedSkillIds,
        skillSearch,
        showSelectedSkillsOnly,
        onOpenChange: setSkillDialogOpen,
        onChangeSkillSearch: setSkillSearch,
        onChangeShowSelectedSkillsOnly: setShowSelectedSkillsOnly,
        onToggleSkill: handleToggleSkill,
      },
      knowledge: {
        open: knowledgeDialogOpen,
        datasets: knowledgeDialogDatasets,
        selectedDatasetIds: knowledgeDatasetIds,
        search: knowledgeSearch,
        showSelectedOnly: showSelectedKnowledgeOnly,
        isLoading: isKnowledgeDialogDatasetsLoading || isSelectedDatasetsLoading,
        onOpenChange: setKnowledgeDialogOpen,
        onChangeSearch: setKnowledgeSearch,
        onChangeShowSelectedOnly: setShowSelectedKnowledgeOnly,
        onToggleDataset: handleToggleKnowledgeDataset,
      },
      memoryValues: {
        agentId,
        open: memoryValuesOpen,
        defaultUserId: profile?.id,
        onOpenChange: setMemoryValuesOpen,
      },
    },
  };
}

export type AgentRuntimePageModel = ReturnType<typeof useAgentRuntimePageModel>;
