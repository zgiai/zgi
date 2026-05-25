'use client';

import { use, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  createAgentDraftTransport,
  useAIChatController,
} from '@/components/chat';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import type { ModelSelectorParameterValue, ModelSelectorValue } from '@/components/common/model-selector';
import {
  AgentRuntimeHeader,
  AgentRuntimeLoadingState,
  AgentRuntimeOrchestrationPanel,
  AgentRuntimePreviewPanel,
  AgentRuntimePromptPanel,
  AgentRuntimeVersionPopover,
  AgentRuntimeSkillDialog,
  buildAgentRuntimeSignature,
  pickAgentInitials,
  toModelParams,
  type AgentConfigSection,
  type AgentPublishedVersionListItem,
  type AgentRuntimeSaveState,
} from '@/components/agents/agent-runtime';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { useAgent, useAgentConfig, usePublishAgent } from '@/hooks/agent/use-agents';
import { useAIChatSkillConfig, useAIChatSkills } from '@/hooks/aichat/use-aichat-skills';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type { AgentRuntimeConfig, UpdateAgentRuntimeConfigRequest } from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentRuntimePageProps {
  params: Promise<{ agentId: string }>;
}

function getSaveText(
  t: ReturnType<typeof useT<'agents.agentRuntime'>>,
  saveState: AgentRuntimeSaveState,
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
  lastSavedAt: number | null;
  lastSavedSignature: string;
  saveState: AgentRuntimeSaveState;
}

export default function AgentRuntimePage({ params }: AgentRuntimePageProps) {
  const { agentId } = use(params);
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale } = useLocale();
  const t = useT('agents.agentRuntime');
  const { agent, isLoading: isAgentLoading } = useAgent(agentId);
  const { data: configResponse, isLoading: isConfigLoading } = useAgentConfig(agentId);
  const { data: allSkills = [], isLoading: isSkillsLoading } = useAIChatSkills();
  const { data: organizationSkillConfig, isLoading: isSkillConfigLoading } = useAIChatSkillConfig();
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
  const [useMemory, setUseMemory] = useState(false);
  const [fileUploadEnabled, setFileUploadEnabled] = useState(false);
  const [homeTitle, setHomeTitle] = useState(defaultHomeTitle);
  const [inputPlaceholder, setInputPlaceholder] = useState(defaultInputPlaceholder);
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [promptOptimizerOpen, setPromptOptimizerOpen] = useState(false);
  const [skillSearch, setSkillSearch] = useState('');
  const [showSelectedSkillsOnly, setShowSelectedSkillsOnly] = useState(false);
  const [isGeneratingSuggestions, setIsGeneratingSuggestions] = useState(false);
  const [publishedVersionsOpen, setPublishedVersionsOpen] = useState(false);
  const [publishedVersions, setPublishedVersions] = useState<AgentPublishedVersionListItem[]>([]);
  const [isLoadingVersions, setIsLoadingVersions] = useState(false);
  const [isRollingBackVersion, setIsRollingBackVersion] = useState(false);
  const [selectedPublishedVersionId, setSelectedPublishedVersionId] = useState('');
  const [isVersionPreviewing, setIsVersionPreviewing] = useState(false);
  const [openSections, setOpenSections] = useState<Record<AgentConfigSection, boolean>>({
    experience: true,
    model: true,
    skills: true,
    files: true,
    memory: true,
  });
  const [saveState, setSaveState] = useState<AgentRuntimeSaveState>('idle');
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(null);
  const lastSavedSignatureRef = useRef('');
  const autosaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const versionPreviewBackupRef = useRef<VersionPreviewBackup | null>(null);

  const enabledOrganizationSkillIds = useMemo(
    () => new Set(organizationSkillConfig?.enabled_skill_ids ?? []),
    [organizationSkillConfig?.enabled_skill_ids]
  );
  const selectableSkills = useMemo(
    () => allSkills.filter(skill => enabledOrganizationSkillIds.has(skill.skill_id)),
    [allSkills, enabledOrganizationSkillIds]
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
      use_memory: useMemory,
      file_upload_enabled: fileUploadEnabled,
      home_title: homeTitle.trim() || defaultHomeTitle,
      input_placeholder: inputPlaceholder.trim() || defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: suggestedQuestions
        .map(item => item.trim())
        .filter(Boolean)
        .slice(0, 6),
    }),
    [
      defaultHomeTitle,
      defaultInputPlaceholder,
      fileUploadEnabled,
      homeTitle,
      inputPlaceholder,
      modelValue,
      normalizedSelectedSkillIds,
      suggestedQuestions,
      systemPrompt,
      useMemory,
    ]
  );
  const currentSignature = useMemo(
    () => buildAgentRuntimeSignature(currentPayload),
    [currentPayload]
  );
  const isDirty = Boolean(
    !isVersionPreviewing &&
      lastSavedSignatureRef.current &&
      currentSignature !== lastSavedSignatureRef.current
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
    if (!config) return;
    const nextPayload: UpdateAgentRuntimeConfigRequest = {
      system_prompt: config.system_prompt ?? '',
      model_provider: config.model_provider ?? '',
      model: config.model ?? '',
      model_parameters: toModelParams(config.model_parameters),
      enabled_skill_ids: config.enabled_skill_ids ?? [],
      use_memory: config.use_memory ?? false,
      file_upload_enabled: config.file_upload_enabled ?? false,
      home_title:
        !config.home_title || config.home_title === 'title'
          ? defaultHomeTitle
          : config.home_title,
      input_placeholder: config.input_placeholder ?? defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: config.suggested_questions ?? [],
    };
    setSystemPrompt(nextPayload.system_prompt);
    setModelValue({
      provider: nextPayload.model_provider,
      model: nextPayload.model,
      params: toModelParams(nextPayload.model_parameters),
    });
    setSelectedSkillIds(nextPayload.enabled_skill_ids);
    setUseMemory(nextPayload.use_memory);
    setFileUploadEnabled(nextPayload.file_upload_enabled);
    setHomeTitle(nextPayload.home_title);
    setInputPlaceholder(nextPayload.input_placeholder);
    setSuggestedQuestions(nextPayload.suggested_questions);
    setLastSavedAt(config.updated_at ?? null);
    lastSavedSignatureRef.current = buildAgentRuntimeSignature(nextPayload);
    setSaveState('saved');
  }, [config, defaultHomeTitle, defaultInputPlaceholder]);

  const applyRuntimePayload = useCallback((payload: UpdateAgentRuntimeConfigRequest) => {
    setSystemPrompt(payload.system_prompt);
    setModelValue({
      provider: payload.model_provider,
      model: payload.model,
      params: toModelParams(payload.model_parameters),
    });
    setSelectedSkillIds(payload.enabled_skill_ids);
    setUseMemory(payload.use_memory);
    setFileUploadEnabled(payload.file_upload_enabled);
    setHomeTitle(payload.home_title);
    setInputPlaceholder(payload.input_placeholder);
    setSuggestedQuestions(payload.suggested_questions);
  }, []);

  const payloadFromRuntimeConfig = useCallback(
    (runtimeConfig: AgentRuntimeConfig): UpdateAgentRuntimeConfigRequest => ({
      system_prompt: runtimeConfig.system_prompt ?? '',
      model_provider: runtimeConfig.model_provider ?? '',
      model: runtimeConfig.model ?? '',
      model_parameters: toModelParams(runtimeConfig.model_parameters),
      enabled_skill_ids: runtimeConfig.enabled_skill_ids ?? [],
      use_memory: runtimeConfig.use_memory ?? false,
      file_upload_enabled: runtimeConfig.file_upload_enabled ?? false,
      home_title:
        !runtimeConfig.home_title || runtimeConfig.home_title === 'title'
          ? defaultHomeTitle
          : runtimeConfig.home_title,
      input_placeholder: runtimeConfig.input_placeholder ?? defaultInputPlaceholder,
      theme_color: 'default',
      suggested_questions: runtimeConfig.suggested_questions ?? [],
    }),
    [defaultHomeTitle, defaultInputPlaceholder]
  );

  useEffect(() => {
    if (!lastSavedSignatureRef.current || isVersionPreviewing) return;
    setSaveState(currentSignature === lastSavedSignatureRef.current ? 'saved' : 'dirty');
  }, [currentSignature, isVersionPreviewing]);

  const saveConfig = useCallback(
    async (silent: boolean) => {
      if (isVersionPreviewing) {
        if (!silent) {
          toast.info(t('toasts.finishVersionPreviewFirst'));
        }
        return;
      }
      if (currentSignature === lastSavedSignatureRef.current) return;
      setSaveState('saving');
      try {
        const response = await agentService.updateAgentConfig(agentId, currentPayload);
        const updatedAt = response.data.updated_at ?? Math.floor(Date.now() / 1000);
        const savedPayload = {
          ...currentPayload,
          model_parameters: toModelParams(response.data.model_parameters),
          file_upload_enabled:
            response.data.file_upload_enabled ?? currentPayload.file_upload_enabled,
          home_title: response.data.home_title ?? currentPayload.home_title,
          input_placeholder:
            response.data.input_placeholder ?? currentPayload.input_placeholder,
          theme_color: 'default',
          suggested_questions:
            response.data.suggested_questions ?? currentPayload.suggested_questions,
        };
        setLastSavedAt(updatedAt);
        lastSavedSignatureRef.current = buildAgentRuntimeSignature(savedPayload);
        queryClient.setQueryData(AGENT_KEYS.config(agentId), response);
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(agentId) });
        setSaveState('saved');
        if (!silent) {
          toast.success(t('toasts.saveSuccess'));
        }
      } catch (error) {
        setSaveState('error');
        if (!silent) {
          toast.error(getErrorMessage(error) || t('toasts.saveFailed'));
        }
      }
    },
    [agentId, currentPayload, currentSignature, isVersionPreviewing, queryClient, t]
  );

  useEffect(() => {
    if (isVersionPreviewing) return;
    if (!lastSavedSignatureRef.current || currentSignature === lastSavedSignatureRef.current) {
      return;
    }
    if (autosaveTimerRef.current) {
      clearTimeout(autosaveTimerRef.current);
    }
    autosaveTimerRef.current = setTimeout(() => {
      void saveConfig(true);
    }, 900);
    return () => {
      if (autosaveTimerRef.current) {
        clearTimeout(autosaveTimerRef.current);
      }
    };
  }, [currentSignature, isVersionPreviewing, saveConfig]);

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
      setLastSavedAt(backup.lastSavedAt);
      lastSavedSignatureRef.current = backup.lastSavedSignature;
      setSaveState(backup.saveState);
    }
    versionPreviewBackupRef.current = null;
    setSelectedPublishedVersionId('');
    setIsVersionPreviewing(false);
  }, [applyRuntimePayload]);

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
          lastSavedAt,
          lastSavedSignature: lastSavedSignatureRef.current,
          saveState,
        };
      }
      if (autosaveTimerRef.current) {
        clearTimeout(autosaveTimerRef.current);
        autosaveTimerRef.current = null;
      }
      const nextPayload = payloadFromRuntimeConfig(version.config_snapshot);
      applyRuntimePayload(nextPayload);
      setSelectedPublishedVersionId(version.id);
      setIsVersionPreviewing(true);
      setSaveState('previewing');
    },
    [applyRuntimePayload, currentPayload, lastSavedAt, payloadFromRuntimeConfig, publishedVersions, saveState]
  );

  const handleConfirmVersionRollback = useCallback(
    async () => {
      if (!selectedPublishedVersionId || isRollingBackVersion) return;
      setIsRollingBackVersion(true);
      try {
        const response = await agentService.rollbackPublishedVersion(agentId, {
          version_id: selectedPublishedVersionId,
        });
        const nextPayload = payloadFromRuntimeConfig(response.data);
        applyRuntimePayload(nextPayload);
        setLastSavedAt(response.data.updated_at ?? Math.floor(Date.now() / 1000));
        lastSavedSignatureRef.current = buildAgentRuntimeSignature(nextPayload);
        queryClient.setQueryData(AGENT_KEYS.config(agentId), response);
        setSaveState('saved');
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
    },
    [
      applyRuntimePayload,
      agentId,
      isRollingBackVersion,
      payloadFromRuntimeConfig,
      queryClient,
      selectedPublishedVersionId,
      t,
    ]
  );

  const handleApplyOptimizedPrompt = useCallback((payload: { text: string }) => {
    setSystemPrompt(payload.text);
    setPromptOptimizerOpen(false);
  }, []);

  const handleManualSave = useCallback(() => {
    void saveConfig(false);
  }, [saveConfig]);

  const handlePublish = useCallback(async () => {
    await saveConfig(true);
    publishAgent.mutate({ agentId });
  }, [agentId, publishAgent, saveConfig]);

  const webAppUrl = agentDetail?.web_app_id ? `/webapp/${agentDetail.web_app_id}/chat` : '';

  if (isAgentLoading || isConfigLoading) {
    return <AgentRuntimeLoadingState />;
  }

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-background">
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
        onBack={() => router.back()}
        onSave={handleManualSave}
        onPublish={handlePublish}
        onCopyWebAppUrl={handleCopyWebAppUrl}
        onOpenPublishedVersions={handleOpenPublishedVersions}
      />

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(320px,0.95fr)_minmax(320px,0.95fr)_minmax(420px,1.2fr)] divide-x overflow-hidden">
        <AgentRuntimePromptPanel
          systemPrompt={systemPrompt}
          onChangeSystemPrompt={setSystemPrompt}
          onOpenOptimizer={() => setPromptOptimizerOpen(true)}
        />
        <AgentRuntimeOrchestrationPanel
          locale={locale}
          openSections={openSections}
          modelValue={modelValue}
          homeTitle={homeTitle}
          inputPlaceholder={inputPlaceholder}
          selectedSkills={selectedSkills}
          normalizedSelectedSkillIds={normalizedSelectedSkillIds}
          selectableSkillsCount={selectableSkills.length}
          isSkillsLoading={isSkillsLoading}
          isSkillConfigLoading={isSkillConfigLoading}
          suggestedQuestions={suggestedQuestions}
          isGeneratingSuggestions={isGeneratingSuggestions}
          systemPrompt={systemPrompt}
          fileUploadEnabled={fileUploadEnabled}
          useMemory={useMemory}
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
          onGenerateSuggestedQuestions={() => void handleGenerateSuggestedQuestions()}
          onChangeSuggestedQuestions={setSuggestedQuestions}
          onChangeFileUploadEnabled={setFileUploadEnabled}
          onChangeUseMemory={setUseMemory}
        />
        <AgentRuntimePreviewPanel
          controller={chatController}
          modelSelectorValue={modelSelectorValue}
          useMemory={useMemory}
          fileUploadEnabled={fileUploadEnabled}
          suggestions={currentPayload.suggested_questions}
          inputPlaceholder={currentPayload.input_placeholder}
          homeBrand={agentHomeBrand}
          homeTitle={currentPayload.home_title}
          onModelChange={handleModelChange}
        />
      </div>

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

    </div>
  );
}
