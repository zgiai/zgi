import React from 'react';
import { type EdgeProps, getBezierPath, EdgeLabelRenderer, BaseEdge } from '@xyflow/react';
import { cn } from '@/lib/utils';
import { useWorkflowStore } from '../store/store';
import { Plus } from 'lucide-react';
import { RUN_STATUS_COLORS } from './constants';
import { useCreateNodeModal } from '../hooks/use-create-node-modal';
import { useT } from '@/i18n';

// Extend EdgeProps to ensure handle ids are available in props
export type CustomEdgeProps = EdgeProps & {
  sourceHandle?: string | null;
  targetHandle?: string | null;
  sourceHandleId?: string | null;
  targetHandleId?: string | null;
};

/**
 * Custom edge component for workflow connections
 * Features:
 * - Smooth bezier curves
 * - Animated flow effect
 * - Custom styling
 * - Optional labels
 * - Hover effects
 * - Custom context menu (right click) for edge actions
 */
const CustomEdge: React.FC<CustomEdgeProps> = ({
  id,
  source,
  target,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  selected,
  sourceHandle,
  targetHandle,
  sourceHandleId,
  targetHandleId,
  data,
}: CustomEdgeProps) => {
  const { agents } = useT();
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const isReadOnly = mode === 'history' || !canEdit;
  // Generate bezier path
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  // Protection: if both source and target are at (0,0) relative to their nodes,
  // it likely means handles haven't been measured yet.
  // However, sourceX/Y and targetX/Y are absolute coordinates.
  // A better check is if the path is too short or if any coordinate is exactly 0
  // while other nodes are elsewhere. But sourceX/Y are often non-zero even if handles are at 0,0 relative to node.
  // In React Flow, if handles are not found, it defaults to node center.
  // The zig-zag happens when one handle is found and the other is (0,0) or node center.

  // We check if both ends are exactly the same (meaning handles both defaulted to same point)
  // or if coordinates are clearly invalid.
  const isInvalid = sourceX === 0 && sourceY === 0 && targetX === 0 && targetY === 0;

  // Use a granular selector to avoid re-rendering ALL edges when hoveredNodeId changes
  // Only this edge will re-render if its source or target is the hovered node
  const isHoveredByNode = useWorkflowStore(
    React.useCallback(
      state =>
        !!state.hoveredNodeId && (state.hoveredNodeId === source || state.hoveredNodeId === target),
      [source, target]
    )
  );
  const activeOutputHandleMap = useWorkflowStore.use.activeOutputHandleByNodeId?.() as
    | Record<string, string | null>
    | undefined;

  const gradientId = `edge-gradient-${id}`;
  const activeOutputHandle = activeOutputHandleMap ? activeOutputHandleMap[source as string] : null;
  const resolvedSourceHandle = sourceHandle ?? sourceHandleId ?? null;
  const resolvedTargetHandle = targetHandle ?? targetHandleId ?? null;
  const normalizedSourceHandle = resolvedSourceHandle ?? 'source';
  const matchesActiveRoute =
    typeof activeOutputHandle === 'string' &&
    activeOutputHandle.length > 0 &&
    normalizedSourceHandle === activeOutputHandle;
  const routeColor = RUN_STATUS_COLORS.succeeded;
  const edgeColor = matchesActiveRoute ? routeColor : 'var(--edge-default)';
  const isColored = matchesActiveRoute;

  // Default edge styles; stroke set via gradient URL below
  const defaultStyle = {
    stroke: `url(#${gradientId})`,
    strokeWidth: 2,
    ...style,
  } as React.CSSProperties;

  // Enhanced styles for selected state
  // Note: isHoveredByNode is now computed in the granular selector above
  const edgeStyle = selected
    ? {
        ...defaultStyle,
        stroke: 'var(--edge-selected)',
        strokeWidth: 3,
        filter: 'drop-shadow(0 0 6px rgba(99, 102, 241, 0.4))',
        strokeDasharray: 'none',
      }
    : isHoveredByNode
      ? {
          ...defaultStyle,
          stroke: 'var(--edge-hover)',
          // Keep stroke width unchanged on node hover
          strokeWidth: defaultStyle.strokeWidth,
          filter: 'drop-shadow(0 0 6px rgba(99, 102, 241, 0.35))',
          strokeDasharray: 'none',
        }
      : {
          ...defaultStyle,
          // If edge is colored by run-status, force solid line; otherwise dashed as default
          strokeDasharray: isColored ? 'none' : '5,5',
        };

  // Custom context menu state for edge
  const [menuOpen, setMenuOpen] = React.useState<boolean>(false);
  const onEdgesChange = useWorkflowStore.use.onEdgesChange();
  const { openEdgeInsertModal } = useCreateNodeModal();

  const description = data?.desc as string | undefined;

  const setEdgeDescId = useWorkflowStore.use.setEdgeDescId();
  const setEdgeDescPosition = useWorkflowStore.use.setEdgeDescPosition();

  const handleLabelClick = React.useCallback(
    (e: React.MouseEvent) => {
      if (isReadOnly) return;
      e.stopPropagation();
      setEdgeDescId(id);
      setEdgeDescPosition({ x: e.clientX, y: e.clientY });
    },
    [id, isReadOnly, setEdgeDescId, setEdgeDescPosition]
  );

  // Delete edge handler using onEdgesChange to keep state consistent
  const handleDeleteEdge = React.useCallback(() => {
    if (!id || isReadOnly) return;
    onEdgesChange([{ id, type: 'remove' }]);
    setMenuOpen(false);
  }, [id, isReadOnly, onEdgesChange]);

  // Open context menu on right click
  const handleEdgeContextMenu = React.useCallback(
    (event: React.MouseEvent<SVGPathElement>) => {
      if (isReadOnly) return;
      event.preventDefault();
      event.stopPropagation();
      setMenuOpen(true);
    },
    [isReadOnly]
  );

  // Close menu on any outside click
  React.useEffect(() => {
    if (!menuOpen) return;
    const onDocClick = () => setMenuOpen(false);
    document.addEventListener('click', onDocClick);
    return () => document.removeEventListener('click', onDocClick);
  }, [menuOpen]);

  const handleOpenInsert = React.useCallback(
    (e: React.MouseEvent) => {
      if (isReadOnly) return;
      e.stopPropagation();
      if (!id || !source || !target) return;
      // Use handle midpoint X and target handle Y to better align the inserted node center
      const midX = (sourceX + targetX) / 2;
      const midY = targetY; // align to target input handle center vertically
      // Try to recover handle ids from edge id when props are missing
      let recoveredSrcH: string | undefined = undefined;
      let recoveredTgtH: string | undefined = undefined;
      if ((!resolvedSourceHandle || !resolvedTargetHandle) && typeof id === 'string') {
        const prefix = `${source}-`;
        const middle = `-${target}-`;
        const p = id.indexOf(prefix);
        const q = id.indexOf(middle, p + prefix.length);
        if (p >= 0 && q > p) {
          recoveredSrcH = id.substring(p + prefix.length, q);
          recoveredTgtH = id.substring(q + middle.length);
        }
      }
      openEdgeInsertModal({
        position: { x: midX, y: midY },
        anchorClientPosition: { x: e.clientX, y: e.clientY },
        edge: {
          edgeId: id,
          sourceId: source,
          targetId: target,
          sourceHandle: (resolvedSourceHandle ?? recoveredSrcH) || undefined,
          targetHandle: (resolvedTargetHandle ?? recoveredTgtH) || undefined,
          midPoint: { x: midX, y: midY },
        },
      });
    },
    [
      id,
      isReadOnly,
      source,
      target,
      resolvedSourceHandle,
      resolvedTargetHandle,
      sourceX,
      targetX,
      targetY,
      openEdgeInsertModal,
    ]
  );

  // Hover state to show insert button on edge hover instead of requiring selection
  const [isHovered, setIsHovered] = React.useState(false);

  if (isInvalid) return null;

  return (
    <>
      {/* Gradient is route-driven by node_finished.output_handle. */}
      <defs>
        <linearGradient
          id={gradientId}
          gradientUnits="userSpaceOnUse"
          x1={sourceX}
          y1={sourceY}
          x2={targetX}
          y2={targetY}
        >
          <stop offset="0%" stopColor={edgeColor} />
          <stop offset="100%" stopColor={edgeColor} />
        </linearGradient>
      </defs>
      {/* Main edge path */}
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={edgeStyle}
        className={cn(
          'cursor-pointer',
          !!selected && 'animate-pulse',
          'hover:[stroke:var(--edge-hover)] hover:stroke-[3px]'
        )}
      />
      {/* Transparent thick stroke to capture context menu events */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        onContextMenu={handleEdgeContextMenu}
        onMouseEnter={() => setIsHovered(true)}
        onMouseLeave={() => setIsHovered(false)}
        style={{ pointerEvents: 'all', cursor: 'pointer' }}
      />

      {/* Edge context menu rendered at edge midpoint */}
      <EdgeLabelRenderer>
        {!isReadOnly && menuOpen && (
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: 'all',
              zIndex: 100,
            }}
            className="nodrag nopan"
          >
            <div className="min-w-[8rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md">
              {description && (
                <div className="px-2 py-1.5 text-xs text-muted-foreground border-b mb-1 max-w-[200px] break-words">
                  {description}
                </div>
              )}
              <button
                type="button"
                onClick={handleDeleteEdge}
                className={cn(
                  'relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors',
                  'hover:bg-accent hover:text-accent-foreground text-red-600'
                )}
              >
                {agents('workflow.deleteEdge')}
              </button>
            </div>
          </div>
        )}
      </EdgeLabelRenderer>

      {/* Persistent description label */}
      {description && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, 0) translate(${labelX}px,${labelY + 6}px)`,
              pointerEvents: 'auto',
              zIndex: 30,
              cursor: 'pointer',
            }}
            className="nodrag nopan select-none"
            onDoubleClick={handleLabelClick}
          >
            <div className="px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground/80 transition-opacity hover:text-muted-foreground max-w-[200px] break-words line-clamp-2">
              {description}
            </div>
          </div>
        </EdgeLabelRenderer>
      )}

      {/* Insert button displayed when edge is hovered */}
      {!isReadOnly && isHovered && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: 'all',
              zIndex: 40,
            }}
            className="nodrag nopan"
            onMouseEnter={() => setIsHovered(true)}
            onMouseLeave={() => setIsHovered(false)}
          >
            <button
              type="button"
              onClick={handleOpenInsert}
              className={cn(
                'flex h-5 w-5 items-center justify-center rounded-full bg-[var(--edge-hover)] text-white shadow-sm transition-colors',
                'hover:bg-[var(--edge-hover)]'
              )}
              title={agents('workflow.insertNode')}
            >
              <Plus size={16} />
            </button>
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
};

// Wrap with React.memo to prevent unnecessary re-renders from parent components
export default React.memo(CustomEdge);
