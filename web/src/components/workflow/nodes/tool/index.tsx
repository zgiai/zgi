import type { WorkflowNodeData } from '../../store';

// Minimal content component for Tool node
// Shows provider and tool identity, keep lightweight for performance

type ToolData = Extract<WorkflowNodeData, { type: 'tools' }>;

interface ToolContentProps {
  nodeId: string;
  data: ToolData;
}

const ToolContent: React.FC<ToolContentProps> = ({ nodeId: _nodeId, data: _data }) => {
  return null;
};

export default ToolContent;
