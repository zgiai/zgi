'use client';

import React, { useCallback, useEffect, useLayoutEffect } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { ReactFlowProvider } from '@xyflow/react';
import { type Viewport } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { WorkflowHeader } from './ui';
import { useCombinedWorkflowSave, WorkflowEditorProvider } from './hooks';
import { useWorkflowStore } from './store';
import { useWorkflowDraft } from '@/hooks';
import useWorkflowValidation from './hooks/use-workflow-validation';
import { toast } from 'sonner';
import { WORKFLOW_AUTOSAVE_INTERVAL_MS } from '@/lib/config';
import type { AgentDetail } from '@/services';
import { AgentType } from '@/services/types/agent';
import { useT } from '@/i18n';
import { WorkflowSkeleton } from './ui/workflow-skeleton';
import WorkflowKeyboardBindings from './keyboard-bindings';
import CanvasWithDnd from './canvas-with-dnd';
import { useActivePanel } from './hooks/use-active-panel';
import { useWorkflowLifecycle } from './hooks/use-workflow-lifecycle';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkflowLeaveGuard } from './hooks/use-workflow-leave-guard';
import { isWorkflowDebugPanelActive } from './hooks/use-debug-focus-mode';
import { getNodeAbsolutePosition } from './store/helpers/graph';
import { useAuthStore } from '@/store/auth-store';
import { useBuiltinTools } from '@/hooks/workflow/use-builtin-tools';
import { useLocale } from '@/hooks/use-locale';
import { WorkflowAIChatContextRegistration } from './aichat-context';

// Throttled global mouse tracker to isolate re-renders from WorkflowEditor
// Uses both requestAnimationFrame and time-based throttling (50ms) to minimize store updates
const MouseTracker = () => {
  useEffect(() => {
    let rafId: number | null = null;
    let last: { x: number; y: number } | null = null;
    let lastUpdateTime = 0;
    const THROTTLE_MS = 50; // Throttle to 20fps max to reduce store update frequency

    const onMove = (e: MouseEvent) => {
      last = { x: e.clientX, y: e.clientY };
      const now = performance.now();

      // Skip if already scheduled or within throttle window
      if (rafId !== null || now - lastUpdateTime < THROTTLE_MS) return;

      rafId = window.requestAnimationFrame(() => {
        try {
          const setter = (
            useWorkflowStore.getState() as unknown as {
              setLastMouseClient?: (pos: { x: number; y: number } | null) => void;
            }
          ).setLastMouseClient;
          if (typeof setter === 'function' && last) setter(last);
          lastUpdateTime = performance.now();
        } catch {
          // no-op
        }
        rafId = null;
      });
    };

    window.addEventListener('mousemove', onMove, { passive: true });
    return () => {
      window.removeEventListener('mousemove', onMove);
      if (rafId !== null) {
        window.cancelAnimationFrame(rafId);
        rafId = null;
      }
    };
  }, []);

  return null;
};

interface WorkflowEditorProps {
  agentDetail: AgentDetail;
  isDetailLoading: boolean;
  focusNodeId?: string;
}

const WorkflowEditor: React.FC<WorkflowEditorProps> = ({ agentDetail, focusNodeId }) => {
  const { id: agentId, agent_type: agentType } = agentDetail;
  const workspaceId = agentDetail.workspace?.id || '';
  // Use workflow-specific translation namespace for all workflow UI strings
  const t = useT('agents');
  // Track whether a connection drag is in progress to temporarily disable panning in hand mode

  // Edge types memoized within CanvasWithDnd
  // Workflow data hooks
  const { data: workflowData, isLoading, error, refetch } = useWorkflowDraft(agentId);
  const { handleCombinedSave, handlePublish, isSaving, isPublishing, isDirty } =
    useCombinedWorkflowSave(agentId);
  const { isValid } = useWorkflowValidation();
  const systemFeatures = useAuthStore.use.systemFeatures();
  const syncRunnableSets = useWorkflowStore.use.syncRunnableSets();
  const setToolValidationContext = useWorkflowStore.use.setToolValidationContext();
  const { tools } = useBuiltinTools();
  const { locale } = useLocale();

  useEffect(() => {
    setToolValidationContext(Array.isArray(tools) ? tools : null, locale);
  }, [locale, setToolValidationContext, tools]);

  useEffect(() => {
    syncRunnableSets();
  }, [syncRunnableSets, systemFeatures]);

  // Initialize keyboard shortcuts with onSave routed to combined save
  const onSave = useCallback(() => {
    handleCombinedSave();
  }, [handleCombinedSave]);
  const onPublish = useCallback(
    async ({ silent = true, saveToast }: { silent?: boolean; saveToast?: string }) => {
      // Block publish when validation errors exist
      if (!isValid) {
        toast.error(t('workflow.fixErrorsBeforePublishing'));
        return;
      }
      // Ensure latest edits are persisted before publishing to avoid stale graph
      await handleCombinedSave({ silent, saveToast });
      await handlePublish({ silent });
    },
    [handleCombinedSave, handlePublish, isValid, t]
  );
  // Disable keyboard shortcuts in history (read-only) mode or permission-based read-only
  const mode = useWorkflowStore.use.mode();
  const storeCanEdit = useWorkflowStore.use.canEdit();
  const setCanEdit = useWorkflowStore.use.setCanEdit();

  const isHistoryMode = mode === 'history';
  const isPermissionReadOnly = !storeCanEdit;
  const isReadOnly = isHistoryMode || isPermissionReadOnly;

  // Use shallow subscription for core graph arrays to avoid re-renders on unrelated store changes
  const { nodes, edges, viewport } = useWorkflowStore(
    useShallow(state => ({
      nodes: state.nodes,
      edges: state.edges,
      viewport: state.viewport,
    }))
  );

  const onNodesChange = useWorkflowStore.use.onNodesChange();
  const onEdgesChange = useWorkflowStore.use.onEdgesChange();
  const onConnect = useWorkflowStore.use.onConnect();
  const setViewport = useWorkflowStore.use.setViewport();
  const selectNode = useWorkflowStore.use.selectNode();
  const setSelectionSource = useWorkflowStore.use.setSelectionSource();
  // const selectedNodeId = useWorkflowStore.use.selectedNodeId();
  // const currentIsDirty = useWorkflowStore.use.isDirty();
  // const interactionMode = useWorkflowStore.use.interactionMode();
  const setAgentType = useWorkflowStore.use.setAgentType();
  const enterHistoryMode = useWorkflowStore.use.enterHistoryMode();
  const exitHistoryMode = useWorkflowStore.use.exitHistoryMode();
  const resetRunStatus = useWorkflowStore.use.resetRunStatus();
  const activePanel = useActivePanel(state => state.active);
  const setActivePanel = useActivePanel(state => state.setActive);
  const closeAllPanels = useActivePanel(state => state.closeAll);
  const selectedRunId = useWorkflowStore.use.selectedRunId();
  const historySnapshots = useWorkflowStore.use.historySnapshots();
  const setHistorySnapshot = useWorkflowStore.use.setHistorySnapshot();
  const currentRunningNodeId = useWorkflowStore.use.currentRunningNodeId();
  const setLastDebugInputs = useWorkflowStore.use.setLastDebugInputs();
  const centeredFocusRef = React.useRef<string | null>(null);
  // const setLastMouseClient = useWorkflowStore.use.setLastMouseClient();

  // Default LLM model for bootstrapping the template graph
  // Bootstrapping is handled by useWorkflowBootstrap

  // Compute effective graph for viewing: draft by default; per-run snapshot when in history mode
  const viewNodes = React.useMemo(() => {
    let baseNodes: typeof nodes = nodes;
    if (isHistoryMode && selectedRunId) {
      const snap = historySnapshots[selectedRunId];
      if (snap && Array.isArray(snap.nodes)) {
        baseNodes = snap.nodes as typeof nodes;
      } else {
        // No snapshot yet: render empty until data arrives to avoid showing draft
        return [] as typeof nodes;
      }
    }

    return baseNodes;
  }, [isHistoryMode, selectedRunId, historySnapshots, nodes]);

  const viewEdges = React.useMemo(() => {
    if (isHistoryMode && selectedRunId) {
      const snap = historySnapshots[selectedRunId];
      if (snap && Array.isArray(snap.edges)) return snap.edges as typeof edges;
      return [] as typeof edges;
    }
    return edges;
  }, [isHistoryMode, selectedRunId, historySnapshots, edges]);

  const viewViewport = React.useMemo(() => {
    if (isHistoryMode && selectedRunId) {
      const snap = historySnapshots[selectedRunId];
      if (snap && snap.viewport) return snap.viewport;
      return { x: 0, y: 0, zoom: 1 };
    }
    return viewport;
  }, [isHistoryMode, selectedRunId, historySnapshots, viewport]);

  const { hasPermission } = useAccountPermissions();
  const canManage = hasPermission('agent.manage');

  // Ensure store holds current agent type for downstream filtering
  useEffect(() => {
    setAgentType(agentType);
  }, [agentType, setAgentType]);

  // Sync canEdit permission from permission system to store
  useEffect(() => {
    setCanEdit(canManage);
  }, [canManage, setCanEdit]);

  const resetWorkflowUiState = useCallback(() => {
    try {
      const st = useWorkflowStore.getState() as unknown as {
        mode?: 'edit' | 'history';
        exitHistoryMode?: () => void;
      };
      if (st.mode === 'history') {
        st.exitHistoryMode?.();
      }
    } catch {
      // no-op
    }
    resetRunStatus();
    setLastDebugInputs(null);
    closeAllPanels();
  }, [closeAllPanels, resetRunStatus, setLastDebugInputs]);

  useLayoutEffect(() => {
    resetWorkflowUiState();
    return () => {
      resetWorkflowUiState();
    };
  }, [agentId, resetWorkflowUiState]);

  // Load workflow once on init; thereafter preserve local graph on server updates to avoid flicker
  // Pass isHistoryMode instead of isReadOnly to allow draft loading in permission-based read-only
  useWorkflowLifecycle({ agentId, isHistoryMode, workflowData, agentType, isLoading });

  // Periodic auto-save every 60 seconds: save when either semantic or layout-only changes are present
  useEffect(() => {
    const intervalId = window.setInterval(() => {
      // Skip autosave entirely in read-only (history) mode
      if (isReadOnly) return;
      // Use silent mode to avoid toast spam
      handleCombinedSave({ silent: true });
    }, WORKFLOW_AUTOSAVE_INTERVAL_MS);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [handleCombinedSave, isReadOnly]);

  // Mouse tracking logic moved to MouseTracker component below

  // Handle node selection
  const handleNodeClick = useCallback(
    (event: React.MouseEvent, node: { id: string }) => {
      // Always stop propagation from node clicks
      event.stopPropagation();

      if (
        event.target instanceof Element &&
        event.target.closest('[data-workflow-runtime-log="true"]')
      ) {
        return;
      }

      // Disallow selecting iteration-start anchor nodes
      try {
        const nodesInStore = (useWorkflowStore.getState() as unknown as { nodes: typeof nodes })
          .nodes;
        const clicked = nodesInStore.find(n => n.id === node.id);
        const clickedType = (clicked?.data as { type?: string })?.type;
        if (clickedType === 'iteration-start' || clickedType === 'loop-start') {
          return;
        }
      } catch {
        // no-op
      }

      // In read-only mode, allow selecting for inspection only
      if (isWorkflowDebugPanelActive(activePanel) && !currentRunningNodeId) {
        setActivePanel(null);
      }
      selectNode(node.id);
      setSelectionSource('click');
    },
    [activePanel, currentRunningNodeId, selectNode, setActivePanel, setSelectionSource]
  );

  const handleNodeContextMenu = useCallback((event: React.MouseEvent, node: { id: string }) => {
    // Prevent default browser context menu and stop bubbling to canvas handler
    event.preventDefault();
    event.stopPropagation();

    // Call the globally exposed handler inside WorkflowContextMenu
    const win = window as Window & {
      __workflowNodeContextMenu?: (event: React.MouseEvent, nodeId: string) => void;
    };
    win.__workflowNodeContextMenu?.(event, node.id);
  }, []);

  const handlePaneClick = useCallback(
    (_event: React.MouseEvent) => {
      // Deselect on pane click
      selectNode(null);
      setSelectionSource('none');
    },
    [selectNode, setSelectionSource]
  );

  // CanvasWithDnd moved to './canvas-with-dnd'

  const handleViewportChange = useCallback(
    (vp: Viewport) => {
      // In history mode, update the viewport inside the run snapshot without touching draft state
      if (isReadOnly && selectedRunId) {
        const snap = historySnapshots[selectedRunId];
        if (snap) {
          setHistorySnapshot(selectedRunId, { nodes: snap.nodes, edges: snap.edges, viewport: vp });
        }
        return;
      }
      setViewport(vp);
    },
    [setViewport, isReadOnly, selectedRunId, historySnapshots, setHistorySnapshot]
  );

  // onConnect handling moved into CanvasWithDnd

  const storeAgentId = useWorkflowStore.use.agentId();
  const isInitialized = useWorkflowStore.use.isInitialized();

  useEffect(() => {
    if (!focusNodeId || !isInitialized || storeAgentId !== agentId) {
      return;
    }

    const targetNode = nodes.find(node => node.id === focusNodeId);
    if (!targetNode) {
      return;
    }

    selectNode(focusNodeId);
    setSelectionSource('click');

    const centerKey = `${agentId}:${focusNodeId}`;
    if (centeredFocusRef.current === centerKey) {
      return;
    }

    const container = document.querySelector('.react-flow') as HTMLElement | null;
    const rect = container?.getBoundingClientRect();
    if (!rect) {
      return;
    }

    const abs = getNodeAbsolutePosition(focusNodeId, nodes);
    const nodeWidth = targetNode.measured?.width ?? targetNode.width ?? 280;
    const nodeHeight = targetNode.measured?.height ?? targetNode.height ?? 120;
    const nodeCenterX = abs.x + nodeWidth / 2;
    const nodeCenterY = abs.y + nodeHeight / 2;
    const nextViewport = {
      x: rect.width / 2 - nodeCenterX * viewport.zoom,
      y: rect.height / 2 - nodeCenterY * viewport.zoom,
      zoom: viewport.zoom,
    };

    setViewport(nextViewport, { markLayoutDirty: false });
    centeredFocusRef.current = centerKey;
  }, [
    agentId,
    focusNodeId,
    isInitialized,
    nodes,
    selectNode,
    setSelectionSource,
    setViewport,
    storeAgentId,
    viewport.zoom,
  ]);

  const leaveGuardDialog = useWorkflowLeaveGuard({
    enabled: !isLoading && isInitialized && storeAgentId === agentId && !error,
    shouldGuard: !isReadOnly,
    isValid,
    isSaving,
    onSave: handleCombinedSave,
  });

  if (isLoading || !isInitialized || storeAgentId !== agentId) {
    return <WorkflowSkeleton />;
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="mx-auto w-16 h-16 rounded-full bg-red-100 flex items-center justify-center mb-4">
            <svg
              className="w-8 h-8 text-red-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z"
              />
            </svg>
          </div>
          <p className="text-gray-600 mb-4">{t('workflow.failed')}</p>
          <button
            onClick={() => refetch()}
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
          >
            {t('workflow.retry')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <ReactFlowProvider>
      <WorkflowEditorProvider value={{ agentId, agentType, workspaceId }}>
        <WorkflowAIChatContextRegistration
          agentDetail={agentDetail}
          workflowDraft={workflowData}
          viewNodes={viewNodes}
          viewEdges={viewEdges}
          isDirty={isDirty}
          isSaving={isSaving}
          isPublishing={isPublishing}
          isReadOnly={isReadOnly}
          isHistoryMode={isHistoryMode}
          isPermissionReadOnly={isPermissionReadOnly}
          canPublish={isValid}
          selectedRunId={selectedRunId}
        />
        {/* Bind keyboard shortcuts within provider to ensure context availability */}
        <WorkflowKeyboardBindings onSave={onSave} disabled={isReadOnly} />
        {leaveGuardDialog}
        <div className="h-full w-full flex flex-col bg-gray-50 relative">
          <WorkflowHeader
            agentId={agentId}
            agentName={agentDetail.name}
            agentIconType={agentDetail.icon_type}
            agentIcon={agentDetail.icon}
            agentIconUrl={agentDetail.icon_url}
            agentType={agentType}
            webAppStatus={agentDetail.web_app_status}
            isDirty={isDirty}
            isSaving={isSaving}
            isPublishing={isPublishing}
            canPublish={isValid}
            onSave={() => {
              handleCombinedSave({
                silent: false,
                saveToast: t('workflow.workflowSavedSuccessfully'),
              });
            }}
            onPublish={onPublish}
            isReadOnly={isReadOnly}
            isHistoryMode={isHistoryMode}
            isPermissionReadOnly={isPermissionReadOnly}
            selectedRunId={selectedRunId}
            onSelectRunHistory={(runId: string) => {
              enterHistoryMode(runId);
              try {
                const setActive = useActivePanel.getState().setActive;
                setActive(
                  agentType === AgentType.CONVERSATIONAL_AGENT ? 'conversation-history' : 'run'
                );
              } catch {
                // no-op
              }
            }}
            onExitHistory={() => {
              exitHistoryMode();
              resetRunStatus();
              try {
                const setActive = useActivePanel.getState().setActive;
                setActive(null);
              } catch {
                // no-op
              }
            }}
          />

          {/* Main Content - ReactFlow directly in the main container */}
          <div className="flex-1 relative overflow-hidden">
            <div className="flex h-full">
              {/* Editor container */}
              <div className="flex-1 min-w-0">
                <CanvasWithDnd
                  viewNodes={viewNodes}
                  viewEdges={viewEdges}
                  viewViewport={viewViewport}
                  isReadOnly={isReadOnly}
                  agentType={agentType}
                  agentId={agentId}
                  agentName={agentDetail.name}
                  agentIconType={agentDetail.icon_type}
                  agentIcon={agentDetail.icon}
                  agentIconUrl={agentDetail.icon_url}
                  onNodesChange={onNodesChange}
                  onEdgesChange={onEdgesChange}
                  onConnect={onConnect}
                  onNodeClick={handleNodeClick}
                  onNodeContextMenu={handleNodeContextMenu}
                  onPaneClick={handlePaneClick}
                  onViewportChange={handleViewportChange}
                />

                {/* NodeFloatingPanel is now rendered within ReactFlow as a Panel; outer usage removed */}
              </div>
            </div>
          </div>
          <MouseTracker />
        </div>
      </WorkflowEditorProvider>
    </ReactFlowProvider>
  );
};

export default WorkflowEditor;
