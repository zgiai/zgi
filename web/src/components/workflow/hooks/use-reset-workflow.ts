import { useCallback } from 'react';
import { toast } from 'sonner';
import { useWorkflowStore } from '../store';

/**
 * Encapsulate reset confirmation and toast side-effects.
 * Calls the store.resetWorkflow which performs pure state updates only.
 */
const useResetWorkflow = () => {
  const isDirty = useWorkflowStore.use.isDirty();
  const resetWorkflow = useWorkflowStore.use.resetWorkflow();

  const resetWithConfirm = useCallback(() => {
    if (isDirty) {
      const ok = window.confirm('确定要重置工作流吗？未保存的更改将丢失。');
      if (!ok) return;
    }
    resetWorkflow();
    toast.success('工作流已重置');
  }, [isDirty, resetWorkflow]);

  return { resetWithConfirm };
};

export default useResetWorkflow;
