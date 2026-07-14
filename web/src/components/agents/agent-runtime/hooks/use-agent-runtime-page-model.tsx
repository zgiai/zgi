'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createAgentDraftTransport, useAIChatController } from '@/components/chat';
import { buildOpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import { findAIChatModelProps } from '@/components/chat/variants/aichat/model-props';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import type {
  ModelSelectorModelProps,
  ModelSelectorParameterValue,
  ModelSelectorValue,
} from '@/components/common/model-selector';
import { useAgent, useAgentConfig, usePublishAgent } from '@/hooks/agent/use-agents';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { AGENT_KEYS, DATASET_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useAutoProfile } from '@/hooks/use-profile';
import { useT } from '@/i18n';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import {
  AGENT_PERMISSION_ACTIONS,
  KNOWLEDGE_BASE_READ_PERMISSION_CODES,
} from '@/constants/permissions';
import agentService from '@/services/agent.service';
import { datasetService } from '@/services';
import { getTemplateAwareCharacterCount } from '@/components/workflow/common/workflow-value-editor/utils/value-transform';
import type { OpeningStatementDialogValue } from '@/components/workflow/ui/features-panel/opening-statement-dialog';
import type {
  AgentDetail,
  AgentDatabaseBinding,
  AgentMemorySlotConfig,
  AgentRuntimeConfig,
  AgentSkillBindingCandidate,
  AgentWorkflowBinding,
  AgentWorkflowBindingCandidate,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { Dataset } from '@/services/types/dataset';
import { getErrorMessage } from '@/utils/error-notifications';
import type { AgentConfigSection, AgentPublishedVersionListItem } from '../types';
import { buildAgentRuntimeSignature, toModelParams, validateAgentMemorySlots } from '../utils';
import { useAgentRuntimeDraftPersistence } from '../use-agent-runtime-draft-persistence';
import { useAgentRuntimeLeaveGuard } from '../use-agent-runtime-leave-guard';
import { getAgentRuntimeSaveText, type VersionPreviewBackup } from './page-model-utils';
import { AGENT_SYSTEM_PROMPT_MAX_LENGTH } from '../prompt-limits';
import { buildAgentRuntimeAIChatContext } from '../aichat-context';
import { normalizeAgentDatabaseBindings } from '../database-binding-draft';

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
      search_method: 'hybrid_search',
      reranking_enable: true,
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

function normalizeAgentWorkflowBindings(bindings: AgentWorkflowBinding[]): AgentWorkflowBinding[] {
  const byBindingID = new Map<string, AgentWorkflowBinding>();
  bindings.forEach(binding => {
    const bindingId = binding.binding_id.trim();
    const agentId = binding.agent_id.trim();
    const workflowId = binding.workflow_id.trim();
    if (!bindingId || !agentId || !workflowId) return;
    const versionStrategy = binding.version_strategy || 'latest_published';
    if (versionStrategy !== 'latest_published' && versionStrategy !== 'pinned') return;
    byBindingID.set(bindingId, {
      binding_id: bindingId,
      label: binding.label.trim(),
      description: binding.description?.trim() || undefined,
      agent_id: agentId,
      workflow_id: workflowId,
      agent_type: binding.agent_type,
      version_strategy: versionStrategy,
      version_uuid:
        versionStrategy === 'pinned' ? binding.version_uuid?.trim() || undefined : undefined,
      timeout_seconds: Math.max(0, binding.timeout_seconds ?? 0),
    });
  });
  return Array.from(byBindingID.values()).sort((left, right) =>
    left.binding_id.localeCompare(right.binding_id)
  );
}

function candidateToSkillMetadata(candidate: AgentSkillBindingCandidate): AIChatSkillMetadata {
  return {
    skill_id: candidate.skill_id,
    source: candidate.source === 'custom' ? 'custom' : 'system',
    name: candidate.name,
    description: candidate.description ?? '',
    when_to_use: candidate.when_to_use ?? '',
    runtime_type: (candidate.runtime_type || 'prompt') as AIChatSkillMetadata['runtime_type'],
    enabled: true,
    display: candidate.display,
    has_tools: candidate.has_tools,
    has_references: candidate.has_references,
    has_scripts: candidate.has_scripts,
    scripts_supported: candidate.scripts_supported,
    max_calls_per_turn: 0,
    timeout_seconds: 0,
    required_config: candidate.required_config,
  };
}

function compactSuggestedQuestionContextRef(value: string, maxLength = 180): string {
  const text = value.replace(/\s+/g, ' ').trim();
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength).trim()}...`;
}

function agentDetailWorkspaceID(agent: AgentDetail | undefined): string {
  return agent?.workspace?.id || agent?.workspace_id || agent?.tenant?.id || agent?.tenant_id || '';
}

function buildSuggestedQuestionContextRefs(params: {
  selectedKnowledgeDatasets: AgentKnowledgeDataset[];
  databaseBindings: AgentDatabaseBinding[];
  workflowBindings: AgentWorkflowBinding[];
  workflowCandidatesByBindingID: Map<string, AgentWorkflowBindingCandidate>;
  fileUploadEnabled: boolean;
  agentMemoryEnabled: boolean;
}): string[] {
  const refs: string[] = [];
  params.selectedKnowledgeDatasets.forEach(dataset => {
    const name = dataset.name?.trim();
    if (!name) return;
    refs.push(
      compactSuggestedQuestionContextRef(
        `Knowledge: ${name}${dataset.description ? ` - ${dataset.description}` : ''}`
      )
    );
  });
  params.databaseBindings.forEach(binding => {
    const dataSourceID = binding.data_source_id.trim();
    if (!dataSourceID || binding.table_ids.length === 0) return;
    refs.push(
      compactSuggestedQuestionContextRef(
        `Database: ${dataSourceID} (${binding.table_ids.length} selected tables${
          (binding.writable_table_ids ?? []).length > 0 ? ', writable tables enabled' : ''
        })`
      )
    );
  });
  params.workflowBindings.forEach(binding => {
    const candidate = params.workflowCandidatesByBindingID.get(binding.binding_id);
    const label = binding.label || candidate?.label || binding.binding_id;
    const description = binding.description || candidate?.description || '';
    if (!label.trim()) return;
    refs.push(
      compactSuggestedQuestionContextRef(
        `Workflow: ${label}${description ? ` - ${description}` : ''}`
      )
    );
  });
  if (params.fileUploadEnabled) {
    refs.push('File upload: enabled');
  }
  if (params.agentMemoryEnabled) {
    refs.push('Memory: enabled');
  }
  return Array.from(new Set(refs)).slice(0, 12);
}

export function useAgentRuntimePageModel(agentId: string) {
  const queryClient = useQueryClient();
  const { locale } = useLocale();
  const t = useT('agents.agentRuntime');
  const tRoot = useT();
  const {
    permissions,
    organizationRole,
    workspaceRole,
    workspaceRoleName,
    hasAnyPermission,
    isLoading: isPermissionsLoading,
  } = useAccountPermissions();
  const canCreateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.create);
  const canImportAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.import);
  const canUpdateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.update);
  const canConfigureAgentRuntime = hasAnyPermission(AGENT_PERMISSION_ACTIONS.runtimeConfigManage);
  const canPublishAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.publish);
  const canManageAgentRuntimeAccess = hasAnyPermission(
    AGENT_PERMISSION_ACTIONS.runtimeAccessManage
  );
  const canManageAgent =
    canUpdateAgent || canConfigureAgentRuntime || canPublishAgent || canManageAgentRuntimeAccess;
  const canOpenAgentRuntimeEditor =
    canCreateAgent ||
    canImportAgent ||
    canUpdateAgent ||
    canConfigureAgentRuntime ||
    canPublishAgent ||
    canManageAgentRuntimeAccess;
  const { agent, isLoading: isAgentLoading } = useAgent(agentId, canOpenAgentRuntimeEditor);
  const canBindKnowledge = hasAnyPermission(KNOWLEDGE_BASE_READ_PERMISSION_CODES);
  const { data: profile } = useAutoProfile({ staleTime: 1_800_000 });
  const { data: configResponse, isLoading: isConfigLoading } = useAgentConfig(
    agentId,
    canOpenAgentRuntimeEditor
  );
  const {
    data: skillCandidatesResponse,
    isLoading: isSkillCandidatesLoading,
    isFetching: isSkillCandidatesFetching,
  } = useQuery({
    queryKey: AGENT_KEYS.skillBindingCandidates(agentId),
    queryFn: () => agentService.getAgentSkillBindingCandidates(agentId, { page: 1, limit: 100 }),
    enabled: Boolean(agentId) && canConfigureAgentRuntime,
    staleTime: 60_000,
  });
  const { data: workflowCandidatesResponse, isLoading: isWorkflowCandidatesLoading } = useQuery({
    queryKey: AGENT_KEYS.workflowBindingCandidates(agentId),
    queryFn: () => agentService.getAgentWorkflowBindingCandidates(agentId),
    enabled: Boolean(agentId) && canConfigureAgentRuntime,
    staleTime: 60_000,
  });
  const publishAgent = usePublishAgent();
  const config = configResponse?.data;
  const workflowCandidates = useMemo<AgentWorkflowBindingCandidate[]>(
    () => (workflowCandidatesResponse?.data.data ?? []) as AgentWorkflowBindingCandidate[],
    [workflowCandidatesResponse?.data.data]
  );
  const { models: availableChatModels } = useAvailableModels({ use_case: 'text-chat' });
  const agentDetail = agent?.data;
  const agentWorkspaceId = agentDetailWorkspaceID(agentDetail);
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
  const [openingStatement, setOpeningStatement] = useState('');
  const [inputPlaceholder, setInputPlaceholder] = useState(defaultInputPlaceholder);
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const [knowledgeDatasetIds, setKnowledgeDatasetIds] = useState<string[]>([]);
  const [databaseBindings, setDatabaseBindings] = useState<AgentDatabaseBinding[]>([]);
  const [workflowBindings, setWorkflowBindings] = useState<AgentWorkflowBinding[]>([]);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [knowledgeDialogOpen, setKnowledgeDialogOpen] = useState(false);
  const [workflowDialogOpen, setWorkflowDialogOpen] = useState(false);
  const [promptOptimizerOpen, setPromptOptimizerOpen] = useState(false);
  const [memoryValuesOpen, setMemoryValuesOpen] = useState(false);
  const [isGeneratingSuggestions, setIsGeneratingSuggestions] = useState(false);
  const [publishedVersionsOpen, setPublishedVersionsOpen] = useState(false);
  const [previewSheetOpen, setPreviewSheetOpen] = useState(false);
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
    workflows: true,
    files: true,
    memory: true,
  });
  const hydratedAgentIdRef = useRef<string | null>(null);
  const hydratedConfigSignatureRef = useRef<string | null>(null);
  const versionPreviewBackupRef = useRef<VersionPreviewBackup | null>(null);

  const selectableSkills = useMemo(() => {
    return (skillCandidatesResponse?.data.data ?? []).map(candidateToSkillMetadata);
  }, [skillCandidatesResponse?.data.data]);
  const selectedDatasetQueries = useQueries({
    queries: knowledgeDatasetIds.map(datasetId => ({
      queryKey: DATASET_KEYS.detail(datasetId),
      queryFn: () => datasetService.getDataset(datasetId),
      enabled: Boolean(datasetId) && canBindKnowledge,
      staleTime: 5 * 60 * 1000,
      retry: false,
    })),
  });
  const selectedKnowledgeDatasets = useMemo(() => {
    const byID = new Map<string, AgentKnowledgeDataset>();
    selectedDatasetQueries.forEach(query => {
      const dataset = query.data?.data;
      if (dataset?.id) {
        byID.set(dataset.id, dataset);
      }
    });
    return knowledgeDatasetIds.map((id, index) => {
      const dataset = byID.get(id);
      if (dataset) return dataset;
      const query = selectedDatasetQueries[index];
      const hasLoadError = !canBindKnowledge || Boolean(query?.isError);
      return createAgentKnowledgeDatasetFallback(
        id,
        t('knowledge.loadFailedName'),
        !canBindKnowledge
          ? t('knowledge.bindingPermissionRequired')
          : hasLoadError
            ? t('knowledge.loadFailedDescription')
            : '',
        hasLoadError
      );
    });
  }, [canBindKnowledge, knowledgeDatasetIds, selectedDatasetQueries, t]);
  const isSelectedDatasetsLoading = selectedDatasetQueries.some(query => query.isLoading);
  const workflowCandidatesByBindingID = useMemo<Map<string, AgentWorkflowBindingCandidate>>(
    () => new Map(workflowCandidates.map(candidate => [candidate.binding_id, candidate])),
    [workflowCandidates]
  );
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
  const selectedModelProps = useMemo<ModelSelectorModelProps | null>(
    () => findAIChatModelProps(availableChatModels, modelSelectorValue),
    [availableChatModels, modelSelectorValue]
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
      opening_statement: openingStatement,
      input_placeholder: inputPlaceholder.trim() || defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: suggestedQuestions
        .map(item => item.trim())
        .filter(Boolean)
        .slice(0, SUGGESTED_QUESTIONS_LIMIT),
      knowledge_dataset_ids: knowledgeDatasetIds,
      knowledge_retrieval_config: {},
      database_bindings: databaseBindings,
      workflow_bindings: workflowBindings,
    }),
    [
      defaultHomeTitle,
      defaultInputPlaceholder,
      fileUploadEnabled,
      agentMemoryEnabled,
      agentMemorySlots,
      homeTitle,
      openingStatement,
      inputPlaceholder,
      modelValue,
      normalizedSelectedSkillIds,
      suggestedQuestions,
      knowledgeDatasetIds,
      databaseBindings,
      workflowBindings,
      systemPrompt,
    ]
  );
  const agentOpeningGuideBrand = useMemo(
    () =>
      buildOpeningGuideBrand({
        title: agentDetail?.name,
        iconType: agentDetail?.icon_type,
        icon: agentDetail?.icon,
        iconUrl: agentDetail?.icon_url,
      }),
    [agentDetail?.icon, agentDetail?.icon_type, agentDetail?.icon_url, agentDetail?.name]
  );

  useEffect(() => {
    initChatController(null);
  }, [initChatController]);

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
    setOpeningStatement(payload.opening_statement ?? '');
    setInputPlaceholder(payload.input_placeholder);
    setSuggestedQuestions(payload.suggested_questions);
    setKnowledgeDatasetIds(payload.knowledge_dataset_ids ?? []);
    setDatabaseBindings(normalizeAgentDatabaseBindings(payload.database_bindings ?? []));
    setWorkflowBindings(normalizeAgentWorkflowBindings(payload.workflow_bindings ?? []));
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
      opening_statement: runtimeConfig.opening_statement ?? '',
      input_placeholder: runtimeConfig.input_placeholder ?? defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: runtimeConfig.suggested_questions ?? [],
      knowledge_dataset_ids: runtimeConfig.knowledge_dataset_ids ?? [],
      knowledge_retrieval_config: runtimeConfig.knowledge_retrieval_config ?? {},
      database_bindings: normalizeAgentDatabaseBindings(runtimeConfig.database_bindings ?? []),
      workflow_bindings: normalizeAgentWorkflowBindings(runtimeConfig.workflow_bindings ?? []),
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
    canSave: () => canConfigureAgentRuntime && !hasAgentMemorySlotErrors && !isSystemPromptTooLong,
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

    const nextPayload = payloadFromRuntimeConfig(config);
    const nextSignature = `${agentId}:${config.updated_at ?? ''}:${buildAgentRuntimeSignature(nextPayload)}`;
    if (hydratedConfigSignatureRef.current === nextSignature) return;

    const isFirstHydrationForAgent = hydratedAgentIdRef.current !== agentId;
    const canApplyServerConfig =
      isFirstHydrationForAgent || (!isDirty && saveState !== 'saving' && !isVersionPreviewing);
    if (!canApplyServerConfig) return;

    applyRuntimePayload(nextPayload);
    hydratedAgentIdRef.current = agentId;
    hydratedConfigSignatureRef.current = nextSignature;
    markHydrated(nextPayload, config.updated_at ?? null);
  }, [
    agentDetail,
    agentId,
    applyRuntimePayload,
    config,
    isDirty,
    isVersionPreviewing,
    markHydrated,
    payloadFromRuntimeConfig,
    saveState,
  ]);

  useEffect(() => {
    if (!workflowCandidatesResponse) return;
    setWorkflowBindings(current => {
      const pruned = current.filter(binding =>
        workflowCandidatesByBindingID.has(binding.binding_id)
      );
      if (pruned.length === current.length) return current;
      return pruned;
    });
  }, [workflowCandidatesByBindingID, workflowCandidatesResponse]);

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

  const handleConfirmSkills = useCallback((skillIds: string[]) => {
    setSelectedSkillIds(Array.from(new Set(skillIds)));
  }, []);

  const handleConfirmKnowledgeDatasets = useCallback((datasetIds: string[]) => {
    setKnowledgeDatasetIds(Array.from(new Set(datasetIds)));
  }, []);

  const handleConfirmWorkflows = useCallback((bindings: AgentWorkflowBinding[]) => {
    setWorkflowBindings(normalizeAgentWorkflowBindings(bindings));
  }, []);

  const handleGenerateSuggestedQuestions = useCallback(
    async (value: OpeningStatementDialogValue) => {
      if (isGeneratingSuggestions) return undefined;
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
          count: SUGGESTED_QUESTIONS_LIMIT,
          provider: modelValue.provider,
          model: modelValue.model,
          system_prompt: systemPrompt,
          home_title: value.title,
          opening_statement: value.message,
          existing_questions: value.suggestedQuestions,
          skills,
          knowledge_refs: buildSuggestedQuestionContextRefs({
            selectedKnowledgeDatasets,
            databaseBindings,
            workflowBindings,
            workflowCandidatesByBindingID,
            fileUploadEnabled,
            agentMemoryEnabled,
          }),
        });
        const generated = (response.data.questions ?? [])
          .map(item => item.text.trim())
          .filter(Boolean)
          .slice(0, SUGGESTED_QUESTIONS_LIMIT);
        if (generated.length === 0) {
          toast.info(t('toasts.noGeneratedSuggestions'));
          return undefined;
        }
        toast.success(t('toasts.suggestionsGenerated'));
        return { questions: generated };
      } catch (error) {
        toast.error(getErrorMessage(error) || t('toasts.generateSuggestionsFailed'));
        return undefined;
      } finally {
        setIsGeneratingSuggestions(false);
      }
    },
    [
      agentId,
      databaseBindings,
      fileUploadEnabled,
      agentMemoryEnabled,
      isGeneratingSuggestions,
      locale,
      modelValue.model,
      modelValue.provider,
      selectedSkills,
      selectedKnowledgeDatasets,
      systemPrompt,
      t,
      workflowBindings,
      workflowCandidatesByBindingID,
    ]
  );

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
    if (!canPublishAgent) {
      toast.error(tRoot('common.unauthorizedDescription'));
      return;
    }
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
    canPublishAgent,
    isRollingBackVersion,
    markServerSaved,
    payloadFromRuntimeConfig,
    queryClient,
    selectedPublishedVersionId,
    t,
    tRoot,
  ]);

  const handleApplyOptimizedPrompt = useCallback((payload: { text: string }) => {
    setSystemPrompt(payload.text);
    setPromptOptimizerOpen(false);
  }, []);

  const handleManualSave = useCallback(async () => {
    if (!canConfigureAgentRuntime) {
      toast.error(tRoot('common.unauthorizedDescription'));
      return;
    }
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
  }, [
    canConfigureAgentRuntime,
    hasAgentMemorySlotErrors,
    isSystemPromptTooLong,
    saveNow,
    t,
    tRoot,
  ]);

  const handlePublish = useCallback(async () => {
    if (!canPublishAgent) {
      toast.error(tRoot('common.unauthorizedDescription'));
      return;
    }
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
    const canPublishCurrentDraft =
      !canConfigureAgentRuntime || (await saveNow({ silent: true, force: true }));
    if (canPublishCurrentDraft) {
      try {
        await publishAgent.mutateAsync({ agentId, silent: false });
      } catch {
        // The mutation hook owns user-facing error feedback.
      }
    }
  }, [
    agentId,
    canPublishAgent,
    canConfigureAgentRuntime,
    hasAgentMemorySlotErrors,
    isSystemPromptTooLong,
    publishAgent,
    saveNow,
    t,
    tRoot,
  ]);

  const handleSaveBeforeLeave = useCallback(() => {
    if (!canConfigureAgentRuntime) {
      toast.error(tRoot('common.unauthorizedDescription'));
      return Promise.resolve(false);
    }
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
  }, [
    canConfigureAgentRuntime,
    hasAgentMemorySlotErrors,
    isSystemPromptTooLong,
    saveNow,
    t,
    tRoot,
  ]);

  const handlePreviewSheetOpenChange = useCallback(
    async (open: boolean) => {
      if (!open) {
        setPreviewSheetOpen(false);
        return;
      }
      if (!canConfigureAgentRuntime || isVersionPreviewing) {
        setPreviewSheetOpen(true);
        return;
      }
      if (hasAgentMemorySlotErrors || isSystemPromptTooLong) {
        setPreviewSheetOpen(false);
        return;
      }
      const saved = await saveNow({ silent: true, force: true });
      if (saved) {
        setPreviewSheetOpen(true);
      }
    },
    [
      canConfigureAgentRuntime,
      hasAgentMemorySlotErrors,
      isSystemPromptTooLong,
      isVersionPreviewing,
      saveNow,
    ]
  );

  const handlePreviewBeforeSend = useCallback(async () => {
    if (!canConfigureAgentRuntime) {
      toast.error(tRoot('common.unauthorizedDescription'));
      return false;
    }
    if (isVersionPreviewing) {
      return true;
    }
    if (hasAgentMemorySlotErrors) {
      toast.error(t('toasts.fixMemorySlotsBeforeSave'));
      return false;
    }
    if (isSystemPromptTooLong) {
      toast.error(
        t('toasts.systemPromptTooLongBeforeSave', { limit: AGENT_SYSTEM_PROMPT_MAX_LENGTH })
      );
      return false;
    }
    return saveNow({ silent: false, force: true });
  }, [
    canConfigureAgentRuntime,
    hasAgentMemorySlotErrors,
    isSystemPromptTooLong,
    isVersionPreviewing,
    saveNow,
    t,
    tRoot,
  ]);

  const leaveGuardNode = useAgentRuntimeLeaveGuard({
    enabled: canConfigureAgentRuntime && !isVersionPreviewing,
    hasUnsavedChanges: isDirty,
    isSaving,
    onSave: handleSaveBeforeLeave,
  });

  const webAppUrl = agentDetail?.web_app_id ? `/webapp/${agentDetail.web_app_id}/chat` : '';
  const isRuntimeConfigReadOnly = isVersionPreviewing || !canConfigureAgentRuntime;
  const aiChatContext = useMemo(
    () =>
      buildAgentRuntimeAIChatContext({
        agent: agentDetail,
        locale,
        payload: currentPayload,
        promptCharacterCount: systemPromptEffectiveLength,
        isPromptTooLong: isSystemPromptTooLong,
        selectedSkills,
        selectedKnowledgeDatasets,
        permissions,
        organizationRole,
        workspaceRole,
        workspaceRoleName,
        canManageAgent,
        saveState,
        isDirty,
        isVersionPreviewing,
        webAppUrl,
      }),
    [
      agentDetail,
      canManageAgent,
      currentPayload,
      isDirty,
      isSystemPromptTooLong,
      isVersionPreviewing,
      locale,
      organizationRole,
      permissions,
      saveState,
      selectedKnowledgeDatasets,
      selectedSkills,
      systemPromptEffectiveLength,
      webAppUrl,
      workspaceRole,
      workspaceRoleName,
    ]
  );

  return {
    agentId,
    locale,
    t,
    aiChatContext,
    isLoading: isAgentLoading || isConfigLoading || isPermissionsLoading,
    canOpenAgentRuntimeEditor,
    leaveGuardNode,
    previewSheetOpen,
    setPreviewSheetOpen: handlePreviewSheetOpenChange,
    header: {
      agentId,
      agent: agentDetail,
      saveState,
      saveText: getAgentRuntimeSaveText(t, saveState, lastSavedAt),
      isDirty,
      isPublishing: publishAgent.isPending,
      canEditBasicInfo: canUpdateAgent,
      disablePrimaryActions: isVersionPreviewing || !canConfigureAgentRuntime,
      disablePublishActions: isVersionPreviewing || !canPublishAgent,
      disablePublishSettingsActions: isVersionPreviewing || !canManageAgentRuntimeAccess,
      webAppUrl,
      showPreviewAction: canConfigureAgentRuntime,
      isPreviewOpen: previewSheetOpen,
      onSave: handleManualSave,
      onPublish: handlePublish,
      onCopyWebAppUrl: handleCopyWebAppUrl,
      onTogglePreview: () => void handlePreviewSheetOpenChange(!previewSheetOpen),
      onOpenPublishedVersions: handleOpenPublishedVersions,
    },
    version: {
      open: publishedVersionsOpen,
      isLoading: isLoadingVersions,
      isRollingBack: isRollingBackVersion,
      isPreviewing: isVersionPreviewing,
      canRollback: canPublishAgent,
      canOpen: canPublishAgent,
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
      readOnly: isRuntimeConfigReadOnly,
      systemPrompt,
      agentWorkspaceId,
      selectedKnowledgeDatasets,
      selectedSkills,
      databaseBindings,
      workflowBindings,
      workflowCandidatesByBindingID,
      onChangeSystemPrompt: (value: string) => {
        if (isRuntimeConfigReadOnly) return;
        setSystemPrompt(value);
      },
      onOpenOptimizer: () => {
        if (isRuntimeConfigReadOnly) return;
        setPromptOptimizerOpen(true);
      },
    },
    orchestration: {
      readOnly: isRuntimeConfigReadOnly,
      agentId,
      locale,
      openSections,
      modelValue,
      homeTitle,
      openingStatement,
      inputPlaceholder,
      selectedSkills,
      normalizedSelectedSkillIds,
      selectableSkillsCount: skillCandidatesResponse?.data.total ?? selectableSkills.length,
      isSkillsLoading: isSkillCandidatesLoading || isSkillCandidatesFetching,
      isSkillConfigLoading: false,
      isDatasetsLoading: isSelectedDatasetsLoading,
      canBindKnowledge,
      selectedKnowledgeDatasets,
      selectedKnowledgeDatasetIds: knowledgeDatasetIds,
      databaseBindings,
      workflowBindings,
      workflowCandidatesByBindingID,
      isWorkflowCandidatesLoading,
      suggestedQuestions,
      isGeneratingSuggestions,
      fileUploadEnabled,
      agentMemoryEnabled,
      agentMemorySlots,
      agentMemorySlotValidationErrors,
      defaultHomeTitle,
      defaultInputPlaceholder,
      openingGuideBrand: agentOpeningGuideBrand,
      onToggleSection: (section: AgentConfigSection) =>
        setOpenSections(current => ({ ...current, [section]: !current[section] })),
      onChangeModelValue: (value: ModelSelectorParameterValue) => {
        if (isRuntimeConfigReadOnly) return;
        setModelValue(value);
      },
      onChangeHomeTitle: (value: string) => {
        if (isRuntimeConfigReadOnly) return;
        setHomeTitle(value);
      },
      onChangeOpeningStatement: (value: string) => {
        if (isRuntimeConfigReadOnly) return;
        setOpeningStatement(value);
      },
      onChangeInputPlaceholder: (value: string) => {
        if (isRuntimeConfigReadOnly) return;
        setInputPlaceholder(value);
      },
      onOpenSkillDialog: () => {
        if (isRuntimeConfigReadOnly) return;
        setSkillDialogOpen(true);
      },
      onOpenKnowledgeDialog: () => {
        if (isRuntimeConfigReadOnly || !canBindKnowledge) return;
        setKnowledgeDialogOpen(true);
      },
      onOpenWorkflowDialog: () => {
        if (isRuntimeConfigReadOnly) return;
        setWorkflowDialogOpen(true);
      },
      onToggleSkill: (skillId: string, checked: boolean) => {
        if (isRuntimeConfigReadOnly) return;
        handleToggleSkill(skillId, checked);
      },
      onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => {
        if (isRuntimeConfigReadOnly || !canBindKnowledge) return;
        handleToggleKnowledgeDataset(datasetId, checked);
      },
      onChangeDatabaseBindings: (value: AgentDatabaseBinding[]) => {
        if (isRuntimeConfigReadOnly) return;
        setDatabaseBindings(value);
      },
      onChangeWorkflowBindings: (value: AgentWorkflowBinding[]) => {
        if (isRuntimeConfigReadOnly) return;
        setWorkflowBindings(normalizeAgentWorkflowBindings(value));
      },
      onGenerateSuggestedQuestions: async (value: OpeningStatementDialogValue) => {
        if (isRuntimeConfigReadOnly) return undefined;
        return handleGenerateSuggestedQuestions(value);
      },
      onChangeSuggestedQuestions: (value: string[]) => {
        if (isRuntimeConfigReadOnly) return;
        setSuggestedQuestions(value);
      },
      onChangeFileUploadEnabled: (value: boolean) => {
        if (isRuntimeConfigReadOnly) return;
        setFileUploadEnabled(value);
      },
      onChangeAgentMemoryEnabled: (value: boolean) => {
        if (isRuntimeConfigReadOnly) return;
        setAgentMemoryEnabled(value);
      },
      onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => {
        if (isRuntimeConfigReadOnly) return;
        setAgentMemorySlots(value);
      },
    },
    preview: {
      canUseDraftPreview: canConfigureAgentRuntime,
      controller: chatController,
      modelSelectorValue,
      modelProps: selectedModelProps,
      useMemory: false,
      fileUploadEnabled,
      suggestions: currentPayload.suggested_questions,
      inputPlaceholder: currentPayload.input_placeholder,
      openingGuideBrand: agentOpeningGuideBrand,
      homeTitle: currentPayload.home_title || defaultHomeTitle,
      openingStatement: currentPayload.opening_statement,
      beforeSend: handlePreviewBeforeSend,
      onOpenMemoryValues: () => setMemoryValuesOpen(true),
      onModelChange: handleModelChange,
      onClose: () => void handlePreviewSheetOpenChange(false),
    },
    dialogs: {
      promptOptimizer: {
        open: promptOptimizerOpen,
        onOpenChange: setPromptOptimizerOpen,
        initialPrompt: systemPrompt,
        sourceLabel: t('prompt.optimizerSourceLabel'),
        sourceHelpText: t('prompt.optimizerSourceHelp'),
        sourceResetLabel: t('prompt.optimizerSourceReset'),
        targetMaxChars: AGENT_SYSTEM_PROMPT_MAX_LENGTH,
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
        agentId,
        open: skillDialogOpen,
        locale,
        normalizedSelectedSkillIds,
        onOpenChange: setSkillDialogOpen,
        onConfirmSkills: handleConfirmSkills,
      },
      knowledge: {
        agentId,
        open: knowledgeDialogOpen && canBindKnowledge,
        selectedDatasetIds: knowledgeDatasetIds,
        onOpenChange: setKnowledgeDialogOpen,
        onConfirmDatasets: handleConfirmKnowledgeDatasets,
      },
      workflow: {
        agentId,
        open: workflowDialogOpen,
        bindings: workflowBindings,
        onOpenChange: setWorkflowDialogOpen,
        onConfirmWorkflows: handleConfirmWorkflows,
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
