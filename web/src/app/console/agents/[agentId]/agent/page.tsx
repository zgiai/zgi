'use client';

import { use, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import {
  ArrowLeft,
  Bot,
  CheckCircle2,
  ChevronRight,
  ExternalLink,
  Loader2,
  MoreHorizontal,
  Plus,
  Save,
  Sparkles,
  Trash2,
  Upload,
} from 'lucide-react';
import { toast } from 'sonner';
import Chat, { createAgentDraftTransport, useAIChatController } from '@/components/chat';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import {
  ModelSelectorParameter,
  type ModelSelectorParameterValue,
  type ModelSelectorValue,
} from '@/components/common/model-selector';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useAgent, useAgentConfig, usePublishAgent } from '@/hooks/agent/use-agents';
import { useAIChatSkillConfig, useAIChatSkills } from '@/hooks/aichat/use-aichat-skills';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type { UpdateAgentRuntimeConfigRequest } from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentRuntimePageProps {
  params: Promise<{ agentId: string }>;
}

type SaveState = 'idle' | 'dirty' | 'saving' | 'saved' | 'error';

function toModelParams(
  params: Record<string, unknown> | undefined
): Record<string, number | string | boolean> {
  const next: Record<string, number | string | boolean> = {};
  for (const [key, value] of Object.entries(params ?? {})) {
    if (typeof value === 'number' || typeof value === 'string' || typeof value === 'boolean') {
      next[key] = value;
    }
  }
  return next;
}

function buildSignature(payload: UpdateAgentRuntimeConfigRequest): string {
  return JSON.stringify({
    ...payload,
    enabled_skill_ids: [...payload.enabled_skill_ids].sort(),
  });
}

function formatSaveText(state: SaveState, updatedAt?: number | null): string {
  if (state === 'saving') return '正在保存...';
  if (state === 'dirty') return '有未保存更改';
  if (state === 'error') return '保存失败';
  if (updatedAt) {
    return `草稿已保存于 ${new Date(updatedAt * 1000).toLocaleTimeString('zh-CN', {
      hour12: false,
    })}`;
  }
  return '草稿已保存';
}

function pickAgentInitials(name?: string): string {
  const trimmed = name?.trim();
  if (!trimmed) return 'A';
  return trimmed.slice(0, 2).toUpperCase();
}

export default function AgentRuntimePage({ params }: AgentRuntimePageProps) {
  const { agentId } = use(params);
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale } = useLocale();
  const { agent, isLoading: isAgentLoading } = useAgent(agentId);
  const { data: configResponse, isLoading: isConfigLoading } = useAgentConfig(agentId);
  const { data: allSkills = [], isLoading: isSkillsLoading } = useAIChatSkills();
  const { data: organizationSkillConfig, isLoading: isSkillConfigLoading } = useAIChatSkillConfig();
  const publishAgent = usePublishAgent();
  const config = configResponse?.data;
  const agentDetail = agent?.data;

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
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [promptOptimizerOpen, setPromptOptimizerOpen] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>('idle');
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(null);
  const lastSavedSignatureRef = useRef('');
  const autosaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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
      suggested_questions: suggestedQuestions
        .map(item => item.trim())
        .filter(Boolean)
        .slice(0, 6),
    }),
    [
      fileUploadEnabled,
      modelValue,
      normalizedSelectedSkillIds,
      suggestedQuestions,
      systemPrompt,
      useMemory,
    ]
  );
  const currentSignature = useMemo(() => buildSignature(currentPayload), [currentPayload]);
  const isDirty = Boolean(
    lastSavedSignatureRef.current && currentSignature !== lastSavedSignatureRef.current
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
    setSuggestedQuestions(nextPayload.suggested_questions);
    setLastSavedAt(config.updated_at ?? null);
    lastSavedSignatureRef.current = buildSignature(nextPayload);
    setSaveState('saved');
  }, [config]);

  useEffect(() => {
    if (!lastSavedSignatureRef.current) return;
    setSaveState(currentSignature === lastSavedSignatureRef.current ? 'saved' : 'dirty');
  }, [currentSignature]);

  const saveConfig = useCallback(
    async (silent: boolean) => {
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
          suggested_questions:
            response.data.suggested_questions ?? currentPayload.suggested_questions,
        };
        setLastSavedAt(updatedAt);
        lastSavedSignatureRef.current = buildSignature(savedPayload);
        queryClient.setQueryData(AGENT_KEYS.config(agentId), response);
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(agentId) });
        setSaveState('saved');
        if (!silent) {
          toast.success('配置已保存');
        }
      } catch (error) {
        setSaveState('error');
        if (!silent) {
          toast.error(getErrorMessage(error) || '保存失败');
        }
      }
    },
    [agentId, currentPayload, currentSignature, queryClient]
  );

  useEffect(() => {
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
  }, [currentSignature, saveConfig]);

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

  const handleAddSuggestedQuestion = useCallback(() => {
    setSuggestedQuestions(current => (current.length >= 6 ? current : [...current, '']));
  }, []);

  const handleChangeSuggestedQuestion = useCallback((index: number, value: string) => {
    setSuggestedQuestions(current =>
      current.map((item, itemIndex) => (itemIndex === index ? value : item))
    );
  }, []);

  const handleRemoveSuggestedQuestion = useCallback((index: number) => {
    setSuggestedQuestions(current => current.filter((_, itemIndex) => itemIndex !== index));
  }, []);

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

  if (isAgentLoading || isConfigLoading) {
    return (
      <div className="flex h-full w-full flex-col overflow-hidden bg-background">
        <div className="flex h-14 shrink-0 items-center gap-3 border-b px-4">
          <Skeleton className="size-8 rounded-md" />
          <Skeleton className="h-5 w-40" />
          <Skeleton className="ml-auto h-9 w-24" />
        </div>
        <div className="grid min-h-0 flex-1 grid-cols-3 divide-x">
          <div className="p-5">
            <Skeleton className="h-full w-full" />
          </div>
          <div className="p-5">
            <Skeleton className="h-full w-full" />
          </div>
          <div className="p-5">
            <Skeleton className="h-full w-full" />
          </div>
        </div>
      </div>
    );
  }

  const saveText = formatSaveText(saveState, lastSavedAt);

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-background">
      <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b bg-background px-4">
        <div className="flex min-w-0 items-center gap-3">
          <Button isIcon variant="ghost" className="size-8" onClick={() => router.back()}>
            <ArrowLeft className="size-4" />
          </Button>
          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-semibold text-primary">
            {agentDetail?.icon_type === 'image' && agentDetail.icon_url ? (
              <img
                src={agentDetail.icon_url}
                alt=""
                className="size-full rounded-lg object-cover"
              />
            ) : (
              pickAgentInitials(agentDetail?.name)
            )}
          </div>
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <h1 className="truncate text-sm font-semibold">{agentDetail?.name || 'Agent'}</h1>
              <Badge variant="outline" className="h-6 gap-1 rounded-md px-2 text-[11px]">
                <Bot className="size-3" />
                Agent
              </Badge>
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {agentDetail?.description || '自主规划模式'}
            </div>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <div
            className={cn(
              'hidden items-center gap-1.5 text-xs text-muted-foreground md:flex',
              saveState === 'error' ? 'text-destructive' : ''
            )}
          >
            {saveState === 'saving' ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : saveState === 'saved' ? (
              <CheckCircle2 className="size-3.5 text-success" />
            ) : null}
            {saveText}
          </div>
          <Button
            variant="outline"
            onClick={handleManualSave}
            disabled={saveState === 'saving' || !isDirty}
          >
            <Save className="mr-2 size-4" />
            保存
          </Button>
          <Button
            onClick={handlePublish}
            disabled={publishAgent.isPending || saveState === 'saving'}
          >
            <Upload className="mr-2 size-4" />
            发布
          </Button>
          <Button isIcon variant="ghost" className="size-8">
            <MoreHorizontal className="size-4" />
          </Button>
        </div>
      </header>

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(320px,0.95fr)_minmax(320px,0.95fr)_minmax(420px,1.2fr)] divide-x overflow-hidden">
        <section className="flex min-w-0 flex-col overflow-hidden">
          <div className="flex h-12 shrink-0 items-center justify-between px-5">
            <div>
              <h2 className="text-sm font-semibold">人设与回复逻辑</h2>
              <p className="text-xs text-muted-foreground">
                系统提示词会在每轮对话前注入 runtime。
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 gap-1.5 px-2 text-xs"
              onClick={() => setPromptOptimizerOpen(true)}
              disabled={!systemPrompt.trim()}
            >
              <Sparkles className="size-3.5" />
              优化
            </Button>
          </div>
          <div className="min-h-0 flex-1 px-5 pb-5">
            <Textarea
              value={systemPrompt}
              onChange={event => setSystemPrompt(event.target.value)}
              placeholder="描述智能体的角色、目标、工作方式、限制和回答风格。"
              className="h-full min-h-0 resize-none border-0 bg-muted/35 p-4 text-sm leading-6 shadow-none focus-visible:ring-1"
            />
          </div>
        </section>

        <section className="flex min-w-0 flex-col overflow-hidden">
          <div className="flex h-12 shrink-0 items-center justify-between px-5">
            <div>
              <h2 className="text-sm font-semibold">编排</h2>
              <p className="text-xs text-muted-foreground">
                模型、Skill、文件和记忆配置会保存到 AGENT 草稿。
              </p>
            </div>
          </div>
          <ScrollArea className="min-h-0 flex-1">
            <div className="space-y-5 px-5 pb-6">
              <section className="space-y-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <ChevronRight className="size-4 text-muted-foreground" />
                  模型
                </div>
                <ModelSelectorParameter
                  modelType="text-chat"
                  value={modelValue}
                  onChange={setModelValue}
                  className="w-full"
                />
              </section>

              <Separator className="h-px" />

              <section className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    <ChevronRight className="size-4 text-muted-foreground" />
                    Skill
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant="subtle">{normalizedSelectedSkillIds.length} 已选</Badge>
                    <Button
                      isIcon
                      variant="outline"
                      className="size-8"
                      onClick={() => setSkillDialogOpen(true)}
                      aria-label="添加 Skill"
                      title="添加 Skill"
                    >
                      <Plus className="size-4" />
                    </Button>
                  </div>
                </div>
                {isSkillsLoading || isSkillConfigLoading ? (
                  <div className="space-y-2">
                    <Skeleton className="h-14 w-full" />
                    <Skeleton className="h-14 w-full" />
                  </div>
                ) : selectableSkills.length === 0 ? (
                  <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                    当前组织还没有启用可选 Skill。
                    <Button asChild variant="link" className="h-auto px-1 text-sm">
                      <Link href="/dashboard/organization/aichat-skills">
                        去启用 Skill
                        <ExternalLink className="ml-1 size-3.5" />
                      </Link>
                    </Button>
                  </div>
                ) : selectedSkills.length === 0 ? (
                  <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                    还没有为这个 Agent 添加 Skill。
                  </div>
                ) : (
                  <div className="space-y-2">
                    {selectedSkills.map(skill => {
                      const display = getAIChatSkillDisplayInfo(skill, locale);
                      return (
                        <div
                          key={skill.skill_id}
                          className="flex items-start gap-3 rounded-md border bg-background p-3"
                        >
                          <div className="min-w-0 flex-1">
                            <div className="truncate text-sm font-medium">{display.label}</div>
                            <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                              {display.description || skill.description || skill.skill_id}
                            </div>
                          </div>
                          <Button
                            isIcon
                            variant="ghost"
                            className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
                            onClick={() => handleToggleSkill(skill.skill_id, false)}
                            aria-label={`移除 ${display.label}`}
                            title={`移除 ${display.label}`}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      );
                    })}
                  </div>
                )}
              </section>

              <Separator className="h-px" />

              <section className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    <ChevronRight className="size-4 text-muted-foreground" />
                    请求示例
                  </div>
                  <Button
                    isIcon
                    variant="outline"
                    className="size-8"
                    onClick={handleAddSuggestedQuestion}
                    disabled={suggestedQuestions.length >= 6}
                    aria-label="添加请求示例"
                    title="添加请求示例"
                  >
                    <Plus className="size-4" />
                  </Button>
                </div>
                <div className="space-y-2">
                  {suggestedQuestions.length === 0 ? (
                    <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                      还没有配置请求示例，WebApp 首页将不展示示例问题。
                    </div>
                  ) : (
                    suggestedQuestions.map((question, index) => (
                      <div key={index} className="flex items-center gap-2">
                        <Input
                          value={question}
                          maxLength={200}
                          placeholder="例如：帮我总结这份文档的重点"
                          onChange={event =>
                            handleChangeSuggestedQuestion(index, event.target.value)
                          }
                        />
                        <Button
                          isIcon
                          variant="ghost"
                          className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                          onClick={() => handleRemoveSuggestedQuestion(index)}
                          aria-label="删除请求示例"
                          title="删除请求示例"
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </div>
                    ))
                  )}
                </div>
              </section>

              <Separator className="h-px" />

              <section className="space-y-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <ChevronRight className="size-4 text-muted-foreground" />
                  文件
                </div>
                <div className="flex items-center justify-between rounded-md border p-3">
                  <div>
                    <div className="text-sm font-medium">允许文件上传</div>
                    <div className="text-xs text-muted-foreground">
                      开启后，调试和 WebApp 可以上传文档或图片。
                    </div>
                  </div>
                  <Switch checked={fileUploadEnabled} onCheckedChange={setFileUploadEnabled} />
                </div>
              </section>

              <Separator className="h-px" />

              <section className="space-y-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <ChevronRight className="size-4 text-muted-foreground" />
                  记忆
                </div>
                <div className="flex items-center justify-between rounded-md border p-3">
                  <div>
                    <div className="text-sm font-medium">长期记忆</div>
                    <div className="text-xs text-muted-foreground">
                      开启后，调试与运行会允许 runtime 调用记忆能力。
                    </div>
                  </div>
                  <Switch checked={useMemory} onCheckedChange={setUseMemory} />
                </div>
              </section>
            </div>
          </ScrollArea>
        </section>

        <section className="flex min-w-0 flex-col overflow-hidden">
          <div className="flex h-12 shrink-0 items-center justify-between px-5">
            <div>
              <h2 className="text-sm font-semibold">预览与调试</h2>
              <p className="text-xs text-muted-foreground">使用当前草稿配置运行 ChatRuntime。</p>
            </div>
            <Badge variant="subtle">Draft</Badge>
          </div>
          <div className="min-h-0 flex-1">
            <Chat
              mode="aichat"
              controller={chatController}
              modelSelectorValue={modelSelectorValue}
              onModelChange={handleModelChange}
              variant="embedded"
              showModelSelector={false}
              showMemoryToggle={false}
              forcedUseMemory={useMemory}
              enableUpload={fileUploadEnabled}
              suggestions={currentPayload.suggested_questions}
              homeBrand={agentHomeBrand}
              homeTitle={agentDetail?.name || '开启新对话'}
              homeDescription="使用当前草稿配置测试 Agent 效果"
            />
          </div>
        </section>
      </div>

      <PromptOptimizerDialog
        open={promptOptimizerOpen}
        onOpenChange={setPromptOptimizerOpen}
        initialPrompt={systemPrompt}
        sourceLabel="系统提示词"
        sourceHelpText="优化器会基于当前系统提示词重写角色、任务、约束与输出要求；当前 AGENT 不注入节点变量。"
        sourceResetLabel="恢复当前系统提示词"
        initialModel={
          modelValue.model
            ? {
                provider: modelValue.provider,
                model: modelValue.model,
              }
            : null
        }
        applyLabel="应用到系统提示词"
        onApplyResult={handleApplyOptimizedPrompt}
      />

      <Dialog open={skillDialogOpen} onOpenChange={setSkillDialogOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>添加 Skill</DialogTitle>
            <DialogDescription>
              从当前组织已启用的 Skill 中选择这个 Agent 可使用的能力。
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="max-h-[520px]">
            {selectableSkills.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                当前组织还没有启用可选 Skill。
              </div>
            ) : (
              <div className="space-y-2">
                {selectableSkills.map(skill => {
                  const display = getAIChatSkillDisplayInfo(skill, locale);
                  const checked = normalizedSelectedSkillIds.includes(skill.skill_id);
                  return (
                    <label
                      key={skill.skill_id}
                      className="flex cursor-pointer items-start gap-3 rounded-md border bg-background p-3 transition-colors hover:bg-muted/40"
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={value => handleToggleSkill(skill.skill_id, value === true)}
                        className="mt-0.5"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">{display.label}</span>
                        <span className="mt-1 line-clamp-2 block text-xs leading-5 text-muted-foreground">
                          {display.description || skill.description || skill.skill_id}
                        </span>
                      </span>
                    </label>
                  );
                })}
              </div>
            )}
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setSkillDialogOpen(false)}>完成</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
