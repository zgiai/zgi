'use client';

import React, { useMemo, useCallback } from 'react';
import { useWorkflowStore } from './store';
import { isContainerNode, type WorkflowNode } from './store/type';
import { Layers, Ban } from 'lucide-react';
import { useWorkflowOperations } from './hooks';
import { createNodeByTypeFactory } from './ui/create-node-modal/services/create-node';
import { NODE_THEMES as THEME_CONFIGS } from './nodes/custom/config';
import {
  ITER_CANVAS_OFFSET_X,
  ITER_CANVAS_OFFSET_Y,
} from './ui/create-node-modal/constants/iteration-layout';
import { useBuiltinTools } from '@/hooks/workflow/use-builtin-tools';
import { useLocale } from '@/hooks/use-locale';
import { pickLocale, mapParametersToFormFields, createInitialBindings } from '@/utils/tool-helpers';
import { useT, type Locale } from '@/lib/i18n';

const CONTAINER_PAD = 24;
const CONTAINER_DROP_BLOCKED_NODE_TYPES = new Set(['approval']);

interface ContainerOverlayItemProps {
  containerNode: WorkflowNode;
  isHovered: boolean;
  isNestingBlocked: boolean;
  draggingNodeType: string;
  viewport: { zoom: number };
  onHover: (id: string | null) => void;
  createNodeByType: (
    type: string,
    position: { x: number; y: number },
    parentId?: string,
    initialData?: any
  ) => string | null;
  tools: any;
  locale: Locale;
  overlayStyle?: React.CSSProperties;
}

const ContainerOverlayItem: React.FC<ContainerOverlayItemProps> = ({
  containerNode,
  isHovered,
  isNestingBlocked,
  draggingNodeType,
  viewport,
  onHover,
  createNodeByType,
  tools,
  locale,
  overlayStyle,
}) => {
  const t = useT();
  if (!overlayStyle) return null;

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = isNestingBlocked ? 'none' : 'copy';
    onHover(containerNode.id);
  };

  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    onHover(containerNode.id);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    // Only clear if leaving the element bounds
    const elRect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (
      e.clientX < elRect.left ||
      e.clientX > elRect.right ||
      e.clientY < elRect.top ||
      e.clientY > elRect.bottom
    ) {
      onHover(null);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();

    if (isNestingBlocked) {
      onHover(null);
      return;
    }

    // Calculate position relative to container parent node origin
    // The canvas area has an offset (PAD_X, PAD_Y + HEADER_H) relative to parent node (0,0)
    const dropRect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const relativeX = (e.clientX - dropRect.left) / viewport.zoom + ITER_CANVAS_OFFSET_X;
    const relativeY = (e.clientY - dropRect.top) / viewport.zoom + ITER_CANVAS_OFFSET_Y;

    const nodeType = draggingNodeType;

    // Get expected node size from theme
    const theme = (THEME_CONFIGS as any)[nodeType] || THEME_CONFIGS.default;
    const nodeWidth = theme.width ?? 280;
    const nodeHeight = theme.height ?? 120;
    const rawToolInfo = e.dataTransfer?.getData('application/x-workflow-tool-info');
    let toolDragInfo: {
      provider_id: string;
      tool_name: string;
      title?: string;
      iconUrl?: string;
    } | null = null;
    if (nodeType === 'tool' && rawToolInfo) {
      try {
        toolDragInfo = JSON.parse(rawToolInfo);
      } catch (_e) {
        toolDragInfo = null;
      }
    }

    // Check expansion
    const parentWidth = containerNode.width ?? 600;
    const parentHeight = containerNode.height ?? 420;
    const requiredWidth = relativeX + nodeWidth + CONTAINER_PAD;
    const requiredHeight = relativeY + nodeHeight + CONTAINER_PAD;
    const needsExpand = requiredWidth > parentWidth || requiredHeight > parentHeight;

    let initialData: any = undefined;
    if (nodeType === 'tool' && toolDragInfo) {
      // Pre-calculate tool metadata to avoid title pop
      try {
        const providers = Array.isArray(tools) ? tools : [];
        const provider = providers.find(
          (p: any) => p.id === toolDragInfo!.provider_id || p.name === toolDragInfo!.provider_id
        );
        const toolItem = provider?.tools?.find((t: any) => t.name === toolDragInfo!.tool_name);
        const fields = toolItem ? mapParametersToFormFields(toolItem.parameters, locale) : [];
        const bindings = createInitialBindings(fields);
        const nodeTitle = toolItem
          ? pickLocale(toolItem.label, locale, toolItem.name)
          : toolDragInfo!.title || toolDragInfo!.tool_name;

        initialData = {
          provider_type: provider?.type || 'builtin',
          provider_id: toolDragInfo!.provider_id,
          tool_name: toolDragInfo!.tool_name,
          tool_parameters: bindings,
          title: nodeTitle,
        };
      } catch (_e) {
        // Swallow errors
      }
    }

    useWorkflowStore.getState().beginHistoryBatch();
    try {
      const newNodeId = createNodeByType(
        nodeType,
        { x: relativeX, y: relativeY },
        undefined,
        initialData
      );
      if (newNodeId) {
        // Set parent
        useWorkflowStore.getState().updateNode(newNodeId, {
          parentId: containerNode.id,
          extent: 'parent',
        });

        // Update children array
        const parentData = (containerNode.data || {}) as { _children?: string[] };
        const prevChildren = Array.isArray(parentData._children) ? parentData._children : [];
        if (!prevChildren.includes(newNodeId)) {
          useWorkflowStore.getState().updateNodeData(containerNode.id, {
            _children: [...prevChildren, newNodeId],
          });
        }

        // Expand if needed
        if (needsExpand) {
          useWorkflowStore.getState().updateNode(containerNode.id, {
            width: Math.max(parentWidth, requiredWidth),
            height: Math.max(parentHeight, requiredHeight),
          });
        }

        useWorkflowStore.getState().selectNode(newNodeId);
        useWorkflowStore.getState().setSelectionSource('create');
      }
    } finally {
      useWorkflowStore.getState().endHistoryBatch();
      onHover(null);
    }
  };

  return (
    <div
      className={`absolute rounded-xl border-2 pointer-events-auto transition-all duration-150 flex items-center justify-center ${
        isNestingBlocked
          ? isHovered
            ? 'bg-red-500/30 border-red-500 cursor-not-allowed'
            : 'bg-red-500/10 border-red-400 border-dashed cursor-not-allowed'
          : isHovered
            ? 'bg-indigo-500/20 border-indigo-500 border-dashed cursor-copy'
            : 'bg-indigo-500/10 border-indigo-400 border-dashed cursor-copy'
      }`}
      style={overlayStyle}
      onDragOver={handleDragOver}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {isHovered && (
        <div
          className={`flex items-center gap-2 px-4 py-2 rounded-lg shadow-xl animate-in zoom-in-95 duration-200 ${
            isNestingBlocked ? 'bg-red-100 text-red-600' : 'bg-white text-indigo-600'
          }`}
        >
          {isNestingBlocked ? <Ban className="w-5 h-5" /> : <Layers className="w-5 h-5" />}
          <span className="font-semibold text-sm">
            {isNestingBlocked ? t('nodes.container.noNesting') : t('nodes.container.overlayText')}
          </span>
        </div>
      )}
    </div>
  );
};

const GlobalContainerOverlay: React.FC = () => {
  const containerRef = React.useRef<HTMLDivElement>(null);

  const draggingNodeType = useWorkflowStore.use.draggingNodeType();
  const dragOverContainerId = useWorkflowStore.use.dragOverContainerId();
  const setDragOverContainerId = useWorkflowStore.use.setDragOverContainerId();

  const nodes = useWorkflowStore.use.nodes();
  const viewport = useWorkflowStore.use.viewport();
  const { tools } = useBuiltinTools();
  const { locale } = useLocale();

  // Find all container nodes
  const containerNodes = useMemo(() => {
    return nodes.filter(n => isContainerNode(n.data?.type as string));
  }, [nodes]);

  const isNestingBlocked = useMemo(() => {
    if (!draggingNodeType) return false;
    return (
      isContainerNode(draggingNodeType) || CONTAINER_DROP_BLOCKED_NODE_TYPES.has(draggingNodeType)
    );
  }, [draggingNodeType]);

  // Node creation logic
  const operations = useWorkflowOperations();
  const createNodeByType = useMemo(() => createNodeByTypeFactory(operations), [operations]);

  const handleHover = useCallback(
    (id: string | null) => {
      setDragOverContainerId(id);
    },
    [setDragOverContainerId]
  );

  // Calculate relative positions for overlay items
  // We use a separate state or just re-calculate on each render when dragging
  // for absolute positioning relative to the overlay root.
  const [overlayItems, setOverlayItems] = React.useState<any[]>([]);

  React.useEffect(() => {
    if (!draggingNodeType || containerNodes.length === 0 || !containerRef.current) {
      if (overlayItems.length > 0) setOverlayItems([]);
      return;
    }

    const updatePositions = () => {
      const rootEl = document.querySelector('.react-flow') as HTMLElement | null;
      if (!rootEl) return;
      const rootRect = rootEl.getBoundingClientRect();

      const items = containerNodes
        .map(containerNode => {
          const canvasEl = document.querySelector(
            `[data-role="container-canvas"][data-node-id="${containerNode.id}"]`
          ) as HTMLElement | null;

          if (!canvasEl) return null;

          const rect = canvasEl.getBoundingClientRect();

          return {
            id: containerNode.id,
            node: containerNode,
            style: {
              width: rect.width,
              height: rect.height,
              left: rect.left - rootRect.left,
              top: rect.top - rootRect.top,
            },
          };
        })
        .filter(Boolean);

      setOverlayItems(items);
    };

    updatePositions();
    const intervalId = setInterval(updatePositions, 50);
    return () => clearInterval(intervalId);
  }, [draggingNodeType, containerNodes]);

  // Don't render if not dragging
  if (!draggingNodeType) return null;

  // Don't render if no container nodes exist
  if (containerNodes.length === 0) return null;

  return (
    <div
      ref={containerRef}
      className="absolute inset-0 z-[5] pointer-events-none"
      onDragOver={e => e.preventDefault()}
    >
      {overlayItems.map(item => (
        <ContainerOverlayItem
          key={item.id}
          containerNode={item.node}
          isHovered={dragOverContainerId === item.id}
          isNestingBlocked={isNestingBlocked}
          draggingNodeType={draggingNodeType}
          viewport={viewport}
          onHover={handleHover}
          createNodeByType={createNodeByType}
          tools={tools}
          locale={locale}
          overlayStyle={item.style}
        />
      ))}
    </div>
  );
};

export default GlobalContainerOverlay;
