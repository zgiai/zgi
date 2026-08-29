import { useCallback, useRef } from 'react';
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
  const inFlightSaveRef = useRef<Promise<boolean> | null>(null);

  const handleCombinedSave = useCallback(
    async (options?: { silent?: boolean; saveToast?: string }): Promise<boolean> => {
      const silent = options?.silent ?? false;
      const saveToast = options?.saveToast;
      let savedAtLeastOnce = false;

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
        const submittedMetadata = JSON.stringify({
          ...workflowData,
          graph: undefined,
          hash: undefined,
        });

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
            const currentMetadata = JSON.stringify({
              ...current.workflowData,
              graph: undefined,
              hash: undefined,
            });
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
