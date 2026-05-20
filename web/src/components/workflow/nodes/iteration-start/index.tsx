import React from 'react';
import { Play } from 'lucide-react';
import { Position } from '@xyflow/react';
import { cn } from '@/lib/utils';
import type { NodePropsCompat } from '../../store';
import CustomHandle from '../../ui/custom-handle';
import { useWorkflowStore } from '../../store/store';
import { computeRunnableSets } from '../../store/helpers/graph';

export interface IterationStartNodeData {
  type: 'iteration-start';
  title?: string;
  desc?: string;
  isInIteration?: boolean;
  isInLoop?: boolean;
}

const IterationStartNode: React.FC<NodePropsCompat<IterationStartNodeData>> = ({ id }) => {
  const nodes = useWorkflowStore.use.nodes();
  const edges = useWorkflowStore.use.edges();
  const { commentSet } = React.useMemo(() => computeRunnableSets(nodes, edges), [nodes, edges]);
  const isComment = commentSet.has(id as string);
  return (
    <div
      className={cn(
        'w-10 h-10 rounded-xl border border-slate-200/50 dark:border-slate-800/50 shadow-sm flex items-center justify-center relative transition-all bg-white/70 dark:bg-slate-900/70 backdrop-blur-md',
        isComment && 'opacity-50'
      )}
      style={{ pointerEvents: 'none' }}
    >
      <div className="w-6 h-6 rounded-lg bg-indigo-500/90 flex items-center justify-center shadow-[0_0_12px_rgba(99,102,241,0.4)]">
        <Play className="w-4 h-4 text-white fill-white" />
      </div>
      <CustomHandle
        type="source"
        position={Position.Right}
        id="source"
        style={{ top: 20, right: 0, pointerEvents: 'auto' }}
      />
    </div>
  );
};

export default IterationStartNode;
