import React from 'react';
import {
  Handle,
  Position,
  type HandleType,
  useReactFlow,
  type Connection,
  useNodeId,
  useUpdateNodeInternals,
  type IsValidConnection,
} from '@xyflow/react';
import { cn } from '@/lib/utils';
import { useCreateNodeModal } from '../hooks/use-create-node-modal';
import { Plus } from 'lucide-react';
import { useWorkflowStore } from '@/components/workflow/store';

interface CustomHandleProps {
  type: HandleType;
  position: Position;
  id: string;
  className?: string;
  isConnectable?: boolean;
  style?: React.CSSProperties;
  onConnect?: (params: Connection) => void;
  /** Visual variant for the handle - 'default' uses primary colors, 'destructive' uses destructive colors */
  variant?: 'default' | 'destructive';
}

/**
 * Custom Handle component with enhanced styling and animations
 * Provides consistent handle appearance across all workflow nodes
 * Automatically handles connections using useReactFlow
 */
const CustomHandle: React.FC<CustomHandleProps> = ({
  type,
  position,
  id,
  className,
  isConnectable = true,
  style,
  onConnect: externalOnConnect,
  variant = 'default',
  ...props
}) => {
  // Filter out non-DOM props that shouldn't be passed to Handle component
  const { nodeType: _nodeType, ...validProps } = props as Record<string, unknown> & {
    nodeType?: string;
  };
  const { openModal } = useCreateNodeModal();
  const nodeId = useNodeId();
  // Use internal handle interactions
  const { handleConnect } = useHandleInteractions();
  // Access React Flow instance for current edges
  const rf = useReactFlow();
  // History mode disables handle interactions and hover visuals
  const mode = useWorkflowStore.use.mode();
  const isHistory = mode === 'history';
  // Trigger edge/anchor re-calculation when handle visual position changes
  const updateNodeInternals = useUpdateNodeInternals();

  // Recompute internals on mount and whenever relevant visual props change
  React.useLayoutEffect(() => {
    if (!nodeId) return;

    // Use a double-RAF strategy for more reliable registration during initial mount
    // when React Flow might still be calculating the internal coordinate system.
    let rafHandle2: number;

    const rafHandle1 = window.requestAnimationFrame(() => {
      updateNodeInternals(nodeId);
      rafHandle2 = window.requestAnimationFrame(() => {
        updateNodeInternals(nodeId);
      });
    });

    return () => {
      window.cancelAnimationFrame(rafHandle1);
      if (rafHandle2) window.cancelAnimationFrame(rafHandle2);
    };
    // We intentionally depend on visual-affecting fields so anchors stay in sync
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    nodeId,
    position,
    type,
    id,
    style?.top,
    style?.left,
    style?.right,
    style?.bottom,
    style?.transform,
  ]);

  // Combine internal and external handlers
  const combinedOnConnect = React.useCallback(
    (params: Connection) => {
      handleConnect(params);
      externalOnConnect?.(params);
    },
    [handleConnect, externalOnConnect]
  );

  // Disallow connecting two handles that belong to the same node (self-connection)
  const isValidConnection = React.useCallback<IsValidConnection>(
    edgeOrConn => {
      // Normalize to full shape for precise duplicate detection including handles
      interface EdgeLike {
        source?: string;
        target?: string;
        sourceHandle?: string | null;
        targetHandle?: string | null;
      }
      const ec = edgeOrConn as EdgeLike;
      if (!ec.source || !ec.target) return true; // allow during drag
      if (ec.source === ec.target) return false; // disallow self-connection

      const srcNode = rf.getNode(ec.source);
      const tgtNode = rf.getNode(ec.target);
      if (srcNode && tgtNode) {
        const srcPid = (srcNode as unknown as { parentId?: string })?.parentId;
        const tgtPid = (tgtNode as unknown as { parentId?: string })?.parentId;
        const crossScope = !!srcPid !== !!tgtPid || (srcPid && tgtPid && srcPid !== tgtPid);
        if (crossScope) return false;
      }

      // If handles are known on both ends, only block when an identical (source,sourceHandle,target,targetHandle) exists
      const sh = ec.sourceHandle ?? undefined;
      const th = ec.targetHandle ?? undefined;
      if (sh && th) {
        const existsExact = rf
          .getEdges()
          .some(
            e =>
              e.source === ec.source &&
              e.target === ec.target &&
              e.sourceHandle === sh &&
              e.targetHandle === th
          );
        return !existsExact;
      }

      // Otherwise be permissive and let onConnect decide
      return true;
    },
    [rf]
  );

  const handleClick = (event: React.MouseEvent) => {
    event.stopPropagation();
    // Only allow add-node modal from source (output) handles
    if (type !== 'source') {
      return;
    }
    // Open add-node modal and remember which handle initiated it
    if (nodeId) {
      openModal(
        undefined,
        { nodeId, handleId: id, handleType: type },
        {
          x: event.clientX,
          y: event.clientY,
        }
      );
    } else {
      openModal(undefined, null, { x: event.clientX, y: event.clientY });
    }
  };

  // Base handle styles with Tailwind classes
  const handleStyles = cn(
    'relative group',
    // Static Industrial Base: 4px x 14px Rectangular Tab
    '!w-[4px] !h-3.5 !rounded-[1px] !border-none !shadow-none !min-w-0 !min-h-0 !p-0',
    // Vivid Blue Palette: High energy for better visibility
    variant === 'destructive' ? '!bg-red-400 dark:!bg-red-300' : '!bg-blue-400 dark:!bg-blue-300',
    // Custom className
    className
  );

  return (
    <Handle
      type={type}
      position={position}
      id={id}
      className={handleStyles}
      isConnectable={!isHistory && isConnectable}
      style={{
        zIndex: 10,
        // Precision alignment: exactly centered on the 40px header line
        // Docked to stick out 4px (the width of the tab)
        transform: position === Position.Right ? 'translate(80%, -50%)' : 'translate(-80%, -50%)',
        ...style,
      }}
      onConnect={!isHistory ? combinedOnConnect : undefined}
      isValidConnection={!isHistory ? isValidConnection : undefined}
      onClick={!isHistory && type === 'source' ? handleClick : undefined}
      // Add test-friendly and accessible attributes
      data-handle-id={id}
      data-handle-type={type}
      data-node-id={nodeId ?? ''}
      aria-label={`Handle ${type} ${id}${nodeId ? ` of node ${nodeId}` : ''}`}
      // Ensure element is keyboard focusable for accessibility
      tabIndex={isHistory ? -1 : 0}
      {...validProps}
    >
      {!isHistory && type === 'source' ? (
        <div
          className={cn(
            'absolute top-1/2 flex items-center justify-center transition-all duration-200',
            // Button Styling: High contrast white/blue combo
            'w-6 h-6 rounded-full bg-white dark:bg-slate-900 border shadow-md',
            // Entrance animation: fade + scale from 0
            'opacity-0 group-hover:opacity-100 group-hover/node:opacity-100 scale-75 group-hover:scale-100 group-hover/node:scale-100',
            // Precise Embedding Shift: -8px offset (+2px right from deep-in)
            position === Position.Right
              ? 'right-[-8px] -translate-y-1/2'
              : 'left-[-8px] -translate-y-1/2',
            variant === 'destructive'
              ? 'border-red-500/50 dark:border-red-400/50'
              : 'border-blue-500/50 dark:border-blue-400/50'
          )}
        >
          <Plus
            size={18}
            className={cn(
              'text-blue-600 dark:text-blue-400',
              variant === 'destructive' && 'text-red-500'
            )}
          />
        </div>
      ) : null}
    </Handle>
  );
};

export default CustomHandle;

/**
 * Predefined handle configurations for common use cases
 */
// Provide explicit types with optional properties to avoid union indexing errors
// when accessing .source or .target via a dynamic key.
export interface HandleOption {
  type: HandleType;
  position: Position;
  id: string;
  // Node type this handle belongs to
  nodeType:
    | 'start'
    | 'end'
    | 'loop-end'
    | 'answer'
    | 'llm'
    | 'knowledge-retrieval'
    | 'http-request'
    | 'call-database'
    | 'sql-generator'
    | 'if-else'
    | 'code'
    | 'iteration'
    | 'assigner'
    | 'tool'
    | 'document-extractor'
    | 'parameter-extractor'
    | 'variable-aggregator'
    | 'json-parser'
    | 'image-gen'
    | 'create-scheduled-task'
    | 'notification-sms'
    | 'announcement'
    | 'question-answer'
    | 'loop';
  // Optional style to allow fixed visual positioning (e.g., top offset)
  style?: React.CSSProperties;
}

export interface HandleConfig {
  source?: HandleOption;
  target?: HandleOption;
}

export const HandleConfigs: Record<
  | 'start'
  | 'end'
  | 'loopEnd'
  | 'answer'
  | 'llm'
  | 'knowledgeRetrieval'
  | 'httpRequest'
  | 'callDatabase'
  | 'sqlGenerator'
  | 'code'
  | 'iteration'
  | 'loop'
  | 'assigner'
  | 'tools'
  | 'documentExtractor'
  | 'parameterExtractor'
  | 'variableAggregator'
  | 'jsonParser'
  | 'imageGen'
  | 'createScheduledTask'
  | 'notificationSms'
  | 'announcement'
  | 'questionAnswer',
  HandleConfig
> = {
  start: {
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'start',
      style: { top: 20, right: 0 },
    },
  },
  end: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'end',
      style: { top: 20, right: 0 },
    },
  },
  loopEnd: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'loop-end',
      style: { top: 20, right: 0 },
    },
  },
  answer: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'answer',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'answer',
      style: { top: 20, right: 0 },
    },
  },
  questionAnswer: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'question-answer',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'question-answer',
      style: { top: 20, right: 0 },
    },
  },
  llm: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'llm',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'llm',
      style: { top: 20, right: 0 },
    },
  },
  knowledgeRetrieval: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'knowledge-retrieval',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'knowledge-retrieval',
      style: { top: 20, right: 0 },
    },
  },
  httpRequest: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'http-request',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'http-request',
      style: { top: 20, right: 0 },
    },
  },
  createScheduledTask: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'create-scheduled-task',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'create-scheduled-task',
      style: { top: 20, right: 0 },
    },
  },
  notificationSms: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'notification-sms',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'notification-sms',
      style: { top: 20, right: 0 },
    },
  },
  announcement: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'announcement',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'announcement',
      style: { top: 20, right: 0 },
    },
  },
  callDatabase: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'call-database',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'call-database',
      style: { top: 20, right: 0 },
    },
  },
  sqlGenerator: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'sql-generator',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'sql-generator',
      style: { top: 20, right: 0 },
    },
  },
  code: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'code',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'code',
      style: { top: 20, right: 0 },
    },
  },
  iteration: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'iteration',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'iteration',
      style: { top: 20, right: 0 },
    },
  },
  loop: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'loop',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'loop',
      style: { top: 20, right: 0 },
    },
  },
  assigner: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'assigner',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'assigner',
      style: { top: 20, right: 0 },
    },
  },
  tools: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'tool',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'tool',
      style: { top: 20, right: 0 },
    },
  },
  documentExtractor: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'document-extractor',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'document-extractor',
      style: { top: 20, right: 0 },
    },
  },
  parameterExtractor: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'parameter-extractor',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'parameter-extractor',
      style: { top: 20, right: 0 },
    },
  },
  variableAggregator: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'variable-aggregator',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'variable-aggregator',
      style: { top: 20 },
    },
  },
  jsonParser: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'json-parser',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'json-parser',
      style: { top: 20, right: 0 },
    },
  },
  imageGen: {
    target: {
      type: 'target',
      position: Position.Left,
      id: 'target',
      nodeType: 'image-gen',
      style: { top: 20, right: 0 },
    },
    source: {
      type: 'source',
      position: Position.Right,
      id: 'source',
      nodeType: 'image-gen',
      style: { top: 20, right: 0 },
    },
  },
};

/**
 * Hook for managing handle interactions and animations
 * Uses useReactFlow to access React Flow instance and handle connections
 */
export const useHandleInteractions = () => {
  const reactFlowInstance = useReactFlow();
  const [isConnecting, setIsConnecting] = React.useState(false);
  const [connectionSource, setConnectionSource] = React.useState<string | null>(null);

  const handleConnect = React.useCallback((_params: Connection) => {
    // Note: Edge creation is handled by ReactFlow's onConnect callback in the store
    // No need to manually add edges here to avoid duplication

    setIsConnecting(false);
    setConnectionSource(null);
  }, []);

  return {
    isConnecting,
    connectionSource,
    handleConnect,
    reactFlowInstance, // Expose the instance for additional operations if needed
  };
};
