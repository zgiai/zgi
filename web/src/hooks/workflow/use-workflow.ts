import { mergeSavedConnectorNodes } from '@/components/workflow/store/helpers/merge-saved-connectors';
import {
  currentWorkflowEditorSession,
  isWorkflowEditorSessionActive,
} from '@/components/workflow/store/helpers/editor-session';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type {
  WorkflowData,
  WorkflowDraftData,
  WorkflowDraftSavePayload,
  ConversationVariableDraftItem,
} from '@/components/workflow/store/type';
import type { ApiResponseData } from '@/services/types/common';
import type { WorkflowLatestVersion } from '@/services/types/workflow';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { useWorkflowStore } from '@/components/workflow/store/store';
import { WORKFLOW_KEYS, AGENT_KEYS } from '@/hooks/query-keys';

// Convert outgoing WorkflowData to draft payload expected by API
function serializeWorkflowForApi(data: WorkflowData): WorkflowDraftSavePayload {
  const conv: ConversationVariableDraftItem[] = Array.isArray(data.conversation_variables)
    ? data.conversation_variables.map(v => ({
        id: v.id,
        name: v.name,
        value_type: v.type,
        value: v.value,
        description: v.description,
      }))
    : [];
  return {
    graph: data.graph,
    connector_reference_commands: data.connector_reference_commands,
    features: data.features,
    environment_variables: data.environment_variables,
    conversation_variables: conv,
    hash: data.hash,
  };
}

/**
 * Hook for managing workflow draft data
 * @param agentId - The agent ID
 */
export function useWorkflowDraft(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_KEYS.draft(agentId),
    queryFn: () => workflowService.getWorkflowDraft(agentId),
    enabled: !!agentId,
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
}

/**
 * Hook for saving workflow draft data
 */
export function useSaveWorkflowDraft() {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation({
    mutationFn: ({
      agentId,
      workflowData,
    }: {
      agentId: string;
      workflowData: WorkflowData;
      silent?: boolean;
      /** Editor session that started the save; defaults to the one open now. */
      editorSession?: number | null;
    }) => workflowService.saveWorkflowDraft(agentId, serializeWorkflowForApi(workflowData)),
    onMutate: async ({ agentId, workflowData, editorSession: startedBy }) => {
      // Capture the owning editor before any await so later store writes can
      // tell whether that editor is still the one on screen.
      const editorSession = startedBy ?? currentWorkflowEditorSession(agentId);
      // Cancel any outgoing refetches
      await queryClient.cancelQueries({ queryKey: WORKFLOW_KEYS.draft(agentId) });

      // Snapshot the previous value
      const previousWorkflowDraft = queryClient.getQueryData<WorkflowDraftData>(
        WORKFLOW_KEYS.draft(agentId)
      );
      const {
        isDirty: prevIsDirty,
        hasLayoutChanges: prevHasLayoutChanges,
        lastSavedAt,
      } = useWorkflowStore.getState();

      // Optimistically update to the new value
      queryClient.setQueryData<WorkflowDraftData>(WORKFLOW_KEYS.draft(agentId), old => {
        if (!old) return old;
        const conv: ConversationVariableDraftItem[] = Array.isArray(
          workflowData.conversation_variables
        )
          ? workflowData.conversation_variables.map(v => ({
              id: v.id,
              name: v.name,
              value_type: v.type,
              value: v.value,
              description: v.description,
            }))
          : [];
        return {
          ...old,
          graph: workflowData.graph,
          features: workflowData.features,
          conversation_variables: conv,
          updated_at: Date.now(),
        } as unknown as WorkflowDraftData;
      });

      if (isWorkflowEditorSessionActive(editorSession)) {
        useWorkflowStore.setState({
          isDirty: false,
          hasLayoutChanges: false,
          suppressNextLayoutDirty: false,
          suppressNextViewportDirty: false,
          lastSavedAt: Date.now(),
        });
      }

      // Return a context object with the snapshotted value
      return {
        previousWorkflowDraft,
        agentId,
        prevIsDirty,
        prevHasLayoutChanges,
        lastSavedAt,
        editorSession,
      } as const;
    },
    onError: (err, variables, context) => {
      // If the mutation fails, use the context returned from onMutate to roll back
      if (context?.previousWorkflowDraft && context?.agentId) {
        queryClient.setQueryData(
          WORKFLOW_KEYS.draft(context.agentId),
          context.previousWorkflowDraft
        );
      }
      // The global store belongs to whichever editor is open now; only roll
      // back its flags when that is still this workflow.
      if (context && isWorkflowEditorSessionActive(context.editorSession)) {
        useWorkflowStore.setState({
          isDirty: context.prevIsDirty ?? true,
          hasLayoutChanges: context.prevHasLayoutChanges ?? false,
          lastSavedAt: context.lastSavedAt ?? null,
        });
      }
      if (!variables?.silent) {
        toast.error(t('workflow.workflowDraftSaveFailed'));
      }
      console.error('Save workflow draft error:', err);
    },
    onSuccess: (data, variables, context) => {
      void queryClient.invalidateQueries({ queryKey: ['connectors'] });
      // 1. Partially update the React Query cache with canonical server data
      // This prevents data loss (overwriting nodes with partial response)
      // and eliminates the need for a fresh GET request.
      queryClient.setQueryData<WorkflowDraftData>(WORKFLOW_KEYS.draft(variables.agentId), old => {
        if (!old) return undefined;
        return {
          ...old,
          graph: data.graph ?? old.graph,
          hash: data.hash,
          updated_at: new Date(data.updated_at).getTime(),
        };
      });

      // 2. Synchronize the canonical hash into the Zustand store
      // This ensures the next save request carries the correct current hash.
      // Skip when the user already left this workflow: the store now holds
      // another editor's graph and must not receive this save's hash or refs.
      const currentState = useWorkflowStore.getState();
      if (currentState.workflowData && isWorkflowEditorSessionActive(context?.editorSession)) {
        const nodes = mergeSavedConnectorNodes(
          currentState.nodes,
          variables.workflowData.graph.nodes,
          data.graph?.nodes ?? []
        );
        const acceptedCommands = new Map(
          (variables.workflowData.connector_reference_commands ?? []).map(command => [
            command.reference_id,
            JSON.stringify(command),
          ])
        );
        const remainingCommands = (
          currentState.workflowData.connector_reference_commands ?? []
        ).filter(command => acceptedCommands.get(command.reference_id) !== JSON.stringify(command));
        // Only replace nodes when the server assigned new connector references.
        // A fresh array for an unchanged graph would look like an edit to the
        // save loop and trigger another save.
        const nodesChanged = nodes !== currentState.nodes;
        useWorkflowStore.setState({
          ...(nodesChanged ? { nodes } : {}),
          workflowData: {
            ...currentState.workflowData,
            graph: nodesChanged
              ? { ...currentState.workflowData.graph, nodes }
              : currentState.workflowData.graph,
            connector_reference_commands: remainingCommands,
            hash: data.hash,
          },
          isDirty: false,
          hasLayoutChanges: false,
          suppressNextLayoutDirty: false,
          suppressNextViewportDirty: false,
          lastSavedAt: Date.now(),
        });
      }
    },
    onSettled: (_data, error, variables) => {
      // Only invalidate/refetch on error to ensure we recover a consistent state.
      // Success case is now handled by the optimized merge in onSuccess.
      if (error) {
        queryClient.invalidateQueries({ queryKey: WORKFLOW_KEYS.draft(variables.agentId) });
      }
    },
  });
}

export function useGenerateWorkflowSuggestedQuestions() {
  const t = useT('agents');

  return useMutation({
    mutationFn: ({
      agentId,
      payload,
    }: {
      agentId: string;
      payload: Parameters<typeof workflowService.generateWorkflowSuggestedQuestions>[1];
    }) => workflowService.generateWorkflowSuggestedQuestions(agentId, payload),
    onError: error => {
      const message = (error as unknown as { message?: string }).message?.trim();
      toast.error(
        message
          ? t('workflow.features.suggestedQuestions.generateFailedWithReason', { message })
          : t('workflow.features.suggestedQuestions.generateFailed')
      );
      console.error('Generate workflow suggested questions error:', error);
    },
  });
}

/**
 * Hook for publishing workflow for a specific agent
 * Uses optimistic update pattern to manage latest-version cache and toasts.
 */
export function usePublishWorkflow() {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation({
    mutationFn: ({ agentId, silent: _silent }: { agentId: string; silent?: boolean }) =>
      workflowService.publishWorkflow(agentId),
    onMutate: async ({ agentId }) => {
      const key = WORKFLOW_KEYS.latestVersion(agentId);
      await queryClient.cancelQueries({ queryKey: key });
      const previous = queryClient.getQueryData<ApiResponseData<WorkflowLatestVersion>>(key);
      // We do not change data shape optimistically as server generates new version info.
      // Keep snapshot for potential rollback.
      return { previous, key } as const;
    },
    onError: (error, variables, context) => {
      if (context?.previous && context?.key) {
        queryClient.setQueryData(context.key, context.previous);
      }
      if (!variables?.silent) {
        const message = (error as unknown as { message?: string }).message || t('workflow.failed');
        toast.error(message);
      }
      console.error('Publish workflow error:', error);
    },
    onSuccess: async (_data, variables) => {
      // Invalidate latest-version to refetch fresh shape (web_app_id, etc.)
      await queryClient.invalidateQueries({
        queryKey: WORKFLOW_KEYS.latestVersion(variables.agentId),
      });

      // Also invalidate agent list and detail as publish changes version/status
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() }),
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(variables.agentId) }),
      ]);
    },
  });
}

/**
 * Hook for fetching latest workflow version info by agentId.
 * Returns wrapped ApiResponseData to preserve code/message.
 */
export function useLatestWorkflowVersion(agentId: string | null) {
  return useQuery<ApiResponseData<WorkflowLatestVersion>>({
    queryKey: WORKFLOW_KEYS.latestVersion(agentId ?? 'none'),
    queryFn: () => workflowService.getLatestWorkflowVersion(agentId ?? ''),
    enabled: Boolean(agentId),
    staleTime: 60 * 1000,
    gcTime: 10 * 60 * 1000,
    retry: false,
  });
}
