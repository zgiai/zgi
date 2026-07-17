import { useEffect, useCallback } from 'react';
import { useWorkflowStore } from '../store';
import useWorkflowOperations from './use-workflow-operations';
import { saveWorkflowInteractionMode } from '@/utils/ui-local';

const isTextEditingElement = (element: HTMLElement | null) => {
  if (!element) return false;

  return (
    element.tagName === 'INPUT' ||
    element.tagName === 'TEXTAREA' ||
    element.tagName === 'SELECT' ||
    element.isContentEditable ||
    element.closest('input, textarea, select, [contenteditable="true"], .monaco-editor, .ProseMirror') !==
      null
  );
};

const isNodePanelFocused = () => {
  if (typeof document === 'undefined') return false;

  const panel = document.querySelector<HTMLElement>('[data-workflow-shortcut-scope="node-panel"]');
  return panel?.matches(':focus-within') ?? false;
};

/**
 * Hook for handling keyboard shortcuts in workflow editor
 */
const useWorkflowKeyboard = (options?: { onSave?: () => void; disabled?: boolean }) => {
  const onSave = options?.onSave;
  const disabled = options?.disabled;
  const selectedNodeId = useWorkflowStore.use.selectedNodeId();
  const nodes = useWorkflowStore.use.nodes();
  const selectNode = useWorkflowStore.use.selectNode();
  const undo = useWorkflowStore.use.undo();
  const redo = useWorkflowStore.use.redo();
  const setInteractionMode = useWorkflowStore.use.setInteractionMode();
  const edges = useWorkflowStore.use.edges();
  const onEdgesChange = useWorkflowStore.use.onEdgesChange();
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const isReadOnly = Boolean(disabled || mode === 'history' || !canEdit);

  const {
    deleteSelectedNode,
    deleteSelectedNodes,
    copySelectedNode,
    pasteClipboardAtPointer,
    handleCombinedSave: handleSaveWorkflow,
  } = useWorkflowOperations();

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if (isReadOnly) return;

      const target = event.target as HTMLElement;
      const activeElement =
        document.activeElement instanceof HTMLElement ? document.activeElement : null;

      if (isTextEditingElement(target) || isTextEditingElement(activeElement)) {
        return;
      }

      if (event.key === 'Delete' || event.key === 'Backspace') {
        if (isNodePanelFocused()) {
          return;
        }

        event.preventDefault();

        const selectedCount = nodes.filter(n => n.selected).length;
        if (selectedCount > 1) {
          deleteSelectedNodes();
        } else if (selectedNodeId) {
          deleteSelectedNode();
        } else if (selectedCount === 1) {
          deleteSelectedNodes();
        }

        const selectedEdges = edges.filter(e => e.selected);
        if (selectedEdges.length > 0) {
          onEdgesChange(selectedEdges.map(e => ({ id: e.id, type: 'remove' })));
        }
      }

      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'c') {
        const selection = window.getSelection();
        if (selection && selection.toString().length > 0) {
          return;
        }

        event.preventDefault();
        copySelectedNode();
      }

      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'v') {
        event.preventDefault();
        pasteClipboardAtPointer();
      }

      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
        event.preventDefault();
        if (onSave) {
          onSave();
        } else {
          handleSaveWorkflow();
        }
      }

      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z' && !event.shiftKey) {
        event.preventDefault();
        undo();
      }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z' && event.shiftKey) {
        event.preventDefault();
        redo();
      }

      if (!event.ctrlKey && !event.metaKey && !event.altKey) {
        if (event.key.toLowerCase() === 'm') {
          setInteractionMode('mouse');
          saveWorkflowInteractionMode('mouse');
        } else if (event.key.toLowerCase() === 't') {
          setInteractionMode('trackpad');
          saveWorkflowInteractionMode('trackpad');
        }
      }

      if (event.key === 'Escape') {
        event.preventDefault();
        selectNode(null);
      }
    },
    [
      onSave,
      selectedNodeId,
      nodes,
      deleteSelectedNode,
      deleteSelectedNodes,
      copySelectedNode,
      pasteClipboardAtPointer,
      handleSaveWorkflow,
      selectNode,
      undo,
      redo,
      setInteractionMode,
      edges,
      onEdgesChange,
      isReadOnly,
    ]
  );

  useEffect(() => {
    if (isReadOnly) return;
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [handleKeyDown, isReadOnly]);

  return {
    shortcuts: {
      delete: 'Delete/Backspace',
      copy: 'Ctrl+C',
      save: 'Ctrl+S',
      undo: 'Ctrl+Z',
      redo: 'Ctrl+Shift+Z',
      modeMouse: 'M',
      modeTrackpad: 'T',
      deselect: 'Escape',
    },
  };
};

export default useWorkflowKeyboard;
