import React from 'react';
import type { LoopNodeData } from './config';

interface LoopContentProps {
  nodeId: string;
  data: LoopNodeData;
}

const LoopContent: React.FC<LoopContentProps> = ({ nodeId: _nodeId, data: _data }) => {
  return null;
};

export default LoopContent;
