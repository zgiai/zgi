'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Panel } from '@xyflow/react';
import { X, Copy, Trash2, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useWorkflowStore } from '../store';
import { useWorkflowOperations, usePanelStackItem } from '../hooks';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import VariableManager from '../nodes/start/manager';
import OutputManager from '../nodes/end/manager';
import AnswerManager from '../nodes/answer/manager';
import LLMManager from '../nodes/llm/manager';
import HttpRequestManager from '../nodes/http-request/manager';
import CreateScheduledTaskManager from '../nodes/create-scheduled-task/manager';
import NotificationSMSManager from '../nodes/notification-sms/manager';
import ToolManager from '../nodes/tool/manager';
import IfElseManager from '../nodes/if-else/manager';
import CodeManager from '../nodes/code/manager';
import IterationManager from '../nodes/iteration/manager';
import AssignerManager from '../nodes/assigner/manager';
import CallDatabaseManager from '../nodes/call-database/manager';
import SqlGeneratorManager from '../nodes/sql-generator/manager';
import DocumentExtractorManager from '../nodes/document-extractor/manager';
import VariableAggregatorManager from '../nodes/variable-aggregator/manager';
import ParameterExtractorManager from '../nodes/parameter-extractor/manager';
import JsonParserManager from '../nodes/json-parser/manager';
import type { WorkflowNode, WorkflowNodeData } from '../store/type';
import KnowledgeRetrievalManager from '../nodes/knowledge-retrieval/manager';
import type { AgentType } from '@/services/types/agent';
import { useT } from '@/i18n';
import { useDebouncedCommit } from '../hooks/use-debounced-commit';
import LoopManager from '../nodes/loop/manager';
import ImageGenManager from '../nodes/image-gen/manager';
import ApprovalManager from '../nodes/approval/manager';
import AnnouncementManager from '../nodes/announcement/manager';
import QuestionAnswerManager from '../nodes/question-answer/manager';
import { getRightPanelMotionClassName, getRightPanelMotionStyle } from './right-panel-motion';
import NodeDebugActions from './node-debug-actions';

interface NodeFloatingPanelProps {
  temporarilyHidden?: boolean;
}

/**
 * NodeFloatingPanel - Inline right sidebar for editing workflow node properties
 * Turns the previous floating overlay into a real sidebar that squeezes the editor
 * so it will not cover the workflow header or editor content.
 */
export function NodeFloatingPanel({ temporarilyHidden = false }: NodeFloatingPanelProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const [isResizing, setIsResizing] = useState(false);
  const [width, setWidth] = useState<number>(420);
  const resizeRaf = useRef<number | null>(null); // RAF id for throttling resize updates
  const dragWidthRef = useRef<number>(420); // live width during drag without re-render
  const selectedNodeId = useWorkflowStore.use.selectedNodeId();
  const agentId = useWorkflowStore.use.agentId();
  const nodes = useWorkflowStore.use.nodes();
  const edges = useWorkflowStore.use.edges();
  const selectionSource = useWorkflowStore.use.selectionSource();
  const selectNode = useWorkflowStore.use.selectNode();
  const updateNodeData = useWorkflowStore.use.updateNodeData();
  const agentType = useWorkflowStore.use.agentType();
  const { updateNode, deleteNode, duplicateNode } = useWorkflowOperations();
  const t = useT('nodes');
  // Derive read-only status from workflow mode and permission to gate all edits
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const canDebug = useWorkflowStore.use.canDebug();
  const isReadOnly = mode === 'history' || !canEdit;
  const isNodeDebugReadOnly = mode === 'history' || !canDebug;

  // Sticky selection: keep showing last selected node while performing non-explicit deselection
  const [activeNodeId, setActiveNodeId] = useState<string | null>(null);
  useEffect(() => {
    if (selectedNodeId) {
      setActiveNodeId(selectedNodeId);
    } else if (selectionSource === 'none') {
      // Only clear on explicit pane deselect
      setActiveNodeId(null);
    }
  }, [selectedNodeId, selectionSource]);

  const selectedNode = useWorkflowStore(
    useCallback(
      state => {
        if (state.mode === 'history' && state.selectedRunId) {
          const snap = state.historySnapshots[state.selectedRunId];
          if (snap) {
            return snap.nodes.find(n => n.id === activeNodeId) as WorkflowNode | undefined;
          }
        }
        return state.nodes.find(n => n.id === activeNodeId) as WorkflowNode | undefined;
      },
      [activeNodeId]
    )
  );

  // Ensure panel is open by default on mount
  useEffect(() => {
    setIsExpanded(true);
  }, []);

  // Track previous selection and selection source to control auto open/close behavior
  const prevSelectedRef = useRef<string | null>(null);
  const prevSelSrcRef = useRef<string>('none');
  useEffect(() => {
    const prevSelected = prevSelectedRef.current;

    // Auto-open when a node is explicitly selected via click/create while collapsed
    // This covers both selection changes and re-clicking the same node after a drag
    const isExplicit = selectionSource === 'click' || selectionSource === 'create';
    if (!!selectedNodeId && !isExpanded && isExplicit) {
      setIsExpanded(true);
    }

    // Auto-close when transitioning from a selected node to none
    // Only close on explicit pane deselect (selectionSource === 'none')
    if (prevSelected && !selectedNodeId && isExpanded && selectionSource === 'none') {
      setIsExpanded(false);
    }

    // Update previous refs
    prevSelectedRef.current = selectedNodeId ?? null;
    prevSelSrcRef.current = selectionSource as string;
  }, [selectedNodeId, isExpanded, selectionSource]);

  // Derived open state: keep panel open during graph interactions (including drags)
  const open =
    isExpanded &&
    !!activeNodeId &&
    selectedNode?.data.type !== 'loop-end' &&
    selectedNode?.data.type !== 'note';

  // Check if there is content to render
  const hasContent = React.useMemo(() => {
    if (!selectedNode) return false;
    const type = selectedNode.data.type;
    return type !== 'note' && type !== 'loop-end';
  }, [selectedNode]);

  // Register with panel stack to allow horizontal layout with other panels
  const { panelStyle } = usePanelStackItem({
    id: 'node-floating-panel',
    position: 'top-right',
    order: 2,
    visible: open && hasContent, // Ensure it registers as hidden if no content
    width,
    gap: 8,
  });

  // Ensure pending RAF is cleaned up on unmount
  useEffect(() => {
    return () => {
      if (resizeRaf.current) cancelAnimationFrame(resizeRaf.current);
    };
  }, []);

  // Keep CSS var in sync with committed width so initial render matches
  useEffect(() => {
    document.documentElement.style.setProperty('--node-panel-w', `${width}px`);
    dragWidthRef.current = width;
  }, [width]);

  // Resize logic (throttled via RAF for performance)
  const onMouseDownResize = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      e.preventDefault();
      e.stopPropagation();
      setIsResizing(true);
      const startX = e.clientX;
      const startWidth = width;
      const onMouseMove = (ev: MouseEvent) => {
        if (resizeRaf.current) cancelAnimationFrame(resizeRaf.current);
        resizeRaf.current = requestAnimationFrame(() => {
          const delta = startX - ev.clientX;
          const next = Math.max(420, Math.min(720, startWidth + delta));
          // Avoid React re-render during drag; update CSS var directly
          dragWidthRef.current = next;
          document.documentElement.style.setProperty('--node-panel-w', `${next}px`);
        });
      };
      const onMouseUp = () => {
        setIsResizing(false);
        // Commit final width to state once
        setWidth(dragWidthRef.current);
        window.removeEventListener('mousemove', onMouseMove);
        window.removeEventListener('mouseup', onMouseUp);
      };
      window.addEventListener('mousemove', onMouseMove);
      window.addEventListener('mouseup', onMouseUp);
    },
    [width]
  );

  // Node update helpers
  const handleNodeUpdate = useCallback(
    (key: keyof WorkflowNodeData | 'data', value: unknown) => {
      // Use activeNodeId from local state as it is more stable than selectedNodeId from store
      // but still represents the currently displayed node.
      if (!activeNodeId || isReadOnly) return;
      if (key === 'data') {
        // Merge partial node data via store helper for type safety
        updateNodeData(activeNodeId, value as Partial<WorkflowNodeData>);
      } else {
        // Future non-data updates fallback
        updateNode(activeNodeId, {
          [key]: value,
        } as Partial<WorkflowNode>);
      }
    },
    [activeNodeId, updateNode, updateNodeData, isReadOnly]
  );

  const handleDuplicate = useCallback(() => {
    if (!selectedNode || isReadOnly) return;
    if (selectedNode.data.type === 'start') return;
    duplicateNode(selectedNode.id);
  }, [duplicateNode, selectedNode, isReadOnly]);

  // Helper: close panel and clear current selection to keep panel and selection in sync
  const closePanel = useCallback(() => {
    setIsExpanded(false);
    // Ensure editor selection is cleared when panel is closed
    selectNode(null);
    setActiveNodeId(null);
  }, [selectNode]);

  const handleDelete = useCallback(() => {
    if (!selectedNode || isReadOnly) return;
    if (selectedNode.data.type === 'start') return;
    deleteNode(selectedNode.id);
    closePanel();
  }, [deleteNode, selectedNode, isReadOnly, closePanel]);

  const renderNodeSpecificFields = useCallback(
    (node: WorkflowNode | undefined) => {
      if (!node) return null;

      // Render node-specific settings panel，by node type in data
      switch (node.data.type) {
        case 'start':
          return (
            <VariableManager
              id={node.id}
              agentType={agentType as AgentType}
              readOnly={isReadOnly}
            />
          );
        case 'knowledge-retrieval':
          return <KnowledgeRetrievalManager id={node.id} readOnly={isReadOnly} />;
        case 'llm':
          return <LLMManager id={node.id} readOnly={isReadOnly} />;
        case 'http-request':
          return <HttpRequestManager id={node.id} readOnly={isReadOnly} />;
        case 'create-scheduled-task':
          return <CreateScheduledTaskManager id={node.id} readOnly={isReadOnly} />;
        case 'notification-sms':
          return <NotificationSMSManager id={node.id} readOnly={isReadOnly} />;
        case 'call-database':
          return <CallDatabaseManager id={node.id} readOnly={isReadOnly} />;
        case 'sql-generator':
          return <SqlGeneratorManager id={node.id} readOnly={isReadOnly} />;
        case 'tools':
          return <ToolManager id={node.id} readOnly={isReadOnly} />;
        case 'end':
          return <OutputManager id={node.id} readOnly={isReadOnly} />;
        case 'answer':
          return <AnswerManager id={node.id} readOnly={isReadOnly} />;
        case 'if-else':
          return <IfElseManager id={node.id} readOnly={isReadOnly} />;
        case 'code':
          return <CodeManager id={node.id} readOnly={isReadOnly} />;
        case 'iteration':
          return <IterationManager id={node.id} readOnly={isReadOnly} />;
        case 'loop':
          return <LoopManager id={node.id} readOnly={isReadOnly} />;
        case 'assigner':
          return <AssignerManager id={node.id} readOnly={isReadOnly} />;
        case 'document-extractor':
          return <DocumentExtractorManager id={node.id} readOnly={isReadOnly} />;
        case 'variable-aggregator':
          return <VariableAggregatorManager id={node.id} readOnly={isReadOnly} />;
        case 'parameter-extractor':
          return <ParameterExtractorManager id={node.id} readOnly={isReadOnly} />;
        case 'json-parser':
          return <JsonParserManager id={node.id} readOnly={isReadOnly} />;
        case 'image-gen':
          return <ImageGenManager id={node.id} readOnly={isReadOnly} />;
        case 'approval':
          return <ApprovalManager id={node.id} readOnly={isReadOnly} />;
        case 'announcement':
          return <AnnouncementManager id={node.id} readOnly={isReadOnly} />;
        case 'question-answer':
          return <QuestionAnswerManager id={node.id} readOnly={isReadOnly} />;
        default:
          return (
            <p className="text-sm text-muted-foreground">No settings for this node type yet</p>
          );
      }
    },
    [agentType, isReadOnly]
  );

  const deferredSelectedNode = React.useDeferredValue(selectedNode);

  // Debounced local fields for title/desc to reduce store churn
  const { value: localTitle, setValue: setLocalTitle } = useDebouncedCommit(
    (selectedNode?.data.title as string) || '',
    {
      delay: 300,
      onCommit: v => handleNodeUpdate('data', { title: v }),
      isEqual: (a, b) => a === b,
      flushOnUnmount: true,
    }
  );
  const { value: localDesc, setValue: setLocalDesc } = useDebouncedCommit(
    (selectedNode?.data.desc as string) || '',
    {
      delay: 300,
      onCommit: v => handleNodeUpdate('data', { desc: v }),
      isEqual: (a, b) => a === b,
      flushOnUnmount: true,
    }
  );
  useEffect(() => {
    // keep hook in sync when selection changes
  }, [selectedNode?.id]);

  // Do not render when closed
  if (!open) return null;

  // If open is true but no content, return null to avoid rendering empty panel
  if (open && !hasContent) {
    return null;
  }

  return (
    <Panel
      position="top-right"
      data-workflow-shortcut-scope="node-panel"
      aria-hidden={temporarilyHidden}
      className={getRightPanelMotionClassName(
        'relative glass-panel h-[calc(100%-120px)] rounded-xl flex flex-col overflow-hidden',
        temporarilyHidden
      )}
      style={{
        ...getRightPanelMotionStyle(panelStyle, temporarilyHidden),
        width: 'min(var(--node-panel-w, 420px), calc(100vw - 16px))',
      }}
      onKeyDown={e => {
        const target = e.target as HTMLElement | null;
        const isCtrlOrCmd = e.ctrlKey || e.metaKey;

        if (e.key === 'Delete' || e.key === 'Backspace') {
          e.stopPropagation();
          return;
        }

        if (isCtrlOrCmd && (e.key === 'c' || e.key === 'C' || e.key === 'v' || e.key === 'V')) {
          e.stopPropagation();
          return;
        }

        // Allow key events inside Monaco editor and form inputs to pass through
        if (
          target &&
          (target.closest('.monaco-editor') ||
            target.tagName === 'INPUT' ||
            target.tagName === 'TEXTAREA' ||
            target.getAttribute('contenteditable') === 'true')
        ) {
          return;
        }
        e.stopPropagation();
      }}
      onContextMenu={e => e.stopPropagation()}
    >
      {/* Resize handle */}
      <div
        onMouseDown={onMouseDownResize}
        className={
          'absolute left-0 top-0 h-full w-1.5 cursor-ew-resize bg-transparent hover:bg-gray-200 ' +
          (isResizing ? 'bg-gray-300' : '')
        }
      />

      {/* Header */}
      <div className="px-2 pt-3 pb-2 border-b">
        <div className="flex items-center gap-2">
          <input
            type="text"
            className="bg-transparent hover:bg-muted px-1.5 py-0.5 rounded text-base w-0 grow focus:bg-muted"
            value={localTitle}
            onChange={e => setLocalTitle(e.target.value.slice(0, 30))}
            placeholder={t('common.namePlaceholder')}
            disabled={isReadOnly}
            maxLength={30}
          />
          <div className="flex gap-1">
            {selectedNode && !isReadOnly && selectedNode.data.type !== 'start' && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" isIcon className="w-7 h-7" aria-label="More">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="min-w-[160px]">
                  <DropdownMenuItem onClick={handleDuplicate}>
                    <Copy className="h-4 w-4" />
                    {t('common.copy')}
                  </DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onClick={handleDelete}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                    {t('common.delete')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            <Button
              variant="ghost"
              isIcon
              className="w-7 h-7"
              onClick={closePanel}
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
        <div className="mt-1">
          <input
            id="node-desc"
            value={localDesc}
            onChange={e => setLocalDesc(e.target.value.slice(0, 200))}
            placeholder={t('common.descriptionPlaceholder')}
            className="w-full placeholder:text-muted-foreground px-1.5 py-1 rounded text-xs hover:bg-muted focus:bg-muted"
            disabled={isReadOnly}
            maxLength={200}
          />
        </div>
      </div>

      {/* Content */}
      <div className="flex flex-col h-0 grow">
        {selectedNode ? (
          <div className="space-y-4 overflow-auto h-0 grow px-4 pt-4 pb-40">
            <NodeDebugActions
              agentId={agentId}
              node={selectedNode}
              nodes={nodes}
              edges={edges}
              readOnly={isNodeDebugReadOnly}
            />
            {/* Type-specific settings */}
            <div
              className={cn('space-y-3 transition-opacity duration-200', {
                'opacity-60': deferredSelectedNode !== selectedNode,
              })}
              // Use id as key to force remount when switching tools nodes
              key={
                deferredSelectedNode?.data.type === 'tools'
                  ? deferredSelectedNode.id
                  : deferredSelectedNode?.data.type
              }
            >
              {renderNodeSpecificFields(deferredSelectedNode)}
            </div>
          </div>
        ) : (
          <div className="p-4 text-center">
            <p>{t('common.selectNodeToEdit')}</p>
          </div>
        )}
      </div>
    </Panel>
  );
}

export default NodeFloatingPanel;
