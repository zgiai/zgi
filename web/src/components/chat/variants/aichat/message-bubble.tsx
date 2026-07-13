'use client';

import { ModelIcon } from 'modelicons';
import { useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  CircleStop,
  Download,
  Eye,
  FileImage,
  FileText,
  Loader2,
} from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useFileDetail } from '@/hooks/file/use-file-detail';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import type {
  AIChatGeneratedFile,
  AIChatMessage,
  AIChatMessageFile,
} from '@/services/types/aichat';
import { isSensitiveOutputBlockedValue } from '@/utils/model-output-filter';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';
import {
  AssistantMessageToolbar,
  UserEditToolbar,
  UserMessageToolbar,
} from '@/components/chat/variants/aichat/message-toolbars';
import { UniversalFilePreviewDialog } from '@/components/files/universal-file-preview-dialog';
import { MarkdownImage } from '@/components/common/markdown-image';
import { isOriginalPreviewImage } from '@/utils/file-helpers';
import { API_URL } from '@/lib/config';
import { AIChatAgenticTimeline } from '@/components/chat/variants/aichat/agentic-timeline';
import type { AIChatToolGovernanceDecisionSubmitPayload } from '@/components/chat/variants/aichat/agentic-timeline';
import {
  getAIChatMessageErrorInput,
  resolveAIChatErrorMessage,
} from '@/components/chat/variants/aichat/error-utils';
import type { AIChatSkillDisplayMap } from '@/components/chat/variants/aichat/skill-display';
import type { AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat';
import {
  dedupeTimelineItems,
  mergeRuntimeTimelineWithMessageTimeline,
  timelineFromAIChatMessage,
} from '@/components/chat/controllers/aichat/selectors';
import {
  debugAIChatTimeline,
  summarizeAIChatTimeline,
} from '@/components/chat/controllers/aichat/debug';
import { MAX_AICHAT_BRANCHES } from '@/components/chat/variants/aichat/types';

interface AIChatMessageBubbleProps {
  message: AIChatMessage;
  isSending?: boolean;
  timeline?: AIChatAgenticTimelineItem[];
  skillDisplayById: AIChatSkillDisplayMap;
  isLastMessage?: boolean;
  canReplaceRoot?: boolean;
  onRegenerate?: (message: AIChatMessage) => void;
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>;
  branchNavigation?: ChatBranchNavigation;
  onSwitchBranch?: (messageId: string) => void;
  isEditing?: boolean;
  editValue?: string;
  onEditStart?: (message: AIChatMessage) => void;
  onEditChange?: (value: string) => void;
  onEditCancel?: () => void;
  onEditSubmit?: (message: AIChatMessage) => void;
  showAssistantModelMeta?: boolean;
  showMemoryKey?: boolean;
  showSkillEventDetails?: boolean;
  enableToolGovernanceApprovals?: boolean;
  suppressPendingToolGovernanceApprovals?: boolean;
}

const EMPTY_MESSAGE_FILES: AIChatMessageFile[] = [];
const EMPTY_GENERATED_FILES: AIChatGeneratedFile[] = [];

function formatAIChatTime(timestamp: number): string {
  if (!timestamp) return '';

  const date = new Date(timestamp * 1000);
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function formatFileSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B';
  }

  const units = ['B', 'KB', 'MB', 'GB'];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${value >= 10 || unitIndex === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}

function formatGeneratedFileExtension(file: AIChatGeneratedFile): string {
  const extension = file.extension || file.filename.split('.').pop() || '';
  return extension.replace(/^\./, '').toUpperCase();
}

function timelineRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function timelineString(value: unknown): string {
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return '';
}

function timelineStatus(value: unknown): string {
  return timelineString(value).toLowerCase();
}

function isSuccessfulTimelineStatus(status: unknown): boolean {
  return ['success', 'succeeded', 'allowed', 'completed', 'approved'].includes(
    timelineStatus(status)
  );
}

function isRunningTimelineStatus(status: unknown): boolean {
  return [
    'loading',
    'running',
    'streaming',
    'pending',
    'needs_approval',
    'waiting_client_action',
  ].includes(timelineStatus(status));
}

function isFailedTimelineStatus(status: unknown): boolean {
  return [
    'blocked',
    'denied',
    'error',
    'failed',
    'failure',
    'rejected',
    'timeout',
    'timed_out',
  ].includes(timelineStatus(status));
}

function timelineInvocationActionType(invocation: Record<string, unknown>): string {
  const result = timelineRecord(invocation.result);
  const args = timelineRecord(invocation.arguments);
  return (
    timelineString(invocation.action_type) ||
    timelineString(result.action_type) ||
    timelineString(args.action_type)
  );
}

function hasAssetOperationEvidence(invocation: Record<string, unknown>): boolean {
  const result = timelineRecord(invocation.result);
  const args = timelineRecord(invocation.arguments);
  return Boolean(
    invocation.asset_operation_audit ||
      result.asset_operation_audit ||
      result.asset_type ||
      result.effect ||
      args.asset_type ||
      args.effect
  );
}

function runningInvocationBlocksStreamingStatus(invocation: Record<string, unknown>): boolean {
  if (!isRunningTimelineStatus(invocation.status)) return false;

  const kind = timelineString(invocation.kind);
  const actionType = timelineInvocationActionType(invocation);
  if (
    kind === 'skill_load' ||
    kind === 'reference_read' ||
    kind === 'intermediate_answer' ||
    kind === 'metadata_exposed' ||
    kind === 'guardrail'
  ) {
    return false;
  }

  if (
    kind === 'client_action' &&
    (actionType === 'asset_observation' || actionType === 'route_navigation')
  ) {
    return false;
  }

  return kind === 'tool_call' || kind === 'tool_governance' || kind === 'client_action';
}

type StreamingOperationStatusKey =
  | 'pageChanged'
  | 'assetCreated'
  | 'assetSaved'
  | 'assetUpdated'
  | 'assetDeleted'
  | 'bindingUpdated'
  | 'bindingBound'
  | 'bindingUnbound'
  | 'agentConfigUpdated'
  | 'approvalContinuing'
  | 'toolCompleted'
  | 'operationFailed'
  | 'pageActionFailed';

interface StreamingOperationStatus {
  key: StreamingOperationStatusKey;
  count?: number;
  assetType?: string;
}

function timelineFirstString(...values: unknown[]): string {
  for (const value of values) {
    const text = timelineString(value);
    if (text) return text;
  }
  return '';
}

function timelineInvocationAudit(invocation: Record<string, unknown>): Record<string, unknown> {
  const result = timelineRecord(invocation.result);
  return timelineRecord(invocation.asset_operation_audit || result.asset_operation_audit);
}

function timelineInvocationEffect(invocation: Record<string, unknown>): string {
  const result = timelineRecord(invocation.result);
  const args = timelineRecord(invocation.arguments);
  const audit = timelineInvocationAudit(invocation);
  const governance = timelineRecord(invocation.governance);
  const manifest = timelineRecord(governance.manifest);
  return timelineFirstString(
    audit.effect,
    result.effect,
    args.effect,
    manifest.effect
  ).toLowerCase();
}

function timelineInvocationAssetType(invocation: Record<string, unknown>): string {
  const result = timelineRecord(invocation.result);
  const args = timelineRecord(invocation.arguments);
  const audit = timelineInvocationAudit(invocation);
  const governance = timelineRecord(invocation.governance);
  const manifest = timelineRecord(governance.manifest);
  return timelineFirstString(
    audit.asset_type,
    result.asset_type,
    args.asset_type,
    manifest.asset_type
  ).toLowerCase();
}

function streamingStatusFromAssetOperation(
  invocation: Record<string, unknown>
): StreamingOperationStatusKey {
  const effect = timelineInvocationEffect(invocation);
  const assetType = timelineInvocationAssetType(invocation);
  const toolName = timelineString(invocation.tool_name).toLowerCase();

  if (effect === 'delete' || toolName.includes('delete') || toolName.includes('remove')) {
    return 'assetDeleted';
  }
  if (toolName.includes('bind') || toolName.includes('unbind') || toolName.includes('binding')) {
    return 'bindingUpdated';
  }
  if (effect === 'create') {
    return assetType === 'file' ? 'assetSaved' : 'assetCreated';
  }
  if (effect === 'publish' || effect === 'update') {
    return 'assetUpdated';
  }
  if (assetType === 'file' && (toolName.includes('save') || toolName.includes('upload'))) {
    return 'assetSaved';
  }
  if (toolName.includes('create')) {
    return assetType === 'file' ? 'assetSaved' : 'assetCreated';
  }
  return 'assetUpdated';
}

function timelineNumber(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value;
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && parsed > 0) return parsed;
  }
  return undefined;
}

function timelineArrayLength(value: unknown): number | undefined {
  return Array.isArray(value) && value.length > 0 ? value.length : undefined;
}

function timelineFirstNumber(...values: unknown[]): number | undefined {
  for (const value of values) {
    const number = timelineNumber(value);
    if (number !== undefined) return number;
    const length = timelineArrayLength(value);
    if (length !== undefined) return length;
  }
  return undefined;
}

function streamingAgentConfigChangeCount(result: Record<string, unknown>): number | undefined {
  return timelineFirstNumber(
    result.config_changes,
    result.binding_changes,
    result.updated_fields,
    result.change_count
  );
}

function timelineFirstRecord(value: unknown): Record<string, unknown> {
  if (Array.isArray(value)) {
    for (const item of value) {
      const record = timelineRecord(item);
      if (Object.keys(record).length > 0) return record;
    }
    return {};
  }
  return timelineRecord(value);
}

function streamingAgentConfigBindingAction(result: Record<string, unknown>): {
  key?: StreamingOperationStatusKey;
  assetType?: string;
} {
  const change = timelineFirstRecord(result.binding_changes);
  const action = timelineFirstString(result.change_action, change.change_action).toLowerCase();
  const assetType = timelineFirstString(
    result.binding_kind,
    change.binding_kind,
    result.asset_type
  );

  if (action === 'bind') return { key: 'bindingBound', assetType };
  if (action === 'unbind') return { key: 'bindingUnbound', assetType };
  return { assetType };
}

function streamingIsAgentConfigUpdateInvocation(invocation: Record<string, unknown>): boolean {
  const result = timelineRecord(invocation.result);
  const skillId = timelineString(invocation.skill_id);
  const toolName = timelineString(invocation.tool_name);
  return (
    skillId === 'agent-management' &&
    (toolName === 'update_agent_config' || toolName === 'agent.update_config') &&
    streamingAgentConfigChangeCount(result) !== undefined
  );
}

function streamingOperationCount(invocation: Record<string, unknown>): number | undefined {
  const result = timelineRecord(invocation.result);
  const args = timelineRecord(invocation.arguments);
  const audit = timelineInvocationAudit(invocation);
  const governance = timelineRecord(invocation.governance);
  const operationGroup = timelineRecord(result.operation_group);
  return timelineFirstNumber(
    audit.asset_count,
    result.asset_count,
    result.target_count,
    result.deleted_count,
    result.created_count,
    result.saved_count,
    result.updated_count,
    result.resource_count,
    result.change_count,
    result.item_results,
    operationGroup.asset_count,
    operationGroup.target_count,
    operationGroup.item_results,
    args.assets,
    audit.assets,
    governance.assets
  );
}

function streamingOperationStatusFromInvocation(
  invocation: Record<string, unknown>
): StreamingOperationStatus {
  const result = timelineRecord(invocation.result);
  if (streamingIsAgentConfigUpdateInvocation(invocation)) {
    const bindingAction = streamingAgentConfigBindingAction(result);
    if (bindingAction.key) {
      return {
        key: bindingAction.key,
        count: streamingAgentConfigChangeCount(result),
        assetType: bindingAction.assetType || 'asset',
      };
    }
    return {
      key: 'agentConfigUpdated',
      count: streamingAgentConfigChangeCount(result),
      assetType: 'agent_config',
    };
  }

  const key = streamingStatusFromAssetOperation(invocation);
  return {
    key,
    count: streamingOperationCount(invocation),
    assetType: timelineInvocationAssetType(invocation),
  };
}

function streamingOperationStatusFromProgress(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>
): StreamingOperationStatus | null {
  if (item.phase !== 'client_action_result') return null;

  const result = timelineRecord(item.result);
  const status = timelineString(item.status) || timelineString(result.status);
  const actionType = (
    timelineString(item.action_type) || timelineString(result.action_type)
  ).toLowerCase();

  if (status && isFailedTimelineStatus(status)) {
    return actionType === 'route_navigation'
      ? { key: 'pageActionFailed' }
      : {
          key: 'operationFailed',
          count: streamingOperationCount({
            kind: 'client_action',
            action_type: actionType,
            status: item.status,
            arguments: {
              effect: item.effect,
              asset_type: item.asset_type,
              assets: item.assets,
            },
            result: item.result,
          }),
          assetType: item.asset_type,
        };
  }
  if (status && !isSuccessfulTimelineStatus(status)) {
    return null;
  }

  if (actionType === 'route_navigation') {
    return { key: 'pageChanged' };
  }
  if (actionType !== 'asset_observation') {
    return null;
  }

  return streamingOperationStatusFromInvocation({
    kind: 'client_action',
    action_type: actionType,
    status: item.status,
    arguments: {
      effect: item.effect,
      asset_type: item.asset_type,
      assets: item.assets,
    },
    result: item.result,
  });
}

function streamingOperationStatusFromGovernanceDecision(
  item: Extract<AIChatAgenticTimelineItem, { type: 'tool_governance_decision' }>,
  pendingMeansContinuation: boolean
): StreamingOperationStatus | null {
  const event = item.event;
  const governance = timelineRecord(event.governance);
  const governanceApprovalResult = timelineRecord(governance.approval_result);
  const eventApprovalResult = timelineRecord(event.approval_result);
  const executionResult = timelineRecord(event.execution_result);
  const manifest = timelineRecord(governance.manifest);
  const audit = event.asset_operation_audit || governance.asset_operation_audit;
  const executionStatus = timelineFirstString(
    event.execution_status,
    executionResult.status
  ).toLowerCase();
  const approvalStatus = timelineFirstString(
    event.approval_status,
    governance.approval_status,
    eventApprovalResult.approval_status,
    governanceApprovalResult.approval_status
  ).toLowerCase();
  const decisionStatus = timelineFirstString(
    event.decision,
    event.status,
    governance.status
  ).toLowerCase();
  const invocation: Record<string, unknown> = {
    kind: 'tool_governance',
    status: executionStatus || approvalStatus || decisionStatus,
    skill_id: event.skill_id,
    tool_name: event.tool_name || event.approval_event?.tool_id || manifest.tool_id,
    asset_operation_audit: audit,
    governance,
    result: {
      ...executionResult,
      asset_operation_audit: executionResult.asset_operation_audit || audit,
      effect: timelineFirstString(executionResult.effect, event.effect, manifest.effect),
      asset_type: timelineFirstString(
        executionResult.asset_type,
        event.asset_type,
        manifest.asset_type
      ),
    },
  };

  if (
    isFailedTimelineStatus(executionStatus) ||
    isFailedTimelineStatus(approvalStatus) ||
    isFailedTimelineStatus(decisionStatus) ||
    event.execution_error
  ) {
    const failedStatus = hasAssetOperationEvidence(invocation)
      ? streamingOperationStatusFromInvocation(invocation)
      : {
          key: 'operationFailed' as const,
          count: streamingOperationCount(invocation),
          assetType: timelineInvocationAssetType(invocation),
        };
    return { ...failedStatus, key: 'operationFailed' };
  }

  const hasExecutionResult = Boolean(event.execution_result);
  const executionSucceeded =
    hasExecutionResult || (executionStatus && isSuccessfulTimelineStatus(executionStatus));
  if (executionSucceeded) {
    if (hasAssetOperationEvidence(invocation)) {
      return streamingOperationStatusFromInvocation(invocation);
    }
    return { key: 'toolCompleted', count: streamingOperationCount(invocation) };
  }

  const approved = approvalStatus === 'approved' || decisionStatus === 'allowed';
  const requiresApproval =
    governance.requires_approval === true ||
    decisionStatus === 'needs_approval' ||
    approvalStatus === 'pending';
  const pendingDuringContinuation =
    pendingMeansContinuation &&
    (decisionStatus === 'needs_approval' ||
      approvalStatus === 'pending' ||
      governance.requires_approval === true);
  if (!approved && !pendingDuringContinuation) {
    return null;
  }
  if (decisionStatus === 'allowed' && !requiresApproval && !approvalStatus) {
    return null;
  }

  return {
    key: 'approvalContinuing',
    count: streamingOperationCount(invocation),
    assetType: timelineInvocationAssetType(invocation),
  };
}

function streamingOperationStatus(
  timeline: AIChatAgenticTimelineItem[],
  pendingGovernanceMeansContinuation: boolean
): StreamingOperationStatus | null {
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const item = timeline[index];
    if (item.type === 'progress_text') {
      const progressStatus = streamingOperationStatusFromProgress(item);
      if (progressStatus) return progressStatus;
      continue;
    }
    if (item.type === 'tool_governance_decision') {
      const governanceStatus = streamingOperationStatusFromGovernanceDecision(
        item,
        pendingGovernanceMeansContinuation
      );
      if (governanceStatus) return governanceStatus;
      continue;
    }
    if (item.type !== 'skill_event') continue;
    const invocation = item.invocation as unknown as Record<string, unknown>;
    if (runningInvocationBlocksStreamingStatus(invocation)) return null;

    const skillId = timelineString(invocation.skill_id);
    const toolName = timelineString(invocation.tool_name);
    const kind = timelineString(invocation.kind);
    const actionType = timelineInvocationActionType(invocation);
    if (isFailedTimelineStatus(invocation.status)) {
      if (
        (skillId === 'console-navigator' && toolName === 'navigate') ||
        (kind === 'client_action' && actionType === 'route_navigation')
      ) {
        return { key: 'pageActionFailed' };
      }
      if (hasAssetOperationEvidence(invocation)) {
        const failedStatus = streamingOperationStatusFromInvocation(invocation);
        return { ...failedStatus, key: 'operationFailed' };
      }
      return {
        key: 'operationFailed',
        count: streamingOperationCount(invocation),
        assetType: timelineInvocationAssetType(invocation),
      };
    }
    if (!isSuccessfulTimelineStatus(invocation.status)) continue;
    if (
      (skillId === 'console-navigator' && toolName === 'navigate') ||
      (kind === 'client_action' && actionType === 'route_navigation')
    ) {
      return { key: 'pageChanged' };
    }
    if (kind === 'client_action' && actionType === 'asset_observation') {
      return { key: 'assetUpdated', count: streamingOperationCount(invocation) };
    }
    if (hasAssetOperationEvidence(invocation)) {
      return streamingOperationStatusFromInvocation(invocation);
    }
    return { key: 'toolCompleted', count: streamingOperationCount(invocation) };
  }
  return null;
}

function streamingOperationAssetLabel(
  assetType: string | undefined,
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  switch ((assetType ?? '').trim().toLowerCase()) {
    case 'agent':
      return t('consoleChat.operationStatus.assetLabels.agent');
    case 'agent_skill':
    case 'skill':
    case 'skills':
    case 'enabled_skill_ids':
      return t('consoleChat.operationStatus.assetLabels.skill');
    case 'file':
      return t('consoleChat.operationStatus.assetLabels.file');
    case 'knowledge_base':
    case 'knowledge':
    case 'dataset':
    case 'knowledge_dataset_ids':
      return t('consoleChat.operationStatus.assetLabels.knowledgeBase');
    case 'database_table':
    case 'database':
    case 'table':
    case 'database_bindings':
      return t('consoleChat.operationStatus.assetLabels.databaseTable');
    case 'workflow':
    case 'workflow_bindings':
      return t('consoleChat.operationStatus.assetLabels.workflow');
    default:
      return t('consoleChat.operationStatus.assetLabels.asset');
  }
}

function streamingOperationStatusText(
  status: StreamingOperationStatus,
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  const count = status.count;
  if (!count || count <= 0) {
    return t(`consoleChat.operationStatus.${status.key}`);
  }
  const asset = streamingOperationAssetLabel(status.assetType, t);
  switch (status.key) {
    case 'assetCreated':
      return t('consoleChat.operationStatus.assetCreatedDetailed', { count, asset });
    case 'assetSaved':
      return t('consoleChat.operationStatus.assetSavedDetailed', { count, asset });
    case 'assetUpdated':
      return t('consoleChat.operationStatus.assetUpdatedDetailed', { count, asset });
    case 'assetDeleted':
      return t('consoleChat.operationStatus.assetDeletedDetailed', { count, asset });
    case 'bindingUpdated':
      return t('consoleChat.operationStatus.bindingUpdatedDetailed', { count, asset });
    case 'bindingBound':
      return t('consoleChat.operationStatus.bindingBoundDetailed', { count, asset });
    case 'bindingUnbound':
      return t('consoleChat.operationStatus.bindingUnboundDetailed', { count, asset });
    case 'agentConfigUpdated':
      return t('consoleChat.operationStatus.agentConfigUpdatedDetailed', { count });
    case 'toolCompleted':
      return t('consoleChat.operationStatus.toolCompletedDetailed', { count, asset });
    case 'operationFailed':
      return t('consoleChat.operationStatus.operationFailedDetailed', { count, asset });
    case 'pageChanged':
    case 'pageActionFailed':
    default:
      return t(`consoleChat.operationStatus.${status.key}`);
  }
}

function apiAbsoluteUrl(pathOrUrl: string | undefined): string {
  const value = pathOrUrl?.trim() ?? '';
  if (!value) return '';
  const base = API_URL.trim().replace(/\/+$/, '');
  if (/^https?:/i.test(value)) {
    return value;
  }
  if (/^(?:data:|blob:)/i.test(value)) return value;
  if (!value.startsWith('/')) return value;
  return base ? `${base}${value}` : value;
}

function apiReachableUrl(pathOrUrl: string | undefined): string {
  const absolute = apiAbsoluteUrl(pathOrUrl);
  const base = API_URL.trim().replace(/\/+$/, '');
  if (!absolute || !base || !/^https?:/i.test(absolute)) {
    return absolute;
  }

  try {
    const parsed = new URL(absolute);
    if (!parsed.pathname.startsWith('/console/api/files/')) {
      return absolute;
    }
    return `${base}${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return absolute;
  }
}

function appendQueryFlag(rawUrl: string, key: string, value: string): string {
  const url = rawUrl.trim();
  if (!url) return '';
  try {
    const parsed = new URL(url, window.location.origin);
    parsed.searchParams.set(key, value);
    if (url.startsWith('/')) {
      return `${parsed.pathname}${parsed.search}${parsed.hash}`;
    }
    return parsed.toString();
  } catch {
    const separator = url.includes('?') ? '&' : '?';
    return `${url}${separator}${encodeURIComponent(key)}=${encodeURIComponent(value)}`;
  }
}

function isManagedGeneratedFile(file: AIChatGeneratedFile): boolean {
  return file.target === 'managed_file' || Boolean(file.upload_file_id);
}

function generatedFileIsExpired(file: AIChatGeneratedFile, now = Date.now()): boolean {
  if (isManagedGeneratedFile(file)) return false;
  if (file.availability === 'expired') return true;
  if (!file.expires_at) return false;
  return file.expires_at * 1000 <= now;
}

function useGeneratedFileExpired(file: AIChatGeneratedFile): boolean {
  const [isExpired, setIsExpired] = useState(() => generatedFileIsExpired(file));

  useEffect(() => {
    const expired = generatedFileIsExpired(file);
    setIsExpired(expired);
    if (expired || isManagedGeneratedFile(file) || !file.expires_at) return;

    const delay = Math.max(0, file.expires_at * 1000 - Date.now());
    const timeoutId = window.setTimeout(() => setIsExpired(true), delay + 50);
    return () => window.clearTimeout(timeoutId);
  }, [file]);

  return isExpired;
}

function managedGeneratedFileId(file: AIChatGeneratedFile): string {
  if (!isManagedGeneratedFile(file)) return '';
  return file.upload_file_id || file.file_id || '';
}

function managedGeneratedFilePreviewUrl(file: AIChatGeneratedFile): string {
  const fileId = managedGeneratedFileId(file);
  return fileId
    ? apiReachableUrl(`/console/api/files/${encodeURIComponent(fileId)}/file-preview`)
    : '';
}

function managedGeneratedFileDownloadUrl(file: AIChatGeneratedFile): string {
  const fileId = managedGeneratedFileId(file);
  if (file.download_url) {
    return apiReachableUrl(file.download_url);
  }
  if (file.url) {
    return appendQueryFlag(apiReachableUrl(file.url), 'as_attachment', 'true');
  }
  return apiReachableUrl(fileId ? `/console/api/files/${encodeURIComponent(fileId)}/download` : '');
}

function generatedFilePreviewUrl(file: AIChatGeneratedFile): string {
  if (isManagedGeneratedFile(file)) {
    return managedGeneratedFilePreviewUrl(file);
  }
  if (file.url) {
    return apiReachableUrl(file.url);
  }
  return '';
}

function generatedFileNotFoundError(error: unknown): boolean {
  const candidate = error as
    | {
        message?: string;
        response?: { status?: number; data?: { message?: string; errorMessage?: string } };
        businessError?: { message?: string };
      }
    | null
    | undefined;
  if (!candidate) return false;
  if (candidate.response?.status === 404) return true;

  const message = (
    candidate.businessError?.message ||
    candidate.response?.data?.message ||
    candidate.response?.data?.errorMessage ||
    candidate.message ||
    ''
  )
    .trim()
    .toLowerCase();
  return (
    message.includes('file not found') ||
    message.includes('not found') ||
    message.includes('文件不存在') ||
    message.includes('未找到')
  );
}

function generatedFileDownloadUrl(file: AIChatGeneratedFile): string {
  if (isManagedGeneratedFile(file)) {
    return managedGeneratedFileDownloadUrl(file);
  }
  return apiReachableUrl(file.download_url || file.url);
}

function generatedImagePreviewFiles(
  _answer: string,
  generatedFiles: AIChatGeneratedFile[],
  shouldShow: boolean
): AIChatGeneratedFile[] {
  if (!shouldShow || generatedFiles.length === 0) {
    return [];
  }

  return generatedFiles.filter(file => {
    const previewUrl = generatedFilePreviewUrl(file);
    if (!previewUrl) return false;
    if (!isOriginalPreviewImage(file.extension, file.mime_type)) return false;
    return true;
  });
}

interface AIChatHistoryImagePreviewProps {
  file: AIChatMessageFile;
}

/**
 * @component AIChatHistoryImagePreview
 * @category Feature
 * @status Stable
 * @description Renders historical AIChat image attachments using the signed preview URL endpoint.
 * @usage Used in AIChatMessageBubble for message metadata files
 * @example
 * <AIChatHistoryImagePreview file={file} />
 */
function AIChatHistoryImagePreview({ file }: AIChatHistoryImagePreviewProps) {
  const t = useT('webapp');
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file.id, {
    enabled: Boolean(file.id),
  });
  const isFiltered = file.content_status === 'filtered';
  const isError = file.parse_status === 'error' || Boolean(error);
  const title =
    file.error ||
    error ||
    (file.filtered_reason === 'model_without_vision'
      ? t('consoleChat.attachments.filteredModelWithoutVision')
      : file.name);

  return (
    <>
      <button
        type="button"
        className={cn(
          'relative size-24 overflow-hidden rounded-lg border bg-background/70 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          isError || isFiltered ? 'border-destructive/40' : 'border-border',
          previewUrl || isError || isFiltered ? 'cursor-pointer' : 'cursor-default'
        )}
        title={title}
        onClick={() => {
          if (previewUrl || isError || isFiltered) {
            setIsPreviewOpen(true);
          }
        }}
      >
        {previewUrl ? (
          <img src={previewUrl} alt={file.name} className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            {isLoading ? (
              <Loader2 className="size-5 animate-spin" />
            ) : isError || isFiltered ? (
              <AlertCircle className="size-5 text-destructive" />
            ) : (
              <FileImage className="size-5" />
            )}
          </div>
        )}
      </button>
      <Dialog open={isPreviewOpen} onOpenChange={setIsPreviewOpen}>
        <DialogContent className="max-h-[90vh] max-w-[90vw] overflow-hidden p-0">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle className="truncate text-sm">{file.name}</DialogTitle>
          </DialogHeader>
          <div className="flex max-h-[calc(90vh-56px)] min-h-64 items-center justify-center overflow-auto bg-muted/30 p-4">
            {previewUrl ? (
              <img
                src={previewUrl}
                alt={file.name}
                className="max-h-[calc(90vh-96px)] max-w-full object-contain"
              />
            ) : (
              <div className="flex max-w-sm flex-col items-center gap-2 text-center text-sm text-muted-foreground">
                <AlertCircle className="size-6 text-destructive" />
                <span>{title || t('consoleChat.attachments.previewLoadError')}</span>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

interface AIChatGeneratedFileCardProps {
  file: AIChatGeneratedFile;
}

/**
 * @component AIChatGeneratedFileCard
 * @category Feature
 * @status Stable
 * @description Renders a downloadable file artifact generated by an AIChat skill.
 * @usage Used in AIChatMessageBubble for skill_artifact_created outputs.
 * @example
 * <AIChatGeneratedFileCard file={file} />
 */
function AIChatGeneratedFileCard({ file }: AIChatGeneratedFileCardProps) {
  const t = useT('webapp');
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const extension = formatGeneratedFileExtension(file);
  const managedFileId = managedGeneratedFileId(file);
  const isManagedFile = isManagedGeneratedFile(file);
  const isExpiredTemporaryFile = useGeneratedFileExpired(file);
  const { data: managedFileDetail, error: managedFileError } = useFileDetail(managedFileId, {
    enabled: isManagedFile && Boolean(managedFileId),
    refetchOnWindowFocus: true,
    skipErrorHandling: true,
  });
  const isArchivedManagedFile = Boolean(managedFileDetail?.data?.file?.is_archived);
  const isDeletedManagedFile =
    isManagedFile && (isArchivedManagedFile || generatedFileNotFoundError(managedFileError));
  const isUnavailableFile = isDeletedManagedFile || isExpiredTemporaryFile;
  const downloadUrl = generatedFileDownloadUrl(file);
  const previewUrl = generatedFilePreviewUrl(file) || downloadUrl;
  const canPreview = Boolean(previewUrl) && !isUnavailableFile;
  const canDownload = Boolean(downloadUrl) && !isUnavailableFile;
  const lifecycleLabel = isDeletedManagedFile
    ? t('consoleChat.attachments.deletedGeneratedFile')
    : isExpiredTemporaryFile
      ? t('consoleChat.attachments.expiredGeneratedFile')
      : isManagedFile
        ? t('consoleChat.attachments.managedGeneratedFile')
        : t('consoleChat.attachments.temporaryGeneratedFile');

  return (
    <>
      <div
        className={cn(
          'flex min-w-0 max-w-sm items-center gap-3 rounded-md border bg-background px-3 py-2 text-sm shadow-sm',
          isUnavailableFile && 'border-destructive/30 bg-destructive/5'
        )}
        title={file.filename}
      >
        <div
          className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground',
            isUnavailableFile && 'bg-destructive/10 text-destructive'
          )}
        >
          {isUnavailableFile ? <AlertCircle className="size-4" /> : <FileText className="size-4" />}
        </div>
        <div className="min-w-0 flex-1">
          <div
            className={cn(
              'truncate font-medium text-foreground',
              isUnavailableFile && 'text-muted-foreground'
            )}
          >
            {file.filename}
          </div>
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span className={cn(isUnavailableFile && 'text-destructive')}>{lifecycleLabel}</span>
            {extension ? <span>{extension}</span> : null}
            <span>{formatFileSize(file.size)}</span>
          </div>
        </div>
        <Button
          type="button"
          isIcon
          variant="ghost"
          className="size-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
          aria-label={t('consoleChat.attachments.previewGeneratedFile')}
          title={t('consoleChat.attachments.previewGeneratedFile')}
          disabled={!canPreview}
          onClick={() => {
            if (canPreview) {
              setIsPreviewOpen(true);
            }
          }}
        >
          <Eye className="size-4" />
        </Button>
        {canDownload ? (
          <Button
            asChild
            type="button"
            isIcon
            variant="ghost"
            className="size-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
            aria-label={t('consoleChat.attachments.downloadGeneratedFile')}
            title={t('consoleChat.attachments.downloadGeneratedFile')}
          >
            <a href={downloadUrl} download={file.filename}>
              <Download className="size-4" />
            </a>
          </Button>
        ) : (
          <Button
            type="button"
            isIcon
            variant="ghost"
            disabled
            className="size-8 shrink-0 rounded-full text-muted-foreground"
            aria-label={t('consoleChat.attachments.downloadGeneratedFile')}
            title={t('consoleChat.attachments.downloadGeneratedFile')}
          >
            <Download className="size-4" />
          </Button>
        )}
      </div>
      <UniversalFilePreviewDialog
        open={isPreviewOpen && canPreview}
        onOpenChange={setIsPreviewOpen}
        file={{
          id: managedFileId || file.file_id,
          name: file.filename,
          extension: file.extension,
          mimeType: file.mime_type,
          size: file.size,
          previewUrl,
          downloadUrl: canDownload ? downloadUrl : '',
        }}
      />
    </>
  );
}

interface AIChatGeneratedImagePreviewsProps {
  files: AIChatGeneratedFile[];
}

function AIChatGeneratedImagePreview({ file }: { file: AIChatGeneratedFile }) {
  const isExpired = useGeneratedFileExpired(file);
  const previewUrl = generatedFilePreviewUrl(file);
  if (!previewUrl || isExpired) {
    return null;
  }

  return (
    <MarkdownImage
      src={previewUrl}
      alt={file.filename}
      frameClassName="max-w-full"
      imageClassName="min-w-32 max-w-full"
    />
  );
}

function AIChatGeneratedImagePreviews({ files }: AIChatGeneratedImagePreviewsProps) {
  if (files.length === 0) {
    return null;
  }

  return (
    <div className="mt-3 flex max-w-full flex-col items-start gap-3">
      {files.map(file => (
        <AIChatGeneratedImagePreview
          key={file.file_id || generatedFilePreviewUrl(file)}
          file={file}
        />
      ))}
    </div>
  );
}

/**
 * @component AIChatMessageBubble
 * @category Feature
 * @status Stable
 * @description Renders one AIChat persisted turn as user query plus assistant answer.
 * @usage Used by AIChatShell for standalone console chat messages
 * @example
 * <AIChatMessageBubble message={message} />
 */
export function AIChatMessageBubble({
  message,
  isSending = false,
  timeline = [],
  skillDisplayById,
  isLastMessage = false,
  canReplaceRoot = false,
  onRegenerate,
  onToolGovernanceDecision,
  branchNavigation,
  onSwitchBranch,
  isEditing = false,
  editValue = '',
  onEditStart,
  onEditChange,
  onEditCancel,
  onEditSubmit,
  showAssistantModelMeta = true,
  showMemoryKey = true,
  showSkillEventDetails = true,
  enableToolGovernanceApprovals = false,
  suppressPendingToolGovernanceApprovals = false,
}: AIChatMessageBubbleProps) {
  const t = useT('webapp');
  const tGlobal = useT();
  const tCommon = useT('common');
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const { canManageModelConfig: isBillingAdmin } = useAccountCapabilities();
  const isStreaming = message.status === 'pending' || message.status === 'streaming';
  const isWaitingForUser =
    message.status === 'waiting_approval' || message.status === 'waiting_question';
  const isWaitingForClientAction = message.status === 'waiting_client_action';
  const isActiveMessage = isStreaming || isWaitingForUser || isWaitingForClientAction;
  const isError = message.status === 'error';
  const isStopped = message.status === 'stopped';
  const isSensitiveBlocked =
    message.metadata?.sensitiveOutputBlocked === true ||
    isSensitiveOutputBlockedValue(message.answer);
  const displayAnswer = isSensitiveBlocked ? tCommon('sensitiveOutput.blocked') : message.answer;
  const hasParent = Boolean(message.parent_id);
  const branchCount = branchNavigation?.total ?? 1;
  const canCreateBranch = hasParent && branchCount < MAX_AICHAT_BRANCHES;
  const canEdit =
    Boolean(onEditStart && (canReplaceRoot || canCreateBranch)) && !isSending && !isActiveMessage;
  const canRegenerateMessage = Boolean(onRegenerate && (canReplaceRoot || canCreateBranch));
  const canSwitchBranch =
    Boolean(branchNavigation && onSwitchBranch) && !isSending && !isActiveMessage;
  const shouldHideAssistantToolbar = isLastMessage && isActiveMessage;
  const toolbarVisibility = isLastMessage
    ? 'opacity-100'
    : 'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100';
  const files = message.metadata?.files ?? EMPTY_MESSAGE_FILES;
  const generatedFiles = message.metadata?.generated_files ?? EMPTY_GENERATED_FILES;
  const visibleGeneratedFiles = generatedFiles;
  const generatedImagePreviewFilesForDisplay = useMemo(
    () => generatedImagePreviewFiles(displayAnswer, visibleGeneratedFiles, !isSensitiveBlocked),
    [displayAnswer, visibleGeneratedFiles, isSensitiveBlocked]
  );
  const hasGeneratedImagePreviews = generatedImagePreviewFilesForDisplay.length > 0;
  const answer = displayAnswer.trim();
  const userInputRequest = message.metadata?.user_input_request;
  const waitingMessage =
    message.status === 'waiting_approval'
      ? t('consoleChat.waitingApprovalMessage')
      : message.status === 'waiting_question'
        ? userInputRequest
          ? null
          : t('consoleChat.waitingQuestionMessage')
        : message.status === 'waiting_client_action'
          ? t('consoleChat.waitingClientActionMessage')
          : null;
  const imageFiles = files.filter(file => file.kind === 'image');
  const documentFiles = files.filter(file => file.kind !== 'image');
  const historicalTimeline = useMemo<AIChatAgenticTimelineItem[]>(
    () => timelineFromAIChatMessage(message),
    [message]
  );
  const runtimeTimeline = timeline;
  const shouldPreferPersistedTimeline =
    !isActiveMessage && historicalTimeline.length > 0 && runtimeTimeline.length > 0;
  const displayTimeline = useMemo(() => {
    if (shouldPreferPersistedTimeline) {
      return dedupeTimelineItems(historicalTimeline);
    }
    return dedupeTimelineItems(
      mergeRuntimeTimelineWithMessageTimeline(historicalTimeline, runtimeTimeline)
    );
  }, [historicalTimeline, runtimeTimeline, shouldPreferPersistedTimeline]);
  useEffect(() => {
    debugAIChatTimeline('render:message_bubble', {
      message_id: message.id,
      conversation_id: message.conversation_id,
      status: message.status,
      answer_len: displayAnswer.length,
      historical_count: historicalTimeline.length,
      runtime_count: runtimeTimeline.length,
      display_count: displayTimeline.length,
      historical: summarizeAIChatTimeline(historicalTimeline),
      runtime: summarizeAIChatTimeline(runtimeTimeline),
      display: summarizeAIChatTimeline(displayTimeline),
    });
  }, [
    displayAnswer.length,
    displayTimeline,
    historicalTimeline,
    message.conversation_id,
    message.id,
    message.status,
    runtimeTimeline,
  ]);
  const hasTimeline = displayTimeline.length > 0;
  const streamingStatus = useMemo(() => {
    if (isStreaming || isWaitingForClientAction) {
      return streamingOperationStatus(displayTimeline, isStreaming);
    }
    return null;
  }, [displayTimeline, isStreaming, isWaitingForClientAction]);
  const streamingStatusLabel = useMemo(
    () =>
      streamingStatus
        ? streamingOperationStatusText(streamingStatus, (key, values) => t(key as never, values))
        : null,
    [streamingStatus, t]
  );
  const shouldOpenTimelineByDefault =
    isActiveMessage ||
    displayTimeline.some(
      item =>
        item.type === 'user_input_request' ||
        item.type === 'user_input_response' ||
        (item.type === 'skill_event' &&
          item.invocation.kind !== 'guardrail' &&
          (item.invocation.status === 'error' || item.invocation.status === 'blocked')) ||
        (item.type === 'tool_governance_decision' &&
          ['rejected'].includes(
            String(
              item.event.approval_status ??
                item.event.governance?.approval_status ??
                item.event.governance?.approval_result?.approval_status ??
                ''
            ).toLowerCase()
          ))
    );
  const errorDisplay = useMemo(
    () =>
      isError
        ? resolveAIChatErrorMessage(
            (key, values) => tGlobal(key as never, values),
            getAIChatMessageErrorInput(message),
            {
              isAdmin: isBillingAdmin,
              workspaceId: currentWorkspace?.id,
            }
          )
        : null,
    [currentWorkspace?.id, isBillingAdmin, isError, message, tGlobal]
  );

  return (
    <div className="group space-y-3">
      <div className="flex justify-end">
        <div className={cn('max-w-[82%]', isEditing ? 'w-full max-w-2xl' : '')}>
          {isEditing ? (
            <div className="rounded-2xl border bg-background p-2 shadow-sm">
              <Textarea
                value={editValue}
                onChange={event => onEditChange?.(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault();
                    onEditSubmit?.(message);
                  }
                }}
                className="max-h-40 min-h-20 resize-none border-0 bg-transparent px-2 py-1 text-sm shadow-none focus-visible:ring-0"
                autoFocus
              />
              <UserEditToolbar
                canSubmit={Boolean(editValue.trim())}
                isSending={isSending}
                onCancel={onEditCancel}
                onSubmit={() => onEditSubmit?.(message)}
              />
            </div>
          ) : (
            <>
              <div className="rounded-2xl bg-muted px-3 py-2 text-sm text-foreground shadow-sm">
                <div className="whitespace-pre-wrap break-words">{message.query}</div>
                {files.length > 0 ? (
                  <div className="mt-2 space-y-2">
                    {imageFiles.length > 0 ? (
                      <div className="flex flex-wrap gap-2">
                        {imageFiles.map(file => (
                          <AIChatHistoryImagePreview key={file.id} file={file} />
                        ))}
                      </div>
                    ) : null}
                    {documentFiles.length > 0 ? (
                      <div className="flex flex-wrap gap-1.5">
                        {documentFiles.map(file => {
                          const isFileParsing =
                            file.parse_status === 'parsing' ||
                            (file.content_status === 'pending' && file.parse_status !== 'error');
                          const isFileError = file.parse_status === 'error';
                          const isFileEmpty = file.content_status === 'empty' && !isFileError;
                          const isFileExtracted =
                            file.content_status === 'extracted' && !isFileError;
                          const isVisionReady =
                            file.content_status === 'vision_ready' && !isFileError;
                          const isFiltered = file.content_status === 'filtered' && !isFileError;
                          const label = isFileError
                            ? t('consoleChat.attachments.parseFailed')
                            : isFileEmpty
                              ? t('consoleChat.attachments.empty')
                              : isFiltered
                                ? t('consoleChat.attachments.filtered')
                                : isVisionReady
                                  ? t('consoleChat.attachments.visionReady')
                                  : isFileExtracted
                                    ? t('consoleChat.attachments.parsed')
                                    : t('consoleChat.attachments.parsing');

                          return (
                            <div
                              key={file.id}
                              className={cn(
                                'inline-flex max-w-full items-center gap-1.5 rounded-md border bg-background/70 px-2 py-1 text-xs',
                                isFileError || isFiltered
                                  ? 'border-destructive/40 text-destructive'
                                  : 'border-border text-muted-foreground'
                              )}
                              title={
                                file.error ||
                                (file.filtered_reason === 'model_without_vision'
                                  ? t('consoleChat.attachments.filteredModelWithoutVision')
                                  : file.name)
                              }
                            >
                              {isFileParsing ? (
                                <Loader2 className="size-3.5 shrink-0 animate-spin" />
                              ) : isFileError ? (
                                <AlertCircle className="size-3.5 shrink-0" />
                              ) : isFileExtracted || isVisionReady ? (
                                <CheckCircle2 className="size-3.5 shrink-0 text-emerald-600" />
                              ) : isFiltered ? (
                                <AlertCircle className="size-3.5 shrink-0" />
                              ) : file.kind === 'image' ? (
                                <FileImage className="size-3.5 shrink-0" />
                              ) : (
                                <FileText className="size-3.5 shrink-0" />
                              )}
                              <span className="max-w-40 truncate text-foreground">{file.name}</span>
                              <span className="shrink-0">{formatFileSize(file.size)}</span>
                              <span className="shrink-0">{label}</span>
                            </div>
                          );
                        })}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <UserMessageToolbar
                query={message.query}
                canEdit={canEdit}
                isDisabled={isSending || isActiveMessage}
                toolbarVisibility={toolbarVisibility}
                onEdit={() => onEditStart?.(message)}
              />
            </>
          )}
        </div>
      </div>

      <div className="min-w-0 max-w-full overflow-hidden">
        <div className="mb-2 flex min-w-0 items-center gap-2 overflow-hidden whitespace-nowrap text-sm text-muted-foreground">
          <div
            className={cn(
              'flex size-6 shrink-0 items-center justify-center rounded-full',
              showAssistantModelMeta ? 'border bg-background' : 'bg-primary text-primary-foreground'
            )}
          >
            {showAssistantModelMeta ? (
              <ModelIcon model={message.model_name || 'unknown'} size={24} />
            ) : (
              <Bot className="size-3.5" />
            )}
          </div>
          {showAssistantModelMeta && message.model_name ? (
            <span className="min-w-0 truncate text-sm font-medium leading-5 text-foreground/80">
              {message.model_name}
            </span>
          ) : null}
          {message.created_at ? (
            <span className="shrink-0 text-sm leading-5">
              {formatAIChatTime(message.created_at)}
            </span>
          ) : null}
          {isStreaming ? (
            <span className="inline-flex items-center gap-1">
              <Loader2 className="size-3 animate-spin" />
              {t('consoleChat.streaming')}
            </span>
          ) : null}
          {!isStreaming && isWaitingForUser ? (
            <span className="inline-flex items-center gap-1">
              <Loader2 className="size-3 animate-spin" />
              {message.status === 'waiting_approval'
                ? t('consoleChat.waitingApproval')
                : t('consoleChat.waitingQuestion')}
            </span>
          ) : null}
          {!isStreaming && isWaitingForClientAction ? (
            <span className="inline-flex items-center gap-1">
              <Loader2 className="size-3 animate-spin" />
              {t('consoleChat.waitingClientAction')}
            </span>
          ) : null}
          {isStopped && answer ? (
            <span
              className="inline-flex items-center"
              title={t('consoleChat.stopped')}
              aria-label={t('consoleChat.stopped')}
            >
              <CircleStop className="size-3" />
            </span>
          ) : null}
        </div>

        {hasTimeline ? (
          <AIChatAgenticTimeline
            key={`${message.id}-${isActiveMessage ? 'active' : 'history'}-${
              shouldOpenTimelineByDefault ? 'open' : 'closed'
            }`}
            timeline={displayTimeline}
            skillDisplayById={skillDisplayById}
            defaultOpen={shouldOpenTimelineByDefault}
            showMemoryKey={showMemoryKey}
            showSkillEventDetails={showSkillEventDetails}
            enableToolGovernanceApprovals={enableToolGovernanceApprovals}
            suppressPendingToolGovernanceApprovals={suppressPendingToolGovernanceApprovals}
            messageStatus={message.status}
            onToolGovernanceDecision={onToolGovernanceDecision}
          />
        ) : null}

        {streamingStatusLabel ? (
          <div className="mb-3 flex min-w-0 items-center gap-2 rounded-md border border-muted-foreground/15 bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
            <Loader2 className="size-3.5 shrink-0 animate-spin" />
            <span className="min-w-0 break-words">{streamingStatusLabel}</span>
          </div>
        ) : null}

        {answer ? (
          <div className="prose prose-sm min-w-0 max-w-full overflow-hidden dark:prose-invert">
            <MarkdownViewer
              className="md-viewer min-w-0 max-w-full overflow-hidden break-words"
              content={displayAnswer}
              isStreaming={isStreaming}
              renderIdentity={message.id}
            />
          </div>
        ) : waitingMessage ? (
          <div className="text-sm text-muted-foreground">{waitingMessage}</div>
        ) : isStreaming &&
          !streamingStatusLabel &&
          !userInputRequest &&
          !hasGeneratedImagePreviews ? (
          <div className="border-l-2 border-muted-foreground/20 pl-3 text-sm leading-7 text-muted-foreground">
            <span className="inline-flex min-w-0 items-center gap-2">
              <Loader2 className="size-3.5 shrink-0 animate-spin" />
              <span className="min-w-0">{t('consoleChat.operationStatus.planning')}</span>
            </span>
          </div>
        ) : isStopped && !hasGeneratedImagePreviews ? (
          <div className="text-sm text-muted-foreground">{t('consoleChat.stopped')}</div>
        ) : null}

        <AIChatGeneratedImagePreviews files={generatedImagePreviewFilesForDisplay} />

        {visibleGeneratedFiles.length > 0 ? (
          <div className="mt-3 flex flex-wrap gap-2">
            {visibleGeneratedFiles.map(file => (
              <AIChatGeneratedFileCard key={file.artifact_id || file.file_id} file={file} />
            ))}
          </div>
        ) : null}

        {isError ? (
          <div
            className={cn(
              'mt-2 flex items-start gap-2 rounded-md border p-3 text-sm',
              errorDisplay?.isBilling
                ? 'border-amber-200 bg-amber-50 text-amber-950'
                : 'border-destructive/30 bg-destructive/10 text-destructive'
            )}
          >
            <AlertCircle className="mt-0.5 size-4 shrink-0" />
            <div className="min-w-0 flex-1 space-y-2">
              <div>{errorDisplay?.description || t('consoleChat.streamError')}</div>
              {isBillingAdmin && errorDisplay?.href && errorDisplay.actionLabel ? (
                <a
                  href={errorDisplay.href}
                  className="inline-flex h-7 items-center rounded-[4px] border border-amber-300 bg-white px-2.5 text-xs font-semibold text-amber-950 transition-colors hover:border-amber-400 hover:bg-amber-100"
                >
                  {errorDisplay.actionLabel}
                </a>
              ) : null}
            </div>
          </div>
        ) : null}

        {!shouldHideAssistantToolbar && (answer || canRegenerateMessage) ? (
          <AssistantMessageToolbar
            answer={answer}
            canRegenerate={canRegenerateMessage}
            isDisabled={isSending || isActiveMessage}
            toolbarVisibility={toolbarVisibility}
            branchNavigation={branchNavigation}
            canSwitchBranch={canSwitchBranch}
            onRegenerate={() => onRegenerate?.(message)}
            onSwitchBranch={onSwitchBranch}
          />
        ) : null}
      </div>
    </div>
  );
}
