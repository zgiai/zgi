'use client';

import React, { useCallback, useMemo, useEffect, useState, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import type { LLMNodeData } from '../config';
import { cn } from '@/lib/utils';
import { generateClientId } from '@/utils/client-id';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import NodeValueSelector from '../../../common/node-value-selector';
import type { LLMVision } from '../config';
import { AgentType } from '@/services/types/agent';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import PromptEditor from './prompt-editor';
import type { PromptEditorHandle } from './prompt-editor';
import type { WorkflowValueEditorHandle } from '@/components/workflow/ui';
import { Braces, Trash2, Info, WandSparkles, MoreHorizontal, ChevronDown, ChevronRight } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type { VariableInsertValue } from '@/components/workflow/common/workflow-value-inserter/variable-item';
import PromptTemplateSelector from './prompt-template-selector';
import PromptPickerDialog from '@/components/prompts/prompt-picker-dialog';
import PromptFormDialog from '@/components/prompts/prompt-form-dialog';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { PromptPlaygroundDialog } from '@/components/prompts/prompt-playground-dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import OutputVariablesView from '../../../common/output-variables-view';
import { useWorkflowStore } from '../../../store';
import type { WorkflowVariable } from '../../../store/type';
import { useAvailableModels, useProviderModelsAll } from '@/hooks/model/use-model';
import { useT } from '@/i18n';
import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import { useCreatePrompt } from '@/hooks/prompt/use-prompts';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type { CreatePromptRequest, PromptPickerSelection } from '@/services/types/prompt';

interface LLMManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

interface PromptBlockRowProps {
  idx: number;
  role: LLMNodeData['prompt_template'][number]['role'];
  text: string;
  nodeId: string;
  readOnly: boolean;
  onUpdate: (idx: number, patch: Partial<LLMNodeData['prompt_template'][number]>) => void;
  onRemove: (idx: number) => void;
  // Bubble active editor handle up so parent can route inserts to caret
  onFocusedEditor: (idx: number, handle: WorkflowValueEditorHandle) => void;
  defaultCollapsed?: boolean;
}

// Prompt block row with local state and debounced text update to reduce global store writes
const PromptBlockRowComponent: React.FC<PromptBlockRowProps> = ({
  idx,
  role,
  text,
  nodeId,
  readOnly,
  onUpdate,
  onRemove,
  onFocusedEditor,
  defaultCollapsed = false,
}) => {
  // Keep local role/text for smooth typing; sync when parent changes
  const [localRole, setLocalRole] = useState(role);
  const [localText, setLocalText] = useState(text);
  const t = useT();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingTemplate, setPendingTemplate] = useState<string | null>(null);
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  const lastPushedTextRef = useRef(text);

  useEffect(() => {
    setLocalRole(role);
  }, [role, idx]);

  useEffect(() => {
    // Only sync parent text to local state if it's different from what we've already set
    // and different from our last pushed (debounc-committed) value to avoid rollback.
    if (text !== localText && text !== lastPushedTextRef.current) {
      setLocalText(text);
      lastPushedTextRef.current = text;
    }
  }, [text, idx, localText]);

  const debouncedText = useDebouncedValue(localText, 300);

  useEffect(() => {
    if (readOnly) return;
    // Commit debounced text to parent
    if (debouncedText !== text) {
      lastPushedTextRef.current = debouncedText;
      onUpdate(idx, { text: debouncedText });
    }
  }, [debouncedText, text, onUpdate, idx, readOnly]);

  const editorRef = useRef<PromptEditorHandle | null>(null);

  // First system prompt is locked: cannot change role or remove
  const isFirstSystem = idx === 0 && localRole === 'system';

  // Build upstream variable menu for direct insertion
  const getUpstreamVariables = useWorkflowStore(s => s.getUpstreamVariables);
  const upstreams = useMemo(() => {
    if (!nodeId) {
      return [] as Array<{
        nodeId: string;
        nodeType: string;
        nodeTitle?: string;
        variables?: Array<{ key: string; type: WorkflowVariable['type'] }>;
      }>;
    }
    return (getUpstreamVariables(nodeId) || []) as Array<{
      nodeId: string;
      nodeType: string;
      nodeTitle?: string;
      variables?: Array<{ key: string; type: WorkflowVariable['type'] }>;
    }>;
  }, [getUpstreamVariables, nodeId]);

  const totalOptions = useMemo(
    () =>
      upstreams.reduce(
        (acc, src) =>
          acc + (src.variables && Array.isArray(src.variables) ? src.variables.length : 0),
        0
      ),
    [upstreams]
  );

  const roleLabels = {
    system: t('nodes.llm.roles.system'),
    user: t('nodes.llm.roles.user'),
    assistant: t('nodes.llm.roles.assistant'),
  } as const;

  const previewText = localText.trim().replace(/\s+/g, ' ');

  if (collapsed && !isFirstSystem) {
    return (
      <div className="theme-surface border border-muted rounded-xl overflow-hidden shadow-sm">
        <div className="flex items-center justify-between gap-3 px-3 py-2.5">
          <div className="min-w-0 flex items-center gap-2">
            <Badge variant="secondary">{roleLabels[localRole]}</Badge>
            <div className="min-w-0 text-sm text-muted-foreground truncate">
              {previewText || t('nodes.llm.states.emptyBlock')}
            </div>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setCollapsed(false)}
            className="shrink-0"
          >
            <ChevronRight className="h-4 w-4" />
            {t('nodes.llm.actions.expandBlock')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="theme-surface border border-muted rounded-xl overflow-hidden shadow-sm hover:border-primary/30 transition-all duration-300">
      <PromptEditor
        ref={editorRef}
        role={localRole}
        value={localText}
        nodeId={nodeId}
        onChangeRole={nextRole => {
          if (readOnly || isFirstSystem) return; // locked
          setLocalRole(nextRole);
          onUpdate(idx, { role: nextRole });
        }}
        onChange={val => setLocalText(val)}
        readOnly={readOnly}
        roleLocked={isFirstSystem}
        allowedRoles={
          idx === 0
            ? (['system', 'user', 'assistant'] as Array<
                LLMNodeData['prompt_template'][number]['role']
              >)
            : (['user', 'assistant'] as Array<LLMNodeData['prompt_template'][number]['role']>)
        }
        placeholder={t('nodes.llm.placeholders.promptTemplate')}
        actions={
          <div className="flex items-center gap-1.5 bg-muted/30 rounded-lg">
            {!isFirstSystem && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-flex">
                    <Button
                      variant="ghost"
                      size="xs"
                      isIcon
                      className="hover:bg-background"
                      onClick={() => setCollapsed(true)}
                      aria-label={t('nodes.llm.actions.collapseBlock')}
                    >
                      <ChevronDown className="h-4 w-4" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>{t('nodes.llm.actions.collapseBlock')}</TooltipContent>
              </Tooltip>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <Button
                    variant="ghost"
                    size="xs"
                    isIcon
                    className="hover:bg-background"
                    aria-label={t('nodes.common.insertVariable')}
                    disabled={readOnly || totalOptions === 0}
                    title={
                      totalOptions === 0 ? t('nodes.valueInserter.empty.noUpstream') : undefined
                    }
                    onClick={() => editorRef.current?.openVariableSelector()}
                  >
                    <Braces className="h-4 w-4" />
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent>
                {totalOptions === 0
                  ? t('nodes.valueInserter.empty.noUpstream')
                  : t('nodes.common.insertVariable')}
              </TooltipContent>
            </Tooltip>

            {/* Lightbulb template selector only for first system prompt */}
            {isFirstSystem && (
              <>
                <PromptTemplateSelector
                  disabled={readOnly}
                  onApply={(text: string) => {
                    setPendingTemplate(text);
                    setConfirmOpen(true);
                  }}
                />
                <ConfirmDialog
                  variant="default"
                  open={confirmOpen}
                  onOpenChange={setConfirmOpen}
                  title={t('nodes.llm.promptTemplates.confirm.title')}
                  description={t('nodes.llm.promptTemplates.confirm.description')}
                  confirmText={t('nodes.llm.promptTemplates.confirm.confirm')}
                  cancelText={t('common.cancel')}
                  onConfirm={() => {
                    if (typeof pendingTemplate === 'string') {
                      setLocalText(pendingTemplate);
                      editorRef.current?.focus();
                    }
                    setPendingTemplate(null);
                  }}
                />
              </>
            )}

            {!isFirstSystem && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-flex">
                    <Button
                      variant="ghost"
                      size="xs"
                      isIcon
                      className="hover:bg-destructive/10 hover:text-destructive"
                      onClick={() => onRemove(idx)}
                      aria-label={t('nodes.common.remove')}
                      disabled={readOnly || isFirstSystem}
                      title={
                        isFirstSystem ? t('nodes.llm.tips.cannotRemoveFirstSystem') : undefined
                      }
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  {isFirstSystem
                    ? t('nodes.llm.tips.cannotRemoveFirstSystem')
                    : t('nodes.common.remove')}
                </TooltipContent>
              </Tooltip>
            )}
          </div>
        }
        onFocused={h => onFocusedEditor(idx, h)}
      />
    </div>
  );
};

const PromptBlockRow = React.memo(PromptBlockRowComponent);
PromptBlockRow.displayName = 'PromptBlockRow';

/**
 * LLMManager - Manage LLM node data state with cohesive UI sections
 * - Model: provider/model selection, mode, temperature
 * - Prompt: multi-role prompt template (simple textarea for now)

 * - Context: enable + upstream variable selector list
 * - Vision: enable toggle
 * - Structured Output: toggle + JSON schema textarea (raw for now)
 * - Reasoning: format selector
 */
const LLMManager: React.FC<LLMManagerProps> = ({ id: nodeId, className, readOnly = false }) => {
  const t = useT();
  const updateData = useNodeDataUpdate<LLMNodeData>(nodeId);
  const selfNodeData = useNodeData<LLMNodeData>(nodeId);
  const [promptPickerOpen, setPromptPickerOpen] = useState(false);
  const [savePromptOpen, setSavePromptOpen] = useState(false);
  const [optimizerOpen, setOptimizerOpen] = useState(false);
  const [playgroundOpen, setPlaygroundOpen] = useState(false);
  const [managedPreviewOpen, setManagedPreviewOpen] = useState(false);
  const [variableGuideOpen, setVariableGuideOpen] = useState(false);
  const currentWorkspace = useCurrentWorkspace();
  const createPrompt = useCreatePrompt();

  // Auto-fill provider/name using default model when missing
  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: selfNodeData?.model || {},
    enabled: !readOnly && !selfNodeData?.vision?.enabled,
    onInitialize: v => {
      updateData({
        model: {
          provider: v.provider,
          name: v.model,
          mode: selfNodeData?.model?.mode ?? 'chat',
          completion_params: v.params as Record<string, string | number | boolean>,
        },
      });
    },
  });

  useInitializeDefaultModelByUseCase({
    useCase: 'vision',
    currentModel: selfNodeData?.model || {},
    enabled: !readOnly && !!selfNodeData?.vision?.enabled,
    onInitialize: v => {
      updateData({
        model: {
          provider: v.provider,
          name: v.model,
          mode: selfNodeData?.model?.mode ?? 'chat',
          completion_params: v.params as Record<string, string | number | boolean>,
        },
      });
    },
  });

  const activeEditorRef = useRef<WorkflowValueEditorHandle | null>(null);
  const handleEditorFocused = useCallback((idx: number, handle: WorkflowValueEditorHandle) => {
    activeEditorRef.current = handle;
  }, []);

  const safeNodeData = useMemo(
    () => ({
      ...selfNodeData,
      vision: {
        enabled: selfNodeData?.vision?.enabled ?? false,
        configs: selfNodeData?.vision?.configs ?? undefined,
      } as LLMVision,
      model: {
        provider: selfNodeData?.model?.provider ?? '',
        name: selfNodeData?.model?.name ?? '',
        mode: selfNodeData?.model?.mode ?? 'chat',
        completion_params: ((): Record<string, string | number | boolean> => {
          const src = selfNodeData?.model?.completion_params as Record<string, unknown> | undefined;
          if (!src) return {};
          const out: Record<string, string | number | boolean> = {};
          for (const k in src) {
            const v = src[k];
            if (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') {
              out[k] = v;
            }
          }
          return out;
        })(),
      },
      prompt_config: selfNodeData?.prompt_config ?? undefined,
      structured_output_enabled: selfNodeData?.structured_output_enabled ?? false,
    }),
    [selfNodeData]
  );

  const updateModel = useCallback(
    (patch: Partial<LLMNodeData['model']>) => {
      updateData({
        model: {
          provider: safeNodeData.model.provider,
          name: safeNodeData.model.name,
          mode: safeNodeData.model.mode,
          completion_params: safeNodeData.model.completion_params,
          ...patch,
        },
      });
    },
    [updateData, safeNodeData.model]
  );

  const { models: providerModels } = useProviderModelsAll(safeNodeData.model.provider || '', {
    is_enabled: true,
  });
  const { models: availableTextChatModels } = useAvailableModels({
    use_case: 'text-chat',
  });
  const selectedModel = useMemo(
    () => providerModels.find(m => m.model === safeNodeData.model.name),
    [providerModels, safeNodeData.model.name]
  );
  const supportsVision =
    selectedModel?.features.vision === true || selectedModel?.endpoints.vision === true;
  const supportsTextChat = selectedModel?.use_cases?.includes('text-chat') ?? true;

  const updatePromptTemplate = useCallback(
    (idx: number, patch: Partial<LLMNodeData['prompt_template'][number]>) => {
      updateData((prev: LLMNodeData) => {
        const next = (prev.prompt_template || []).slice();
        next[idx] = {
          ...next[idx],
          ...patch,
        };
        return { prompt_template: next };
      });
    },
    [updateData]
  );

  const addPromptBlock = useCallback(() => {
    if (readOnly) return;
    updateData((prev: LLMNodeData) => {
      const currentPromptTemplate = prev.prompt_template || [];
      const nextRole: LLMNodeData['prompt_template'][number]['role'] = (() => {
        for (let i = currentPromptTemplate.length - 1; i >= 0; i--) {
          const r = currentPromptTemplate[i]?.role;
          if (r && r !== 'system') {
            return r === 'user' ? 'assistant' : 'user';
          }
        }
        return 'user';
      })();

      return {
        prompt_template: [
          ...currentPromptTemplate,
          {
            id: generateClientId('prompt'),
            role: nextRole,
            text: '',
          },
        ],
      };
    });
  }, [updateData, readOnly]);

  const removePromptBlock = useCallback(
    (idx: number) => {
      if (readOnly) return;
      updateData((prev: LLMNodeData) => {
        const currentPromptTemplate = prev.prompt_template || [];
        if (idx === 0 && currentPromptTemplate[0]?.role === 'system') return {};
        const next = currentPromptTemplate.filter((_, i) => i !== idx);
        return { prompt_template: next };
      });
    },
    [updateData, readOnly]
  );

  const handleInsert = useCallback(
    (value: VariableInsertValue) => {
      if (readOnly) return;
      const sourceId = value.sourceId;
      const rawKey = value.key;
      const key = sourceId === 'sys' && rawKey.startsWith('sys.') ? rawKey.slice(4) : rawKey;

      const active = activeEditorRef.current;
      if (active) {
        active.insertToken(sourceId, key);
        active.focus();
        return;
      }

      const token = key ? `{{#${sourceId}.${key}#}}` : `{{#${sourceId}#}}`;

      updateData((prev: LLMNodeData) => {
        const currentPromptTemplate = prev.prompt_template || [];
        if (currentPromptTemplate.length === 0) {
          return {
            prompt_template: [{ id: generateClientId('prompt'), role: 'system', text: token }],
          };
        }

        const current = currentPromptTemplate[0]?.text ?? '';
        const needsSpace = current.length > 0 && !/\s$/.test(current);
        const next = needsSpace ? `${current} ${token}` : `${current}${token}`;

        const nextTemplate = currentPromptTemplate.slice();
        nextTemplate[0] = { ...nextTemplate[0], text: next };

        return { prompt_template: nextTemplate };
      });
    },
    [updateData, readOnly]
  );

  const agentType = useWorkflowStore.use.agentType();

  const handleVisionToggle = useCallback(
    (enabled: boolean, skipVisionCheck = false) => {
      if (enabled) {
        if (!skipVisionCheck && !supportsVision && safeNodeData.model.name) {
          return;
        }
        const isChat = agentType === AgentType.CONVERSATIONAL_AGENT;
        const defaultSelector: [string, string] | undefined = isChat ? ['sys', 'files'] : undefined;
        updateData({
          vision: {
            enabled: true,
            configs: {
              detail: 'high',
              variable_selector: defaultSelector as [string, string],
            },
          },
        });
      } else {
        updateData({
          vision: {
            enabled: false,
            configs: safeNodeData.vision.configs,
          },
        });
      }
    },
    [updateData, safeNodeData.vision.configs, agentType, supportsVision, safeNodeData.model.name]
  );

  const handleVisionDetailChange = useCallback(
    (detail: 'high' | 'low') => {
      updateData({
        vision: {
          ...safeNodeData.vision,
          configs: {
            detail,
            variable_selector: safeNodeData.vision.configs?.variable_selector ?? ['', ''],
          },
        },
      });
    },
    [updateData, safeNodeData.vision]
  );

  const handleVisionVariableChange = useCallback(
    (payload: {
      sourceId: string;
      key: string;
      valuePath: string[];
      type: WorkflowVariable['type'];
    }) => {
      if (payload.type !== 'file' && payload.type !== 'array[file]') return;
      updateData({
        vision: {
          ...safeNodeData.vision,
          configs: {
            detail: safeNodeData.vision.configs?.detail ?? 'high',
            variable_selector: payload.valuePath,
          },
        },
      });
    },
    [updateData, safeNodeData.vision]
  );

  const selectedModalMode = useMemo<'text' | 'image' | 'audio'>(() => {
    return safeNodeData.vision.enabled ? 'image' : 'text';
  }, [safeNodeData?.vision?.enabled]);
  const visionVariableSelector = safeNodeData.vision.configs?.variable_selector;

  const outputs = useNodeOutputVariables(nodeId);

  const handleApplyPromptLibraryVersion = useCallback(
    ({ version }: PromptPickerSelection) => {
      if (version.prompt_type === 'text' && typeof version.content === 'string') {
        const textContent = version.content;
        updateData((prev: LLMNodeData) => {
          const nextTemplate: LLMNodeData['prompt_template'] = prev.prompt_template?.length
            ? [...prev.prompt_template]
            : [{ id: generateClientId(), role: 'system' as const, text: '' }];
          nextTemplate[0] = {
            ...(nextTemplate[0] || { id: generateClientId(), role: 'system' as const }),
            role: 'system',
            text: textContent,
          };
          return {
            prompt_template: nextTemplate,
            prompt_source: 'inline',
            prompt_reference: undefined,
          };
        });
        return;
      }

      if (version.prompt_type === 'chat' && Array.isArray(version.content)) {
        updateData({
          prompt_template: version.content.map(message => ({
            id: generateClientId(),
            role: message.role as LLMNodeData['prompt_template'][number]['role'],
            text: message.content,
          })),
          prompt_source: 'inline',
          prompt_reference: undefined,
        });
      }
    },
    [updateData]
  );

  const promptDraftForSave = useMemo<Partial<CreatePromptRequest>>(() => {
    const firstText = selfNodeData?.prompt_template?.[0]?.text ?? '';
    const chatContent = (selfNodeData?.prompt_template || []).map(block => ({
      role: block.role,
      content: block.text,
    }));
    const promptType = selfNodeData?.prompt_template?.length && selfNodeData.prompt_template.length > 1 ? 'chat' : 'text';
    return {
      workspace_id: currentWorkspace?.id ?? '',
      source: 'personal',
      name: selfNodeData?.title ? `${selfNodeData.title} Prompt` : '',
      locale: 'zh-Hans',
      initial_version: {
        prompt_type: promptType,
        content: promptType === 'text' ? firstText : chatContent,
        commit_message: 'Saved from LLM node',
      },
    };
  }, [currentWorkspace?.id, selfNodeData?.prompt_template, selfNodeData?.title]);

  const optimizerInitialPrompt = useMemo(() => {
    const systemBlock = (selfNodeData?.prompt_template || []).find(block => block.role === 'system');
    return systemBlock?.text ?? '';
  }, [selfNodeData?.prompt_template]);

  const playgroundPromptLabel = useMemo(() => {
    if (selfNodeData?.title?.trim()) {
      return `${selfNodeData.title} · ${t('nodes.llm.playground.currentPrompt')}`;
    }
    return t('nodes.llm.playground.currentPrompt');
  }, [selfNodeData?.title, t]);
  const hasExistingPromptContent = useMemo(
    () => (selfNodeData?.prompt_template || []).some(block => block.text?.trim()),
    [selfNodeData?.prompt_template]
  );

  const optimizerInitialModel = useMemo(() => {
    if (!safeNodeData.model.name) {
      return null;
    }
    const matchedModel = availableTextChatModels.find(model => {
      const target = safeNodeData.model.name.trim().toLowerCase();
      return (
        model.model.trim().toLowerCase() === target ||
        (model.model_name || '').trim().toLowerCase() === target
      );
    });
    const inferredProvider = safeNodeData.model.provider || matchedModel?.provider || '';
    const resolvedModelName = matchedModel?.model || safeNodeData.model.name;
    return {
      provider: inferredProvider,
      model: resolvedModelName,
    };
  }, [availableTextChatModels, safeNodeData.model.name, safeNodeData.model.provider]);

  const isManagedPrompt = selfNodeData?.prompt_source === 'managed';
  const roleLabels = useMemo(
    () => ({
      system: t('nodes.llm.roles.system'),
      user: t('nodes.llm.roles.user'),
      assistant: t('nodes.llm.roles.assistant'),
    }),
    [t]
  );

  const releaseLabelText = useCallback(
    (label?: string) => {
      if (!label) return '';
      const normalized = label.toLowerCase();
      if (normalized === 'production') return t('nodes.llm.promptSource.releaseLabels.production');
      if (normalized === 'staging') return t('nodes.llm.promptSource.releaseLabels.staging');
      if (normalized === 'gray-a') return t('nodes.llm.promptSource.releaseLabels.grayA');
      if (normalized === 'gray-b') return t('nodes.llm.promptSource.releaseLabels.grayB');
      return label;
    },
    [t]
  );

  const managedReferenceSummary = useMemo(() => {
    if (!selfNodeData?.prompt_reference) return '';
    if (selfNodeData.prompt_reference.label) {
      return t('nodes.llm.promptSource.followingRelease', {
        target: releaseLabelText(selfNodeData.prompt_reference.label),
      });
    }
    return t('nodes.llm.promptSource.fixedVersion', {
      version: `v${selfNodeData.prompt_reference.version ?? '-'}`,
    });
  }, [releaseLabelText, selfNodeData?.prompt_reference, t]);

  const handleApplyOptimizedPrompt = useCallback(
    async ({
      text,
      variant: _variant,
    }: {
      text: string;
      variant: 'safe' | 'balanced' | 'advanced';
    }) => {
      updateData((prev: LLMNodeData) => {
        const currentPromptTemplate = prev.prompt_template || [];
        const nextTemplate =
          currentPromptTemplate.length > 0
            ? currentPromptTemplate.map((block, index) =>
                index === 0
                  ? {
                      ...block,
                      role: 'system' as const,
                      text,
                    }
                  : block
              )
            : [{ id: generateClientId('prompt'), role: 'system' as const, text }];

        return {
          prompt_template: nextTemplate,
          prompt_source: 'inline',
          prompt_reference: undefined,
        };
      });
    },
    [updateData]
  );

  return (
    <div className={cn('space-y-6', className)}>
      <div className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-1">
            <h3 className="text-base font-semibold flex items-center gap-2">
              {t('nodes.llm.multimodal.title')}
            </h3>
            <Tooltip>
              <TooltipTrigger asChild>
                <Info className="size-3.5 text-muted-foreground cursor-help" />
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-xs">
                <p>{t('nodes.llm.multimodal.tooltip')}</p>
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
        <Tabs
          value={selectedModalMode}
          onValueChange={val => {
            if (val === 'image') {
              if (!supportsVision && safeNodeData.model.name) {
                updateModel({ provider: '', name: '' });
                handleVisionToggle(true, true);
              } else {
                handleVisionToggle(true);
              }
            } else if (val === 'text') {
              if (!supportsTextChat && safeNodeData.model.name) {
                updateModel({ provider: '', name: '' });
              }
              handleVisionToggle(false);
            }
          }}
        >
          <TabsList className="grid w-full grid-cols-3 h-9">
            <Tooltip delayDuration={300}>
              <TabsTrigger value="text" className="h-7" disabled={readOnly} asChild>
                <TooltipTrigger>{t('nodes.llm.multimodal.modes.text')}</TooltipTrigger>
              </TabsTrigger>
              <TooltipContent>
                <p>{t('nodes.llm.multimodal.modeTooltips.text')}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip delayDuration={300}>
              <TabsTrigger value="image" className="h-7" disabled={readOnly} asChild>
                <TooltipTrigger>{t('nodes.llm.multimodal.modes.image')}</TooltipTrigger>
              </TabsTrigger>
              <TooltipContent>
                <p>{t('nodes.llm.multimodal.modeTooltips.image')}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip delayDuration={300}>
              <TabsTrigger value="audio" className="h-7 cursor-not-allowed" disabled asChild>
                <TooltipTrigger>{t('nodes.llm.multimodal.modes.audio')}</TooltipTrigger>
              </TabsTrigger>
              <TooltipContent>
                <p>{t('nodes.llm.multimodal.modeTooltips.audio')}</p>
              </TooltipContent>
            </Tooltip>
          </TabsList>
        </Tabs>
      </div>

      <div>
        <h3 className="text-base font-semibold flex items-center gap-2">
          {t('nodes.llm.section.model')}
        </h3>
        <div className="py-1">
          <div className="flex gap-2 items-center">
            <div className="grow">
              <ModelSelectorParameter
                modelType={selectedModalMode === 'image' ? 'vision' : 'text-chat'}
                value={{
                  provider: safeNodeData.model.provider,
                  model: safeNodeData.model.name,
                  params:
                    (safeNodeData.model.completion_params as Record<
                      string,
                      number | string | boolean
                    >) || {},
                }}
                onChange={v => {
                  updateModel({
                    provider: v.provider,
                    name: v.model,
                    completion_params: v.params as Record<string, string | number | boolean>,
                  });
                }}
                capabilityFilter={
                  selectedModalMode === 'image' ? { features_vision: true } : undefined
                }
                hasError={selectedModalMode === 'image' && !safeNodeData.model.name}
                disabled={readOnly}
              />
            </div>
          </div>
        </div>
      </div>

      {selectedModalMode === 'image' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-1">
              <h3 className="text-sm font-medium text-muted-foreground">
                {t('nodes.llm.section.vision')}
              </h3>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info className="size-3.5 text-muted-foreground cursor-help" />
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs">
                  <p>{t('nodes.llm.tips.visionDescription')}</p>
                </TooltipContent>
              </Tooltip>
            </div>
          </div>
          <div className="space-y-3">
            <div>
              <div className="text-xs font-medium text-muted-foreground mb-1.5 pl-0.5">
                {t('nodes.llm.labels.detail')}
              </div>
              <Tabs
                value={safeNodeData.vision.configs?.detail ?? 'high'}
                onValueChange={val => handleVisionDetailChange(val as 'high' | 'low')}
                className="w-full"
              >
                <TabsList className="grid w-full grid-cols-2 h-9">
                  <TabsTrigger value="high" className="h-7 text-xs" disabled={readOnly}>
                    {t('nodes.llm.labels.high')}
                  </TabsTrigger>
                  <TabsTrigger value="low" className="h-7 text-xs" disabled={readOnly}>
                    {t('nodes.llm.labels.low')}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
            <div>
              <div className="text-xs font-medium text-muted-foreground mb-1.5 pl-0.5">
                {t('nodes.llm.labels.visionVariable')}
              </div>
              <NodeValueSelector
                nodeId={nodeId}
                value={
                  Array.isArray(visionVariableSelector) && visionVariableSelector.length >= 2
                    ? visionVariableSelector
                    : undefined
                }
                onChange={handleVisionVariableChange}
                typeFilter={type => type === 'file' || type === 'array[file]'}
                placeholder={t('nodes.llm.placeholders.selectVisionVar')}
                disabled={readOnly}
              />
            </div>
          </div>
        </div>
      )}

      <div className="space-y-4">
        <div className="space-y-3">
          <h3 className="text-base font-semibold">{t('nodes.llm.section.prompt')}</h3>
          {isManagedPrompt && selfNodeData.prompt_reference ? (
            <div className="rounded-lg border border-primary/20 bg-primary/5 px-4 py-3 space-y-3">
              <div className="space-y-1">
                <div className="text-sm font-medium">
                  {t('nodes.llm.promptSource.currentUsing', {
                    name: selfNodeData.prompt_reference.prompt_name || t('prompts.title'),
                  })}
                </div>
                <div className="text-xs text-muted-foreground">{managedReferenceSummary}</div>
              </div>
              <div className="flex items-center gap-2 flex-wrap">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setOptimizerOpen(true)}
                  disabled={readOnly}
                >
                  <WandSparkles className="h-4 w-4" />
                  {t('nodes.llm.actions.optimizeAsInlineCopy')}
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="sm" isIcon disabled={readOnly} aria-label={t('nodes.llm.actions.more')}>
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onClick={() => setPromptPickerOpen(true)}>
                      {t('nodes.llm.actions.changePromptReference')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() =>
                        updateData({ prompt_source: 'inline', prompt_reference: undefined })
                      }
                    >
                      {t('nodes.llm.actions.useInlineCopy')}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setPlaygroundOpen(true)}>
                      {t('nodes.llm.actions.testCurrentPrompt')}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setManagedPreviewOpen(prev => !prev)}>
                      {managedPreviewOpen
                        ? t('nodes.llm.actions.hidePromptPreview')
                        : t('nodes.llm.actions.showPromptPreview')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          ) : (
            <div className="rounded-lg border bg-muted/20 px-4 py-3">
              <div className="text-sm text-muted-foreground">
                {t('nodes.llm.promptSource.inlineShortDescription')}
              </div>
            </div>
          )}
          <div className="flex items-center gap-2 flex-wrap">
            <Button
              variant="default"
              size="sm"
              onClick={() => setOptimizerOpen(true)}
              disabled={readOnly}
            >
              <WandSparkles className="h-4 w-4" />
              {isManagedPrompt
                ? t('nodes.llm.actions.optimizeAsInlineCopy')
                : t('nodes.llm.actions.optimizePrompt')}
            </Button>
            {!isManagedPrompt && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" isIcon disabled={readOnly} aria-label={t('nodes.llm.actions.more')}>
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => setPromptPickerOpen(true)}>
                    {t('prompts.picker.title')}
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setSavePromptOpen(true)}>
                    {t('nodes.llm.actions.saveAsPrompt')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => setPlaygroundOpen(true)}
                    disabled={!optimizerInitialPrompt.trim()}
                  >
                    {t('nodes.llm.actions.testCurrentPrompt')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </div>
        {isManagedPrompt ? (
          <div className="space-y-2">
            {managedPreviewOpen &&
              (selfNodeData?.prompt_template || []).map((blk, idx) => (
                <div
                  key={blk.id || idx}
                  className="rounded-xl border bg-background overflow-hidden shadow-sm"
                >
                  <div className="flex items-center justify-between border-b bg-muted/50 px-3 py-2">
                    <Badge variant="secondary">
                      {roleLabels[blk.role]}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {t('nodes.llm.promptSource.managedPreview')}
                    </span>
                  </div>
                  <div className="px-4 py-3 text-sm leading-6 whitespace-pre-wrap break-words">
                    {blk.text || t('nodes.llm.promptSource.emptyContent')}
                  </div>
                </div>
              ))}
          </div>
        ) : (
          <>
            <div className="rounded-lg border bg-muted/20 p-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-medium">{t('nodes.llm.variableGuide.title')}</div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setVariableGuideOpen(prev => !prev)}
                >
                  {variableGuideOpen
                    ? t('nodes.llm.actions.hideExplanation')
                    : t('nodes.llm.actions.showExplanation')}
                </Button>
              </div>
              {variableGuideOpen && (
                <div className="mt-3 space-y-3">
                  <div className="grid gap-2 text-xs text-muted-foreground">
                    <div className="flex items-start gap-2">
                      <Badge variant="secondary" className="min-w-5 justify-center px-1.5">
                        1
                      </Badge>
                      <span>{t('nodes.llm.variableGuide.selectBlock')}</span>
                    </div>
                    <div className="flex items-start gap-2">
                      <Badge variant="secondary" className="min-w-5 justify-center px-1.5">
                        2
                      </Badge>
                      <span>{t('nodes.llm.variableGuide.insertMethods')}</span>
                    </div>
                    <div className="flex items-start gap-2">
                      <Badge variant="secondary" className="min-w-5 justify-center px-1.5">
                        3
                      </Badge>
                      <span>{t('nodes.llm.variableGuide.quickVariables')}</span>
                    </div>
                  </div>
                  <div className="rounded-md bg-background/80 px-3 py-2 text-xs text-muted-foreground">
                    <div>{t('nodes.llm.variableGuide.bestPractice')}</div>
                    <div className="mt-1">{t('nodes.llm.variableGuide.fallback')}</div>
                    <div className="mt-1">{t('nodes.llm.variableGuide.noManualSyntax')}</div>
                  </div>
                </div>
              )}
            </div>
            <WorkflowValueInserter
              nodeId={nodeId}
              onInsert={handleInsert}
              disabled={readOnly}
              defaultCollapsed
            />
            <div className="space-y-2">
              {(selfNodeData?.prompt_template || []).map((blk, idx) => (
                <PromptBlockRow
                  key={blk.id || idx}
                  idx={idx}
                  role={blk.role}
                  text={blk.text}
                  nodeId={nodeId}
                  readOnly={readOnly}
                  onUpdate={updatePromptTemplate}
                  onRemove={removePromptBlock}
                  onFocusedEditor={handleEditorFocused}
                  defaultCollapsed={idx > 0}
                />
              ))}
              <Button
                className="w-full"
                variant="outline"
                size="sm"
                onClick={addPromptBlock}
                disabled={readOnly}
              >
                {t('nodes.llm.actions.addBlock')}
              </Button>
            </div>
          </>
        )}
      </div>
      <OutputVariablesView variables={outputs} />
      <PromptPickerDialog
        open={promptPickerOpen}
        onOpenChange={setPromptPickerOpen}
        onApply={handleApplyPromptLibraryVersion}
        applyMode="copy"
        applyLabel={t('nodes.llm.actions.applyPromptTemplate')}
        warnOnReplace={hasExistingPromptContent}
      />
      <PromptFormDialog
        open={savePromptOpen}
        onOpenChange={setSavePromptOpen}
        initialDraft={promptDraftForSave}
        onSubmit={async payload => {
          await createPrompt.mutateAsync(payload);
        }}
      />
      <PromptOptimizerDialog
        open={optimizerOpen}
        onOpenChange={setOptimizerOpen}
        initialPrompt={optimizerInitialPrompt}
        sourceLabel={t('nodes.llm.optimizer.sourceLabel')}
        sourceHelpText={t('nodes.llm.optimizer.sourceHelp')}
        sourceResetLabel={t('nodes.llm.optimizer.resetSource')}
        initialModel={optimizerInitialModel}
        applyLabel={t('nodes.llm.actions.applyOptimizedPrompt')}
        onApplyResult={handleApplyOptimizedPrompt}
      />
      <PromptPlaygroundDialog
        open={playgroundOpen}
        onOpenChange={setPlaygroundOpen}
        prefillPromptMessages={(selfNodeData?.prompt_template || []).map(block => ({
          role: block.role,
          content: block.text,
        }))}
        prefillPromptLabel={playgroundPromptLabel}
        prefillModel={optimizerInitialModel}
      />
    </div>
  );
};

export default React.memo(LLMManager);
