'use client';

import React, { useCallback, useMemo, useEffect, useState, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Slider } from '@/components/ui/slider';
import { Switch } from '@/components/ui/switch';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import type { LLMNodeData, LLMPromptLayoutItem } from '../config';
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
import { Braces, Trash2, Info, WandSparkles, MoreHorizontal, GripVertical, ArrowUpDown } from 'lucide-react';
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
import {
  DndContext,
  PointerSensor,
  KeyboardSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

interface LLMManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

interface PromptBlockRowProps {
  idx: number;
  role: LLMNodeData['prompt_template'][number]['role'];
  text: string;
  title?: string;
  nodeId: string;
  readOnly: boolean;
  onUpdate: (idx: number, patch: Partial<LLMNodeData['prompt_template'][number]>) => void;
  onRemove: (idx: number) => void;
  // Bubble active editor handle up so parent can route inserts to caret
  onFocusedEditor: (idx: number, handle: WorkflowValueEditorHandle) => void;
}

// Prompt block row with local state and debounced text update to reduce global store writes
const PromptBlockRowComponent: React.FC<PromptBlockRowProps> = ({
  idx,
  role,
  text,
  title,
  nodeId,
  readOnly,
  onUpdate,
  onRemove,
  onFocusedEditor,
}) => {
  // Keep local role/text for smooth typing; sync when parent changes
  const [localRole, setLocalRole] = useState(role);
  const [localText, setLocalText] = useState(text);
  const t = useT();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingTemplate, setPendingTemplate] = useState<string | null>(null);

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

  // System prompts are fixed at the top of the final chat prompt.
  const isFirstSystem = idx === 0 && localRole === 'system';
  const isSystemPrompt = localRole === 'system';

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

  return (
    <div className="theme-surface border border-muted rounded-xl overflow-hidden shadow-sm hover:border-primary/30 transition-all duration-300">
      {title ? (
        <div className="border-b bg-muted/30 px-3 py-2 text-xs font-medium text-muted-foreground">
          {title}
        </div>
      ) : null}
      <PromptEditor
        ref={editorRef}
        role={localRole}
        value={localText}
        nodeId={nodeId}
        onChangeRole={nextRole => {
          if (readOnly || isSystemPrompt) return; // locked
          setLocalRole(nextRole);
          onUpdate(idx, { role: nextRole });
        }}
        onChange={val => setLocalText(val)}
        readOnly={readOnly}
        roleLocked={isSystemPrompt}
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

            {!isSystemPrompt && (
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
                      disabled={readOnly || isSystemPrompt}
                      title={
                        isSystemPrompt ? t('nodes.llm.tips.cannotRemoveFirstSystem') : undefined
                      }
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  {isSystemPrompt
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

type PromptBlock = LLMNodeData['prompt_template'][number];

type PromptOrderViewItem =
  | { type: 'history'; id: 'conversation_history'; key: string }
  | {
      type: 'group';
      key: string;
      groupId: string;
      groupKind: NonNullable<PromptBlock['group_kind']>;
      blocks: Array<{ block: PromptBlock; index: number }>;
    };

const isCurrentUserBlock = (block: PromptBlock) =>
  block.group_kind === 'current_user' ||
  (block.role === 'user' && block.text.includes('#sys.query#'));

const isConversationHistoryEnabled = (data?: Partial<LLMNodeData>) =>
  data?.conversation_history?.enabled === true;

const buildLegacyPromptGroups = (
  blocks: PromptBlock[]
): Array<Omit<Extract<PromptOrderViewItem, { type: 'group' }>, 'key'>> => {
  const nonSystem = blocks
    .map((block, index) => ({ block, index }))
    .filter(item => item.block.role !== 'system');
  if (nonSystem.length === 0) return [];

  const currentUserIndex = (() => {
    for (let i = nonSystem.length - 1; i >= 0; i--) {
      if (isCurrentUserBlock(nonSystem[i].block)) return nonSystem[i].index;
    }
    for (let i = nonSystem.length - 1; i >= 0; i--) {
      if (nonSystem[i].block.role === 'user') return nonSystem[i].index;
    }
    return -1;
  })();

  const groups: Array<Omit<Extract<PromptOrderViewItem, { type: 'group' }>, 'key'>> = [];
  const legacyBlocks = nonSystem.filter(item => item.index !== currentUserIndex);
  if (legacyBlocks.length > 0) {
    groups.push({
      type: 'group',
      groupId: legacyBlocks[0]?.block.group_id || `legacy-${legacyBlocks[0]?.index ?? 0}`,
      groupKind: 'legacy_context',
      blocks: legacyBlocks,
    });
  }

  if (currentUserIndex >= 0) {
    const current = nonSystem.find(item => item.index === currentUserIndex);
    if (current) {
      groups.push({
        type: 'group',
        groupId: current.block.group_id || 'current-user',
        groupKind: 'current_user',
        blocks: [current],
      });
    }
  }

  return groups;
};

const buildPromptOrderView = (data?: Partial<LLMNodeData>): PromptOrderViewItem[] => {
  const blocks = data?.prompt_template || [];
  const historyEnabled = isConversationHistoryEnabled(data);
  const promptLayout = data?.prompt_layout;

  if (!promptLayout) {
    const items: PromptOrderViewItem[] = [];
    if (historyEnabled) {
      items.push({ type: 'history', id: 'conversation_history', key: 'history' });
    }
    buildLegacyPromptGroups(blocks).forEach(group => {
      items.push({ ...group, key: `group:${group.groupId}` });
    });
    return items;
  }

  const groups = new Map<string, Extract<PromptOrderViewItem, { type: 'group' }>>();
  blocks.forEach((block, index) => {
    if (block.role === 'system' || !block.group_id) return;
    const existing = groups.get(block.group_id);
    if (existing) {
      existing.blocks.push({ block, index });
      return;
    }
    groups.set(block.group_id, {
      type: 'group',
      key: `group:${block.group_id}`,
      groupId: block.group_id,
      groupKind: block.group_kind || 'legacy_context',
      blocks: [{ block, index }],
    });
  });

  const items: PromptOrderViewItem[] = [];
  let hasHistoryItem = false;
  promptLayout.items.forEach(item => {
    if (item.type === 'history') {
      hasHistoryItem = true;
      if (historyEnabled) {
        items.push({ type: 'history', id: 'conversation_history', key: 'history' });
      }
      return;
    }
    const group = groups.get(item.group_id);
    if (group) items.push(group);
  });
  if (historyEnabled && !hasHistoryItem) {
    items.unshift({ type: 'history', id: 'conversation_history', key: 'history' });
  }
  return items;
};

const materializePromptLayout = (data: LLMNodeData): Pick<LLMNodeData, 'prompt_template' | 'prompt_layout'> => {
  const blocks = (data.prompt_template || []).map(block => ({
    ...block,
    id: block.id || generateClientId('prompt'),
  }));
  const viewItems = buildPromptOrderView({ ...data, prompt_template: blocks });
  const nextBlocks = blocks.map(block => ({ ...block }));

  viewItems.forEach(item => {
    if (item.type !== 'group') return;
    item.blocks.forEach(({ index }) => {
      nextBlocks[index] = {
        ...nextBlocks[index],
        group_id: item.groupId,
        group_kind: item.groupKind,
      };
    });
  });

  const visibleLayoutItems: LLMPromptLayoutItem[] = viewItems.map(item =>
    item.type === 'history'
      ? { type: 'history', id: 'conversation_history' }
      : { type: 'group', group_id: item.groupId }
  );
  const layoutItems: LLMPromptLayoutItem[] = [];
  if (data.prompt_layout) {
    const visibleByKey = new Map(
      visibleLayoutItems.map(item => [
        item.type === 'history' ? 'history' : `group:${item.group_id}`,
        item,
      ])
    );
    data.prompt_layout.items.forEach(item => {
      const key = item.type === 'history' ? 'history' : `group:${item.group_id}`;
      const visibleItem = visibleByKey.get(key);
      if (visibleItem) {
        layoutItems.push(visibleItem);
        visibleByKey.delete(key);
        return;
      }
      if (item.type === 'history') {
        layoutItems.push(item);
      }
    });
    visibleByKey.forEach(item => layoutItems.push(item));
  } else {
    layoutItems.push(...visibleLayoutItems);
  }

  return {
    prompt_template: nextBlocks,
    prompt_layout: {
      version: 1,
      items: layoutItems,
    },
  };
};

interface SortablePromptOrderRowProps {
  id: string;
  label: string;
  description: string;
}

const SortablePromptOrderRow: React.FC<SortablePromptOrderRowProps> = ({
  id,
  label,
  description,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  });

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        'flex items-center gap-3 rounded-lg border bg-background px-3 py-2 shadow-sm',
        isDragging && 'opacity-70'
      )}
    >
      <Button
        type="button"
        variant="ghost"
        size="sm"
        isIcon
        className="cursor-grab"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-4 w-4" />
      </Button>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{label}</div>
        <div className="truncate text-xs text-muted-foreground">{description}</div>
      </div>
    </div>
  );
};

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
  const [orderDialogOpen, setOrderDialogOpen] = useState(false);
  const [draftOrderItems, setDraftOrderItems] = useState<LLMPromptLayoutItem[]>([]);
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
      conversation_history: selfNodeData?.conversation_history ?? {
        enabled: false,
        history_window_size: 3,
      },
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

  const clampConversationHistoryWindow = useCallback((value: number) => {
    const fallback = 3;
    const n = Number.isFinite(value) ? Math.round(value) : fallback;
    return Math.max(1, Math.min(50, n));
  }, []);

  const handleConversationHistoryToggle = useCallback(
    (enabled: boolean) => {
      if (readOnly) return;
      updateData((prev: LLMNodeData) => {
        const next: Partial<LLMNodeData> = {
          conversation_history: {
            enabled,
            history_window_size: clampConversationHistoryWindow(
              safeNodeData.conversation_history?.history_window_size ?? 3
            ),
          },
        };
        if (
          enabled &&
          prev.prompt_layout &&
          !prev.prompt_layout.items.some(item => item.type === 'history')
        ) {
          next.prompt_layout = {
            version: 1,
            items: [
              { type: 'history', id: 'conversation_history' },
              ...prev.prompt_layout.items,
            ],
          };
        }
        return next;
      });
    },
    [
      updateData,
      readOnly,
      safeNodeData.conversation_history?.history_window_size,
      clampConversationHistoryWindow,
    ]
  );

  const handleConversationHistoryWindowChange = useCallback(
    (value: number) => {
      if (readOnly) return;
      updateData({
        conversation_history: {
          enabled: safeNodeData.conversation_history?.enabled ?? true,
          history_window_size: clampConversationHistoryWindow(value),
        },
      });
    },
    [
      updateData,
      readOnly,
      safeNodeData.conversation_history?.enabled,
      clampConversationHistoryWindow,
    ]
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
  const isManagedPrompt = selfNodeData?.prompt_source === 'managed';
  const isChatMode = safeNodeData.model.mode === 'chat';

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

  const promptOrderViewItems = useMemo(
    () => (isChatMode ? buildPromptOrderView(safeNodeData) : []),
    [isChatMode, safeNodeData]
  );
  const systemPromptBlocks = useMemo(
    () =>
      (isChatMode ? safeNodeData.prompt_template || [] : [])
        .map((block, index) => ({ block, index }))
        .filter(item => item.block.role === 'system'),
    [isChatMode, safeNodeData.prompt_template]
  );
  const hasCurrentUserPrompt = useMemo(
    () =>
      promptOrderViewItems.some(
        item => item.type === 'group' && item.groupKind === 'current_user'
      ),
    [promptOrderViewItems]
  );
  const shouldShowMissingUserQuestionTip =
    isChatMode &&
    isConversationHistoryEnabled(safeNodeData) &&
    !hasCurrentUserPrompt;
  const sortablePromptOrderItems = useMemo(
    () =>
      promptOrderViewItems.map(item =>
        item.type === 'history'
          ? ({ type: 'history', id: 'conversation_history' } as LLMPromptLayoutItem)
          : ({ type: 'group', group_id: item.groupId } as LLMPromptLayoutItem)
      ),
    [promptOrderViewItems]
  );
  const canChangePromptOrder =
    isChatMode &&
    !isManagedPrompt &&
    !readOnly &&
    sortablePromptOrderItems.length >= 2;

  const addPromptBlock = useCallback(() => {
    if (readOnly) return;
    updateData((prev: LLMNodeData) => {
      if (prev.model?.mode === 'completion') {
        return {
          prompt_template: [
            ...(prev.prompt_template || []),
            { id: generateClientId('prompt'), role: 'user', text: '' },
          ],
        };
      }

      const materialized = materializePromptLayout(prev);
      const currentPromptTemplate = materialized.prompt_template || [];
      const existingCurrentUser = currentPromptTemplate.some(
        block => block.role !== 'system' && isCurrentUserBlock(block)
      );

      const groupId = generateClientId(existingCurrentUser ? 'context-group' : 'current-user');
      const newBlocks: LLMNodeData['prompt_template'] = existingCurrentUser
        ? [
            {
              id: generateClientId('prompt'),
              role: 'user',
              text: '',
              group_id: groupId,
              group_kind: 'custom_context',
            },
            {
              id: generateClientId('prompt'),
              role: 'assistant',
              text: '',
              group_id: groupId,
              group_kind: 'custom_context',
            },
          ]
        : [
            {
              id: generateClientId('prompt'),
              role: 'user',
              text: '{{#sys.query#}}',
              group_id: groupId,
              group_kind: 'current_user',
            },
          ];

      return {
        ...materialized,
        prompt_template: [
          ...currentPromptTemplate,
          ...newBlocks,
        ],
        prompt_layout: {
          version: 1,
          items: [
            ...(materialized.prompt_layout?.items || []),
            { type: 'group', group_id: groupId },
          ],
        },
      };
    });
  }, [updateData, readOnly]);

  const removePromptBlock = useCallback(
    (idx: number) => {
      if (readOnly) return;
      updateData((prev: LLMNodeData) => {
        const materialized = materializePromptLayout(prev);
        const currentPromptTemplate = materialized.prompt_template || [];
        if (idx === 0 && currentPromptTemplate[0]?.role === 'system') return {};
        const removedGroupID = currentPromptTemplate[idx]?.group_id;
        const next = currentPromptTemplate.filter((_, i) => i !== idx);
        const remainingGroupIDs = new Set(
          next.map(block => block.group_id).filter((groupID): groupID is string => !!groupID)
        );
        const nextLayoutItems = (materialized.prompt_layout?.items || []).filter(item => {
          if (item.type !== 'group') return true;
          return item.group_id !== removedGroupID || remainingGroupIDs.has(item.group_id);
        });
        return {
          prompt_template: next,
          prompt_layout: {
            version: 1,
            items: nextLayoutItems,
          },
        };
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
        const chatContent = version.content;
        updateData((prev: LLMNodeData) => {
          const promptTemplate = chatContent.map(message => ({
            id: generateClientId(),
            role: message.role as LLMNodeData['prompt_template'][number]['role'],
            text: message.content,
          }));
          const materialized = materializePromptLayout({
            ...prev,
            prompt_template: promptTemplate,
          });
          return {
            ...materialized,
            prompt_source: 'inline',
            prompt_reference: undefined,
          };
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

  const roleLabels = useMemo(
    () => ({
      system: t('nodes.llm.roles.system'),
      user: t('nodes.llm.roles.user'),
      assistant: t('nodes.llm.roles.assistant'),
    }),
    [t]
  );
  const promptOrderItemLabels = useMemo(() => {
    const labels = new Map<string, { label: string; description: string }>();
    promptOrderViewItems.forEach(item => {
      if (item.type === 'history') {
        labels.set('history', {
          label: t('nodes.llm.promptOrder.history'),
          description: t('nodes.llm.promptOrder.historyDescription'),
        });
        return;
      }
      const label =
        item.groupKind === 'current_user'
          ? t('nodes.llm.promptOrder.currentUser')
          : item.groupKind === 'custom_context'
            ? t('nodes.llm.promptOrder.customContext')
            : t('nodes.llm.promptOrder.legacyContext');
      labels.set(`group:${item.groupId}`, {
        label,
        description: item.blocks
          .map(({ block }) => roleLabels[block.role])
          .join(' + '),
      });
    });
    return labels;
  }, [promptOrderViewItems, roleLabels, t]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  useEffect(() => {
    if (!orderDialogOpen) return;
    setDraftOrderItems(sortablePromptOrderItems);
  }, [orderDialogOpen, sortablePromptOrderItems]);

  const draftOrderIDs = useMemo(
    () =>
      draftOrderItems.map(item =>
        item.type === 'history' ? 'history' : `group:${item.group_id}`
      ),
    [draftOrderItems]
  );

  const handlePromptOrderDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    setDraftOrderItems(items => {
      const oldIndex = items.findIndex(item =>
        item.type === 'history' ? active.id === 'history' : active.id === `group:${item.group_id}`
      );
      const newIndex = items.findIndex(item =>
        item.type === 'history' ? over.id === 'history' : over.id === `group:${item.group_id}`
      );
      if (oldIndex < 0 || newIndex < 0) return items;
      return arrayMove(items, oldIndex, newIndex);
    });
  }, []);

  const handleSavePromptOrder = useCallback(() => {
    if (readOnly) return;
    updateData((prev: LLMNodeData) => {
      const materialized = materializePromptLayout(prev);
      return {
        ...materialized,
        prompt_layout: {
          version: 1,
          items: draftOrderItems,
        },
      };
    });
    setOrderDialogOpen(false);
  }, [draftOrderItems, readOnly, updateData]);

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

      <div className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-1">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('nodes.llm.section.conversationHistory')}
            </h3>
            <Tooltip>
              <TooltipTrigger asChild>
                <Info className="size-3.5 text-muted-foreground cursor-help" />
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-xs">
                <p>{t('nodes.llm.tips.conversationHistoryDescription')}</p>
              </TooltipContent>
            </Tooltip>
          </div>
          <Switch
            checked={safeNodeData.conversation_history?.enabled ?? false}
            onCheckedChange={handleConversationHistoryToggle}
            disabled={readOnly}
          />
        </div>
        {safeNodeData.conversation_history?.enabled ? (
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <div className="text-xs font-medium text-muted-foreground">
                {t('nodes.llm.labels.conversationHistoryRounds')}
              </div>
              <div className="text-xs text-muted-foreground">
                {String(safeNodeData.conversation_history?.history_window_size ?? 3)}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Slider
                min={1}
                max={50}
                step={1}
                value={[safeNodeData.conversation_history?.history_window_size ?? 3]}
                onValueChange={vals =>
                  handleConversationHistoryWindowChange(
                    Array.isArray(vals) && typeof vals[0] === 'number' ? vals[0] : 3
                  )
                }
                disabled={readOnly}
                className="flex-1"
              />
              <Input
                type="number"
                inputMode="numeric"
                min={1}
                max={50}
                step={1}
                className="h-8 w-20 text-center"
                value={String(safeNodeData.conversation_history?.history_window_size ?? 3)}
                onChange={e =>
                  handleConversationHistoryWindowChange(e.currentTarget.valueAsNumber)
                }
                disabled={readOnly}
              />
            </div>
          </div>
        ) : null}
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
            />
            {isChatMode ? (
              <div className="space-y-2">
              {systemPromptBlocks.map(({ block: blk, index: idx }) => (
                <PromptBlockRow
                  key={blk.id || idx}
                  idx={idx}
                  role={blk.role}
                  text={blk.text}
                  title={t('nodes.llm.promptOrder.systemPrompt')}
                  nodeId={nodeId}
                  readOnly={readOnly}
                  onUpdate={updatePromptTemplate}
                  onRemove={removePromptBlock}
                  onFocusedEditor={handleEditorFocused}
                />
              ))}
              {shouldShowMissingUserQuestionTip && (
                <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
                  <Info className="mt-0.5 h-4 w-4 shrink-0" />
                  <div>
                    <div className="font-medium">
                      {t('nodes.llm.promptOrder.missingUserQuestionTitle')}
                    </div>
                    <div className="mt-0.5">
                      {t('nodes.llm.promptOrder.missingUserQuestionDescription')}
                    </div>
                  </div>
                </div>
              )}
              {promptOrderViewItems.map((item, orderIndex) => {
                if (item.type === 'history') {
                  return (
                    <div
                      key={item.key}
                      className="rounded-xl border border-dashed bg-muted/20 px-4 py-3"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <div className="text-sm font-medium">
                            {t('nodes.llm.promptOrder.history')}
                          </div>
                          <div className="mt-1 text-xs text-muted-foreground">
                            {t('nodes.llm.promptOrder.historyDescription')}
                          </div>
                        </div>
                        <Badge variant="secondary">
                          {t('nodes.llm.promptOrder.positionLabel', {
                            index: orderIndex + 1,
                          })}
                        </Badge>
                      </div>
                    </div>
                  );
                }

                const groupTitle =
                  item.groupKind === 'current_user'
                    ? t('nodes.llm.promptOrder.currentUser')
                    : item.groupKind === 'custom_context'
                      ? t('nodes.llm.promptOrder.customContext')
                      : t('nodes.llm.promptOrder.legacyContext');

                return (
                  <div key={item.key} className="space-y-2">
                    {item.blocks.map(({ block: blk, index: idx }, blockIndex) => (
                      <PromptBlockRow
                        key={blk.id || `${item.groupId}-${idx}`}
                        idx={idx}
                        role={blk.role}
                        text={blk.text}
                        title={
                          blockIndex === 0
                            ? `${groupTitle} - ${t('nodes.llm.promptOrder.positionLabel', {
                                index: orderIndex + 1,
                              })}`
                            : undefined
                        }
                        nodeId={nodeId}
                        readOnly={readOnly}
                        onUpdate={updatePromptTemplate}
                        onRemove={removePromptBlock}
                        onFocusedEditor={handleEditorFocused}
                      />
                    ))}
                  </div>
                );
              })}
              <div className="grid gap-2">
                {canChangePromptOrder && (
                  <Button
                    className="w-full"
                    variant="outline"
                    size="sm"
                    onClick={() => setOrderDialogOpen(true)}
                    disabled={readOnly}
                  >
                    <ArrowUpDown className="mr-2 h-4 w-4" />
                    {t('nodes.llm.actions.changePromptOrder')}
                  </Button>
                )}
                <Button
                  className="w-full"
                  variant="outline"
                  size="sm"
                  onClick={addPromptBlock}
                  disabled={readOnly}
                >
                  {hasCurrentUserPrompt
                    ? t('nodes.llm.actions.addCustomContext')
                  : t('nodes.llm.actions.addUserQuestion')}
                </Button>
              </div>
              </div>
            ) : (
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
            )}
          </>
        )}
      </div>
      <Dialog open={orderDialogOpen} onOpenChange={setOrderDialogOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('nodes.llm.promptOrder.orderDialogTitle')}</DialogTitle>
            <DialogDescription>
              {t('nodes.llm.promptOrder.orderDialogDescription')}
            </DialogDescription>
          </DialogHeader>
          <DialogBody>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handlePromptOrderDragEnd}
            >
              <SortableContext items={draftOrderIDs} strategy={verticalListSortingStrategy}>
                <div className="space-y-2">
                  {draftOrderItems.map(item => {
                    const id = item.type === 'history' ? 'history' : `group:${item.group_id}`;
                    const labelInfo = promptOrderItemLabels.get(id);
                    return (
                      <SortablePromptOrderRow
                        key={id}
                        id={id}
                        label={labelInfo?.label || id}
                        description={labelInfo?.description || ''}
                      />
                    );
                  })}
                </div>
              </SortableContext>
            </DndContext>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOrderDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleSavePromptOrder}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
