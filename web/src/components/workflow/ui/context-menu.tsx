import React, { useState, useCallback, useEffect } from 'react';
import { Plus, Trash2, Copy as CopyIcon, StickyNote } from 'lucide-react';
import { useReactFlow } from '@xyflow/react';
import { useWorkflowStore } from '../store/store';
import { useCreateNodeModal } from '../hooks/use-create-node-modal';
import { useT } from '@/i18n';
import useWorkflowOperations from '../hooks/use-workflow-operations';
import { NODE_TYPES } from '../nodes';

interface WorkflowContextMenuProps {
  children: React.ReactNode;
  onNodeContextMenu?: (event: React.MouseEvent, nodeId: string) => void;
  disabled?: boolean;
}

export function WorkflowContextMenu({
  children,
  onNodeContextMenu,
  disabled = false,
}: WorkflowContextMenuProps) {
  const [clickPosition, setClickPosition] = useState<{ x: number; y: number }>({ x: 0, y: 0 });
  const [contextNodeId, setContextNodeId] = useState<string | null>(null);
  const [isCanvasMenuOpen, setIsCanvasMenuOpen] = useState(false);
  const { agents: t, common } = useT();
  const nodes = useWorkflowStore.use.nodes();
  const selectNode = useWorkflowStore.use.selectNode();
  const setLastMouseClient = useWorkflowStore.use.setLastMouseClient();
  const { deleteNode, copySelectedNode, pasteClipboardAtPointer } = useWorkflowOperations();
  const { openModal } = useCreateNodeModal();
  const setIsContextMenuOpen = useWorkflowStore.use.setIsContextMenuOpen();
  const addNode = useWorkflowStore.use.addNode();
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const isReadOnly = mode === 'history' || !canEdit;
  const effectiveDisabled = disabled || isReadOnly;
  const { screenToFlowPosition } = useReactFlow();

  const handleCanvasContextMenu = useCallback(
    (event: React.MouseEvent) => {
      if (effectiveDisabled) {
        return;
      }

      // Check if the target is the React Flow pane or background
      // This prevents the menu from appearing when right-clicking on panels
      const target = event.target as HTMLElement;
      const isCanvasClick =
        target.classList.contains('react-flow__pane') ||
        !!target.closest('.react-flow__background') ||
        target.tagName === 'svg' ||
        target.tagName === 'circle' || // background pattern dots
        target.tagName === 'path' || // background pattern lines
        !!target.closest('.react-flow__edges');

      if (!isCanvasClick) {
        return;
      }

      event.preventDefault();

      // Use global coordinates for fixed positioning and update pointer
      const x = event.clientX;
      const y = event.clientY;

      setClickPosition({ x, y });
      setLastMouseClient({ x, y });
      setContextNodeId(null);
      setIsCanvasMenuOpen(true);
      setIsContextMenuOpen(true);
    },
    [effectiveDisabled, setIsContextMenuOpen, setLastMouseClient]
  );

  const handleNodeContextMenu = useCallback(
    (event: React.MouseEvent, nodeId: string) => {
      if (effectiveDisabled) {
        return;
      }

      if (event && typeof event.stopPropagation === 'function') {
        event.stopPropagation();
        event.preventDefault();
      }

      // Check if event exists and has clientX/clientY properties
      if (!event || typeof event.clientX === 'undefined' || typeof event.clientY === 'undefined') {
        console.warn('Invalid event object in handleNodeContextMenu');
        return;
      }

      // Use global coordinates for fixed positioning and update pointer
      const x = event.clientX;
      const y = event.clientY;

      setClickPosition({ x, y });
      setLastMouseClient({ x, y });
      setContextNodeId(nodeId);
      setIsContextMenuOpen(true);

      // If parent provided a callback, call it for additional side effects
      if (onNodeContextMenu) {
        onNodeContextMenu(event, nodeId);
      }
    },
    [effectiveDisabled, onNodeContextMenu, setIsContextMenuOpen, setLastMouseClient]
  );

  // Expose the node context menu handler globally for programmatic triggering
  useEffect(() => {
    const windowWithContextMenu = window as Window & {
      __workflowNodeContextMenu?: (event: React.MouseEvent, nodeId: string) => void;
    };
    if (effectiveDisabled) {
      delete windowWithContextMenu.__workflowNodeContextMenu;
      setIsCanvasMenuOpen(false);
      setContextNodeId(null);
      setIsContextMenuOpen(false);
      return;
    }

    windowWithContextMenu.__workflowNodeContextMenu = handleNodeContextMenu;
    return () => {
      if (windowWithContextMenu.__workflowNodeContextMenu === handleNodeContextMenu) {
        delete windowWithContextMenu.__workflowNodeContextMenu;
      }
    };
  }, [effectiveDisabled, handleNodeContextMenu, setIsContextMenuOpen]);

  const handleAddNode = useCallback(() => {
    if (effectiveDisabled) return;
    // Open CreateNodeModal at the last right-click position
    const position = { x: clickPosition.x, y: clickPosition.y };
    openModal(position, null, position);
    // Close context menus after opening modal
    setIsCanvasMenuOpen(false);
    setContextNodeId(null);
    setIsContextMenuOpen(false);
  }, [clickPosition, effectiveDisabled, openModal, setIsContextMenuOpen]);

  const handleAddNote = useCallback(() => {
    if (effectiveDisabled) return;
    const position = screenToFlowPosition({ x: clickPosition.x, y: clickPosition.y });
    addNode({ type: 'note', text: '' }, position);
    setIsCanvasMenuOpen(false);
    setContextNodeId(null);
  }, [clickPosition, addNode, effectiveDisabled, screenToFlowPosition]);

  const handlePasteAtPointer = useCallback(() => {
    if (effectiveDisabled) return;
    // Paste using lastMouseClient recorded at context open
    pasteClipboardAtPointer();
    setIsCanvasMenuOpen(false);
    setContextNodeId(null);
  }, [effectiveDisabled, pasteClipboardAtPointer]);

  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (_event: MouseEvent) => {
      if (isCanvasMenuOpen || contextNodeId) {
        setIsCanvasMenuOpen(false);
        setContextNodeId(null);
        setIsContextMenuOpen(false);
      }
    };

    if (isCanvasMenuOpen || contextNodeId) {
      document.addEventListener('click', handleClickOutside);
      return () => document.removeEventListener('click', handleClickOutside);
    }
  }, [isCanvasMenuOpen, contextNodeId, setIsContextMenuOpen]);

  const handleDeleteNode = useCallback(() => {
    if (effectiveDisabled) return;
    if (contextNodeId) {
      deleteNode(contextNodeId);
      setContextNodeId(null); // Reset context node after deletion
    }
  }, [contextNodeId, deleteNode, effectiveDisabled]);

  const handleCopyNode = useCallback(() => {
    if (!contextNodeId || effectiveDisabled) return;
    // Select the context node, then copy via operations hook (ignores Start internally)
    selectNode(contextNodeId);
    copySelectedNode();
    setIsCanvasMenuOpen(false);
    setContextNodeId(null);
  }, [contextNodeId, effectiveDisabled, selectNode, copySelectedNode]);

  return (
    <>
      <div
        onContextMenu={handleCanvasContextMenu}
        className="w-full h-full relative"
        style={{ pointerEvents: 'auto' }}
      >
        {children}
      </div>
      {/* Custom Context Menu */}
      {!effectiveDisabled && (isCanvasMenuOpen || contextNodeId) && (
        <div
          className="fixed z-50 min-w-[8rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
          style={{
            left: clickPosition.x,
            top: clickPosition.y,
          }}
          onMouseLeave={() => {
            setIsCanvasMenuOpen(false);
            setContextNodeId(null);
            setIsContextMenuOpen(false);
          }}
        >
          {!contextNodeId ? (
            // Canvas context menu
            <>
              <div
                className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground"
                onClick={handleAddNode}
              >
                <Plus className="mr-2 h-4 w-4" />
                {t('workflow.addNode')}
              </div>
              <div
                className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground"
                onClick={handleAddNote}
              >
                <StickyNote className="mr-2 h-4 w-4" />
                {t('workflow.addNoteNode')}
              </div>
              <div
                className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground"
                onClick={handlePasteAtPointer}
              >
                {/* Use plain text to avoid missing i18n key */}
                <CopyIcon className="mr-2 h-4 w-4" />
                {common('paste')}
              </div>
            </>
          ) : (
            // Node context menu
            (() => {
              const node = nodes.find(n => n.id === contextNodeId);
              const nodeType = (node?.data as { type?: string })?.type;
              const isStart = nodeType === NODE_TYPES.START;
              // Container start nodes also cannot be copied or deleted
              const isContainerStart =
                nodeType?.endsWith('-start') || nodeType === 'iteration-start';
              if (isStart || isContainerStart) {
                // Empty state for Start nodes - cannot be copied or deleted
                return (
                  <div className="px-3 py-2 text-sm text-muted-foreground">
                    {t('workflow.startNodeNoActions')}
                  </div>
                );
              }
              return (
                <>
                  <div
                    className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground"
                    onClick={handleCopyNode}
                  >
                    <CopyIcon className="mr-2 h-4 w-4" />
                    {common('copy')}
                  </div>
                  <div
                    className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground text-red-600"
                    onClick={handleDeleteNode}
                  >
                    <Trash2 className="mr-2 h-4 w-4" />
                    {t('workflow.deleteNode')}
                  </div>
                </>
              );
            })()
          )}
        </div>
      )}
    </>
  );
}
