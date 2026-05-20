import { useCallback } from 'react';
import { useWorkflowStore } from '../store';
import { usePublishWorkflow, useSaveWorkflowDraft } from '@/hooks';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { flushWorkflowPendingEdits } from './pending-edits';

/**
 * Combined workflow save hook that handles both local store update and API save
 * This replaces the need for separate save operations
 */
export const useCombinedWorkflowSave = (agentId: string) => {
  const isDirty = useWorkflowStore.use.isDirty();
  const saveWorkflowMutation = useSaveWorkflowDraft();
  const t = useT('agents');
  const publishWorkflowMutation = usePublishWorkflow();

  const handleCombinedSave = useCallback(
    async (options?: { silent?: boolean; saveToast?: string }) => {
      const silent = options?.silent ?? false;
      const saveToast = options?.saveToast;
      // Avoid concurrent saves
      if (saveWorkflowMutation.isPending) return;

      flushWorkflowPendingEdits();

      const stateAfterFlush = useWorkflowStore.getState();

      if (!stateAfterFlush.isDirty && !stateAfterFlush.hasLayoutChanges) {
        useWorkflowStore.setState({ lastSavedAt: Date.now() });
        return;
      }

      // Capture previous flags for rollback on failure
      const {
        isDirty: prevIsDirty,
        hasLayoutChanges: prevHasLayoutChanges,
        lastSavedAt: prevLastSavedAt,
      } = stateAfterFlush;

      try {
        // Get current state from store directly to ensure we have the latest data
        const { nodes, edges, viewport, workflowData } = useWorkflowStore.getState();

        // Build the updated workflow data with current state
        const updatedWorkflowData = {
          ...workflowData,
          graph: {
            nodes,
            edges,
            viewport,
          },
          hash: Date.now().toString(),
        } as typeof workflowData;

        // Update local store optimistically with the new data and clear flags
        useWorkflowStore.setState({
          workflowData: updatedWorkflowData,
          isDirty: false,
          hasLayoutChanges: false,
          suppressNextLayoutDirty: false,
          suppressNextViewportDirty: false,
          lastSavedAt: Date.now(),
        });

        // Then save to API with the updated data
        await saveWorkflowMutation.mutateAsync({
          agentId,
          workflowData: updatedWorkflowData,
          silent,
        });

        // Show success toast if not silent
        if (!silent && saveToast) {
          toast.success(saveToast);
        }
      } catch (error) {
        // Restore flags on failure to reflect unsaved changes
        useWorkflowStore.setState({
          isDirty: prevIsDirty,
          hasLayoutChanges: prevHasLayoutChanges,
          lastSavedAt: prevLastSavedAt ?? null,
        });
        // Errors are handled in the mutation hook; no extra toasts here to avoid duplicates
        console.error('Failed to save workflow:', error);
      }
    },
    [agentId, saveWorkflowMutation]
  );

  const handlePublish = useCallback(
    async (options?: { silent?: boolean }) => {
      const silent = options?.silent ?? false;
      // Ensure the latest edits are saved quietly to avoid duplicate toasts
      await handleCombinedSave({ silent: true });
      // Call publish API and show success toast here
      const res = await publishWorkflowMutation.mutateAsync({ agentId, silent: true });
      if (!silent && res?.code === '0') {
        toast.success(t('workflow.workflowPublishedSuccessfully'));
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
