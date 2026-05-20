import type { LoopEndNodeData } from './config';

export interface LoopEndContentProps {
  nodeId: string;
  data: LoopEndNodeData;
}

const LoopEndContent: React.FC<LoopEndContentProps> = ({ nodeId: _nodeId, data: _data }) => {
  return null;
};

export default LoopEndContent;
