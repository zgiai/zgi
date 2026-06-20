'use client';

import { useMemo } from 'react';
import {
  sanitizeAIChatContextText,
  usePageContextRegistration,
  type AIChatCapabilityDescriptor,
  type AIChatContextRelation,
  type AIChatPageContextItem,
} from '@/components/aichat/page-context';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import type {
  AgentDatabaseBinding,
  AgentDetail,
  AgentMemorySlotConfig,
  AgentWorkflowBinding,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { Dataset } from '@/services/types/dataset';
import { AGENT_KEYS } from '@/hooks/query-keys';
import type { AgentRuntimeSaveState } from './types';

const PROMPT_SUMMARY_MAX_LENGTH = 1200;
const CONTEXT_FIELD_MAX_LENGTH = 500;

interface AgentRuntimeAIChatWorkspaceContext {
  id: string;
  name: string;
}

interface AgentRuntimeAIChatPromptContext {
  excerpt: string;
  characterCount: number;
  isTruncated: boolean;
  isTooLong: boolean;
}

interface AgentRuntimeAIChatSkillContext {
  id: string;
  name: string;
  description: string;
  source?: string;
  runtimeType?: string;
}

interface AgentRuntimeAIChatKnowledgeContext {
  id: string;
  name: string;
  description: string;
  loadError: boolean;
}

interface AgentRuntimeAIChatMemoryContext {
  enabled: boolean;
  slots: Array<Pick<AgentMemorySlotConfig, 'key' | 'description' | 'enabled' | 'max_chars'>>;
}

interface AgentRuntimeAIChatPublishContext {
  isPublished: boolean;
  webAppStatus?: AgentDetail['web_app_status'];
  webAppUrl: string;
  saveState: AgentRuntimeSaveState;
  isDirty: boolean;
  isVersionPreviewing: boolean;
}

interface AgentRuntimeAIChatPermissionContext {
  codes: string[];
  canManageAgent: boolean;
  canEditAgent: boolean;
  organizationRole: string | null;
  workspaceRole: string | null;
  workspaceRoleName: string | null;
}

export interface AgentRuntimeAIChatContext {
  id: string;
  source: 'agent-runtime';
  agent: {
    id: string;
    name: string;
    description: string;
    type: string;
    workspace: AgentRuntimeAIChatWorkspaceContext | null;
  };
  configuration: {
    prompt: AgentRuntimeAIChatPromptContext;
    model: {
      provider: string;
      name: string;
      parameters: Record<string, unknown>;
    };
    skills: AgentRuntimeAIChatSkillContext[];
    knowledge: AgentRuntimeAIChatKnowledgeContext[];
    memory: AgentRuntimeAIChatMemoryContext;
    fileUploadEnabled: boolean;
    suggestedQuestions: string[];
    databaseBindings: AgentDatabaseBinding[];
    workflowBindings: AgentWorkflowBinding[];
  };
  publish: AgentRuntimeAIChatPublishContext;
  permissions: AgentRuntimeAIChatPermissionContext;
}

interface BuildAgentRuntimeAIChatContextParams {
  agent: AgentDetail | undefined;
  locale: string;
  payload: UpdateAgentRuntimeConfigRequest;
  promptCharacterCount: number;
  isPromptTooLong: boolean;
  selectedSkills: AIChatSkillMetadata[];
  selectedKnowledgeDatasets: Array<Dataset & { load_error?: boolean }>;
  permissions: readonly string[];
  organizationRole: string | null;
  workspaceRole: string | null;
  workspaceRoleName: string | null;
  canManageAgent: boolean;
  saveState: AgentRuntimeSaveState;
  isDirty: boolean;
  isVersionPreviewing: boolean;
  webAppUrl: string;
}

interface AgentRuntimeAIChatContextRegistrationProps {
  context: AgentRuntimeAIChatContext | null;
}

function summarizePrompt(prompt: string, characterCount: number, isTooLong: boolean) {
  const normalized = sanitizeAIChatContextText(prompt).replace(/\s+/g, ' ').trim();
  const excerpt =
    normalized.length > PROMPT_SUMMARY_MAX_LENGTH
      ? normalized.slice(0, PROMPT_SUMMARY_MAX_LENGTH).trim()
      : normalized;

  return {
    excerpt,
    characterCount,
    isTruncated: normalized.length > excerpt.length,
    isTooLong,
  };
}

function compactContextField(value: string, maxLength = CONTEXT_FIELD_MAX_LENGTH): string {
  const text = sanitizeAIChatContextText(value).replace(/\s+/g, ' ').trim();
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength).trim()}...`;
}

function summarizeNames(values: string[], emptyLabel = 'none'): string {
  const names = values.map(value => value.trim()).filter(Boolean);
  if (names.length === 0) return emptyLabel;
  return compactContextField(names.join(', '));
}

function workflowBindingLabel(binding: AgentWorkflowBinding, index: number): string {
  return compactContextField(binding.label || `Workflow binding ${index + 1}`, 120);
}

function databaseBindingLabel(binding: AgentDatabaseBinding, index: number): string {
  const tableSummary = `${binding.table_ids.length} tables${
    (binding.writable_table_ids ?? []).length > 0 ? ', writable' : ''
  }`;
  return `Database binding ${index + 1} (${tableSummary})`;
}

function formatBooleanState(value: boolean): string {
  return value ? 'enabled' : 'disabled';
}

function capabilityStatus(isAvailable: boolean): AIChatCapabilityDescriptor['status'] {
  return isAvailable ? 'available' : 'unavailable';
}

function buildAgentWorkflowRelations(
  context: AgentRuntimeAIChatContext
): AIChatContextRelation[] | undefined {
  const relations = context.configuration.workflowBindings
    .filter(binding => binding.workflow_id)
    .map((binding, index) => ({
      type: 'binds_workflow',
      resourceType: 'workflow' as const,
      resourceId: binding.workflow_id,
      title: workflowBindingLabel(binding, index),
      metadata: {
        binding_id: binding.binding_id,
        version_strategy: binding.version_strategy,
        version_uuid: binding.version_uuid,
      },
    }));

  return relations.length > 0 ? relations : undefined;
}

function buildAgentRuntimeCapabilities(
  context: AgentRuntimeAIChatContext
): AIChatCapabilityDescriptor[] {
  const canEdit = context.permissions.canEditAgent;
  const canManage = context.permissions.canManageAgent;

  return [
    {
      id: 'inspect_agent_runtime',
      title: 'Inspect agent runtime',
      description: 'Read the Agent Runtime configuration currently visible in the console.',
      risk: 'low',
      status: 'available',
    },
    {
      id: 'update_agent_runtime_config',
      title: 'Update agent runtime config',
      description: 'Prepare or apply Agent Runtime configuration changes.',
      risk: 'medium',
      requiresConfirmation: true,
      status: capabilityStatus(canEdit),
      permissions: context.permissions.codes,
      metadata: {
        can_edit_agent: canEdit,
        is_dirty: context.publish.isDirty,
        save_state: context.publish.saveState,
      },
    },
    {
      id: 'publish_agent',
      title: 'Publish agent',
      description: 'Publish the Agent web app or promote the current runtime configuration.',
      risk: 'high',
      requiresConfirmation: true,
      status: capabilityStatus(canManage),
      permissions: context.permissions.codes,
      metadata: {
        can_manage_agent: canManage,
        is_published: context.publish.isPublished,
        web_app_status: context.publish.webAppStatus,
      },
    },
    {
      id: 'invoke_agent',
      title: 'Invoke agent',
      description: 'Run the Agent with the current runtime configuration.',
      risk: 'low',
      status: 'available',
      metadata: {
        file_upload_enabled: context.configuration.fileUploadEnabled,
        memory_enabled: context.configuration.memory.enabled,
      },
    },
    {
      id: 'bind_workflow_to_agent',
      title: 'Bind workflow to agent',
      description: 'Add or update workflow bindings for this Agent.',
      risk: 'medium',
      requiresConfirmation: true,
      status: capabilityStatus(canEdit),
      permissions: context.permissions.codes,
      metadata: {
        can_edit_agent: canEdit,
        workflow_binding_count: context.configuration.workflowBindings.length,
      },
    },
  ];
}

function buildWorkflowBindingCapabilities(
  binding: AgentWorkflowBinding
): AIChatCapabilityDescriptor[] {
  return [
    {
      id: 'inspect_workflow',
      title: 'Inspect workflow',
      description: 'Read workflow binding metadata and selected version strategy.',
      risk: 'low',
      status: 'available',
      metadata: {
        binding_id: binding.binding_id,
        version_strategy: binding.version_strategy,
        version_uuid: binding.version_uuid,
      },
    },
    {
      id: 'run_workflow',
      title: 'Run workflow',
      description: 'Invoke this bound workflow through the Agent Runtime.',
      risk: 'medium',
      requiresConfirmation: true,
      status: binding.workflow_id ? 'available' : 'unavailable',
      metadata: {
        binding_id: binding.binding_id,
        timeout_seconds: binding.timeout_seconds,
      },
    },
  ];
}

export function buildAgentRuntimeAIChatContext({
  agent,
  locale,
  payload,
  promptCharacterCount,
  isPromptTooLong,
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
}: BuildAgentRuntimeAIChatContextParams): AgentRuntimeAIChatContext | null {
  if (!agent) return null;

  return {
    id: `agent-runtime:${agent.id}`,
    source: 'agent-runtime',
    agent: {
      id: agent.id,
      name: agent.name,
      description: agent.description,
      type: agent.agent_type,
      workspace: agent.workspace
        ? {
            id: agent.workspace.id,
            name: agent.workspace.name,
          }
        : null,
    },
    configuration: {
      prompt: summarizePrompt(payload.system_prompt, promptCharacterCount, isPromptTooLong),
      model: {
        provider: payload.model_provider,
        name: payload.model,
        parameters: payload.model_parameters,
      },
      skills: selectedSkills.map(skill => {
        const display = getAIChatSkillDisplayInfo(skill, locale);
        return {
          id: skill.skill_id,
          name: display.label,
          description: display.description || skill.description || '',
          source: skill.source,
          runtimeType: skill.runtime_type,
        };
      }),
      knowledge: selectedKnowledgeDatasets.map(dataset => ({
        id: dataset.id,
        name: dataset.name,
        description: dataset.description ?? '',
        loadError: Boolean(dataset.load_error),
      })),
      memory: {
        enabled: Boolean(payload.agent_memory_enabled),
        slots: (payload.agent_memory_slots ?? []).map(slot => ({
          key: slot.key,
          description: slot.description,
          enabled: slot.enabled,
          max_chars: slot.max_chars,
        })),
      },
      fileUploadEnabled: payload.file_upload_enabled,
      suggestedQuestions: payload.suggested_questions,
      databaseBindings: payload.database_bindings ?? [],
      workflowBindings: payload.workflow_bindings ?? [],
    },
    publish: {
      isPublished: Boolean(agent.is_published),
      webAppStatus: agent.web_app_status,
      webAppUrl,
      saveState,
      isDirty,
      isVersionPreviewing,
    },
    permissions: {
      codes: Array.from(new Set(permissions)).sort(),
      canManageAgent,
      canEditAgent: agent.can_edit,
      organizationRole,
      workspaceRole,
      workspaceRoleName,
    },
  };
}

function buildAgentRuntimeAIChatContextItems(
  context: AgentRuntimeAIChatContext | null
): AIChatPageContextItem[] {
  if (!context) return [];

  const workspaceLabel = context.agent.workspace?.name ?? 'no workspace';
  const modelLabel = [context.configuration.model.provider, context.configuration.model.name]
    .filter(Boolean)
    .join('/');
  const skillNames = context.configuration.skills.map(skill => skill.name);
  const knowledgeNames = context.configuration.knowledge.map(dataset => dataset.name);
  const enabledMemorySlots = context.configuration.memory.slots.filter(slot => slot.enabled);
  const workflowLabels = context.configuration.workflowBindings.map((binding, index) =>
    workflowBindingLabel(binding, index)
  );
  const databaseLabels = context.configuration.databaseBindings.map((binding, index) =>
    databaseBindingLabel(binding, index)
  );
  const agentCapabilities = buildAgentRuntimeCapabilities(context);
  const hasGovernedAgentOperations = agentCapabilities.some(
    capability => capability.status === 'available' && capability.requiresConfirmation
  );
  const canAutoRefreshAgent =
    !context.publish.isDirty &&
    context.publish.saveState !== 'saving' &&
    !context.publish.isVersionPreviewing;
  const workflowContextItems: AIChatPageContextItem[] = context.configuration.workflowBindings
    .filter(binding => binding.workflow_id || binding.binding_id)
    .map((binding, index) => ({
      id: binding.workflow_id || binding.binding_id,
      type: 'workflow',
      title: workflowBindingLabel(binding, index),
      subtitle: `Bound to ${context.agent.name}`,
      description: compactContextField(
        binding.description ||
          `This workflow binding uses ${binding.version_strategy} version strategy.`
      ),
      source: 'Agent Runtime',
      risk: 'medium',
      status: 'available',
      relations: [
        {
          type: 'bound_to_agent',
          resourceType: 'agent',
          resourceId: context.agent.id,
          title: context.agent.name,
          metadata: {
            binding_id: binding.binding_id,
            version_strategy: binding.version_strategy,
            version_uuid: binding.version_uuid,
          },
        },
      ],
      capabilities: buildWorkflowBindingCapabilities(binding),
      metadata: {
        agent_id: context.agent.id,
        workflow_id: binding.workflow_id,
        binding_id: binding.binding_id,
        version_strategy: binding.version_strategy,
        version_uuid: binding.version_uuid,
        timeout_seconds: binding.timeout_seconds,
      },
    }));

  return [
    {
      id: context.agent.id,
      type: 'agent',
      title: context.agent.name,
      subtitle: `${context.agent.type} in ${workspaceLabel}`,
      description: compactContextField(context.agent.description),
      href: `/console/agents/${context.agent.id}/agent`,
      source: 'Agent Runtime',
      risk: 'low',
      status: context.publish.isPublished ? 'published' : 'draft',
      relations: buildAgentWorkflowRelations(context),
      capabilities: agentCapabilities,
      permissions: context.permissions.codes,
      hints: {
        handledAssetTypes: ['agent'],
        toolGovernance: hasGovernedAgentOperations ? { enabled: true } : undefined,
        refreshHints: canAutoRefreshAgent
          ? [
              { assetType: 'agent', queryKey: AGENT_KEYS.lists() },
              { assetType: 'agent', queryKey: AGENT_KEYS.detail(context.agent.id) },
              { assetType: 'agent', queryKey: AGENT_KEYS.config(context.agent.id) },
              { assetType: 'agent', queryKey: AGENT_KEYS.runnable(context.agent.workspace?.id) },
            ]
          : undefined,
      },
      metadata: {
        agent_id: context.agent.id,
        agent_type: context.agent.type,
        workspace_id: context.agent.workspace?.id,
        workspace_name: context.agent.workspace?.name,
        is_published: context.publish.isPublished,
        web_app_status: context.publish.webAppStatus,
        can_edit_agent: context.permissions.canEditAgent,
        can_manage_agent: context.permissions.canManageAgent,
      },
    },
    {
      id: `${context.agent.id}:prompt-model`,
      type: 'custom',
      title: 'Runtime prompt and model',
      subtitle: modelLabel || 'No model selected',
      description: context.configuration.prompt.excerpt || 'Prompt is empty.',
      source: 'Agent Runtime',
      metadata: {
        agent_id: context.agent.id,
        model_provider: context.configuration.model.provider,
        model: context.configuration.model.name,
        prompt_character_count: context.configuration.prompt.characterCount,
        prompt_truncated: context.configuration.prompt.isTruncated,
        prompt_too_long: context.configuration.prompt.isTooLong,
        model_parameter_count: Object.keys(context.configuration.model.parameters).length,
      },
    },
    {
      id: `${context.agent.id}:skills`,
      type: 'custom',
      title: 'Runtime skills',
      subtitle: `${context.configuration.skills.length} selected`,
      description: summarizeNames(skillNames),
      source: 'Agent Runtime',
      metadata: {
        agent_id: context.agent.id,
        skill_count: context.configuration.skills.length,
        skill_ids: summarizeNames(context.configuration.skills.map(skill => skill.id)),
      },
    },
    {
      id: `${context.agent.id}:knowledge`,
      type: 'custom',
      title: 'Runtime knowledge',
      subtitle: `${context.configuration.knowledge.length} datasets`,
      description: summarizeNames(knowledgeNames),
      source: 'Agent Runtime',
      metadata: {
        agent_id: context.agent.id,
        dataset_count: context.configuration.knowledge.length,
        dataset_ids: summarizeNames(context.configuration.knowledge.map(dataset => dataset.id)),
        load_error_count: context.configuration.knowledge.filter(dataset => dataset.loadError)
          .length,
      },
    },
    {
      id: `${context.agent.id}:memory-files`,
      type: 'custom',
      title: 'Runtime memory and files',
      subtitle: `memory ${formatBooleanState(context.configuration.memory.enabled)}, files ${formatBooleanState(
        context.configuration.fileUploadEnabled
      )}`,
      description: summarizeNames(
        enabledMemorySlots.map(slot => `${slot.key}: ${slot.description}`),
        'No enabled memory slots.'
      ),
      source: 'Agent Runtime',
      metadata: {
        agent_id: context.agent.id,
        memory_enabled: context.configuration.memory.enabled,
        memory_slot_count: context.configuration.memory.slots.length,
        enabled_memory_slot_count: enabledMemorySlots.length,
        file_upload_enabled: context.configuration.fileUploadEnabled,
      },
    },
    {
      id: `${context.agent.id}:resources`,
      type: 'custom',
      title: 'Runtime resources',
      subtitle: `${context.configuration.databaseBindings.length} databases, ${context.configuration.workflowBindings.length} workflows`,
      description: compactContextField(
        [
          `Databases: ${summarizeNames(databaseLabels)}`,
          `Workflows: ${summarizeNames(workflowLabels)}`,
          `Suggested questions: ${summarizeNames(context.configuration.suggestedQuestions)}`,
        ].join(' | ')
      ),
      source: 'Agent Runtime',
      metadata: {
        agent_id: context.agent.id,
        database_binding_count: context.configuration.databaseBindings.length,
        workflow_binding_count: context.configuration.workflowBindings.length,
        suggested_question_count: context.configuration.suggestedQuestions.length,
      },
    },
    ...workflowContextItems,
    {
      id: `${context.agent.id}:permissions`,
      type: 'custom',
      title: 'Runtime permissions',
      subtitle: context.permissions.canManageAgent ? 'Can manage agent' : 'View-only agent access',
      description: summarizeNames(context.permissions.codes),
      source: 'Agent Runtime',
      permissions: context.permissions.codes,
      metadata: {
        agent_id: context.agent.id,
        organization_role: context.permissions.organizationRole,
        workspace_role: context.permissions.workspaceRole,
        workspace_role_name: context.permissions.workspaceRoleName,
        permission_count: context.permissions.codes.length,
        save_state: context.publish.saveState,
        is_dirty: context.publish.isDirty,
        is_version_previewing: context.publish.isVersionPreviewing,
      },
    },
  ];
}

export function AgentRuntimeAIChatContextRegistration({
  context,
}: AgentRuntimeAIChatContextRegistrationProps) {
  const items = useMemo(() => buildAgentRuntimeAIChatContextItems(context), [context]);

  usePageContextRegistration(items, { scopeId: context?.id ?? 'agent-runtime' });

  return null;
}
