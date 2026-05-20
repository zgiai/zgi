import React from 'react';
import { MiniMap } from '@xyflow/react';
// import { useReactFlow } from '@xyflow/react';
import type { Node } from '@xyflow/react';
// import { FocusIcon, ZoomIn, ZoomOut } from 'lucide-react';
import { NODE_THEMES } from '../nodes/custom/config';

interface WorkflowMinimapProps {
  className?: string;
}

const WorkflowMinimap: React.FC<WorkflowMinimapProps> = () => {
  // const { zoomIn, zoomOut, fitView } = useReactFlow();

  // Node color mapping based on centralized theme config
  const nodeColor = (node: Node) => {
    const t = (node?.data as { type?: keyof typeof NODE_THEMES } | undefined)?.type;
    if (t && NODE_THEMES[t]) return NODE_THEMES[t].miniMapColor;
    return '#6b7280'; // fallback gray
  };

  return (
    <div className="relative w-full h-full flex flex-col items-center justify-center">
      <div className="w-[190px] h-[150px]">
        <MiniMap
          nodeColor={nodeColor}
          nodeStrokeWidth={2}
          nodeStrokeColor="#ffffff"
          nodeBorderRadius={4}
          maskColor="rgba(0, 0, 0, 0.1)"
          style={{
            borderRadius: 8,
            overflow: 'hidden',
            backgroundColor: '#f8fafc',
            left: 0,
            top: 0,
            width: 160,
            height: 120,
          }}
          pannable
          zoomable
        />
      </div>
      {/* <div className="flex bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="w-8 h-8 flex items-center justify-center" onClick={() => zoomIn()}>
          <ZoomIn size={18} />
        </div>
        <div className="w-8 h-8 flex items-center justify-center" onClick={() => zoomOut()}>
          <ZoomOut size={18} />
        </div>
        <div className="w-8 h-8 flex items-center justify-center" onClick={() => fitView()}>
          <FocusIcon size={18} />
        </div>
      </div> */}
    </div>
  );
};

export default WorkflowMinimap;
