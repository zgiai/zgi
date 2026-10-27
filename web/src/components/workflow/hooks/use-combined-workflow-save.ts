import { useCallback, useRef } from 'react';
import { useWorkflowStore } from '../store';
import { usePublishWorkflow, useSaveWorkflowDraft } from '@/hooks';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { flushWorkflowPendingEdits } from './pending-edits';
import type { WorkflowData } from '../store/type';

/**
 * Upper bound on consecutive draft saves one call may run. A follow-up pass is
 * only needed when the user edits while a save is in flight; if the store keeps
 * reporting changes beyond this, stop and leave the dirty flags for the next save
 * instead of hammering the API.
 */
const MAX_SAVE_PASSES = 5;

/**
 * Comparable key for workflow metadata outside the graph. Connector reference
 * commands the server accepted are dropped from the store after a successful
 * save, so they are excluded here (and a missing list equals an empty one).
 */
function workflowMetadataKey(
  data: WorkflowData | null | undefined,
  acceptedCommands: ReadonlySet<string>
): string {
  return JSON.stringify({
    ...data,
    graph: undefined,
    hash: undefined,
    connector_reference_commands: (data?.connector_reference_commands ?? []).filter(
      command => !acceptedCommands.has(JSON.stringify(command))
    ),
  });
}

/**
 * Combined workflow save hook that handles both local store update and API save
 * This replaces the need for separate save operations
 */
export const useCombinedWorkflowSave = (agentId: string) => {
  const isDirty = useWorkflowStore.use.isDirty();
  const saveWorkflowMutation = useSaveWorkflowDraft();
  const t = useT('agents');
  const publishWorkflowMutation = usePublishWorkflow();
  const inFlightSaveRef = useRef<Promise<boolean> | null>(null);

  const handleCombinedSave = useCallback(
    async (options?: { silent?: boolean; saveToast?: string }): Promise<boolean> => {
      const silent = options?.silent ?? false;
      const saveToast = options?.saveToast;
      let savedAtLeastOnce = false;
      let savePasses = 0;

      for (;;) {
        const activeSave = inFlightSaveRef.current;
        if (activeSave) {
          if (!(await activeSave)) return false;
          // Re-enter through the shared barrier. The save owner may have
          // started a follow-up snapshot while this caller was waiting.
          continue;
        }

        flushWorkflowPendingEdits();
        const stateAfterFlush = useWorkflowStore.getState();
        if (!stateAfterFlush.isDirty && !stateAfterFlush.hasLayoutChanges) {
          if (!savedAtLeastOnce) {
            useWorkflowStore.setState({ lastSavedAt: Date.now() });
          }
          if (savedAtLeastOnce && !silent && saveToast) toast.success(saveToast);
          return true;
        }

        if (savePasses >= MAX_SAVE_PASSES) {
          // The latest successful save is persisted; newer local changes keep
          // their dirty flags and will be picked up by the next save.
          console.warn('Workflow draft still changing after repeated saves; deferring.');
          if (!silent && saveToast) toast.success(saveToast);
          return true;
        }
        savePasses += 1;

        const {
          isDirty: prevIsDirty,
          hasLayoutChanges: prevHasLayoutChanges,
          lastSavedAt: prevLastSavedAt,
          nodes,
          edges,
          viewport,
          workflowData,
          graphVersion,
        } = stateAfterFlush;
        const updatedWorkflowData = {
          ...workflowData,
          graph: { nodes, edges, viewport },
          hash: Date.now().toString(),
        } as typeof workflowData;
        const acceptedCommands = new Set(
          (workflowData?.connector_reference_commands ?? []).map(command => JSON.stringify(command))
        );
        const submittedMetadata = workflowMetadataKey(workflowData, acceptedCommands);

        const savePromise = (async (): Promise<boolean> => {
          useWorkflowStore.setState({
            workflowData: updatedWorkflowData,
            isDirty: false,
            hasLayoutChanges: false,
            suppressNextLayoutDirty: false,
            suppressNextViewportDirty: false,
            lastSavedAt: Date.now(),
          });

          try {
            await saveWorkflowMutation.mutateAsync({
              agentId,
              workflowData: updatedWorkflowData,
              silent,
            });

            // The mutation success handler clears dirty flags. Restore them if
            // the graph changed while this exact snapshot was being persisted;
            // the loop will then save the newer snapshot before publishing.
            const current = useWorkflowStore.getState();
            const currentMetadata = workflowMetadataKey(current.workflowData, acceptedCommands);
            const semanticChanged =
              current.graphVersion !== graphVersion || currentMetadata !== submittedMetadata;
            const graphOrLayoutChanged =
              current.nodes !== nodes || current.edges !== edges || current.viewport !== viewport;
            if (semanticChanged || graphOrLayoutChanged) {
              useWorkflowStore.setState({
                isDirty: semanticChanged,
                hasLayoutChanges: !semanticChanged && graphOrLayoutChanged,
              });
            }
            return true;
          } catch (error) {
            useWorkflowStore.setState({
              isDirty: prevIsDirty,
              hasLayoutChanges: prevHasLayoutChanges,
              lastSavedAt: prevLastSavedAt ?? null,
            });
            // The mutation hook owns user-facing error feedback.
            console.error('Failed to save workflow:', error);
            return false;
          }
        })();

        inFlightSaveRef.current = savePromise;
        try {
          if (!(await savePromise)) return false;
          savedAtLeastOnce = true;
        } finally {
          if (inFlightSaveRef.current === savePromise) inFlightSaveRef.current = null;
        }
      }
    },
    [agentId, saveWorkflowMutation]
  );

  const handlePublish = useCallback(
    async (options?: { silent?: boolean; successToast?: string }) => {
      const silent = options?.silent ?? false;
      // Ensure the latest edits are saved quietly to avoid duplicate toasts
      const saved = await handleCombinedSave({ silent: true });
      if (!saved) {
        if (!silent) toast.error(t('workflow.workflowDraftSaveFailed'));
        return;
      }
      // Publish is the user-facing action. Keep its success feedback here so
      // the prerequisite draft save cannot produce a second toast.
      const res = await publishWorkflowMutation.mutateAsync({ agentId, silent: true });
      if (!silent && res?.code === '0') {
        toast.success(options?.successToast ?? t('workflow.workflowPublishedSuccessfully'));
      }
    },
    [agentId, handleCombinedSave, publishWorkflowMutation, t]
  );

  return {
    handleCombinedSave,
    handlePublish,
    isSaving: saveWorkflowMutation.isPending,
    isPublishing: publishWorkflowMutation.isPending,
    // Expose semantic-only dirty to UI; layout-only changes (viewport/position/size)
    // should not show as dirty badge while still being saved by handleCombinedSave.
    isDirty: isDirty,
  };
};

export default useCombinedWorkflowSave;
