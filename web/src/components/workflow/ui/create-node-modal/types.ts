import type { HandleType } from '@xyflow/react';

export interface OriginHandleProp {
  nodeId: string;
  handleId: string;
  handleType: HandleType;
}

export type NodeGroupKey = 'flow' | 'ai' | 'data' | 'tool';

export interface NodeTypeInfo {
  type: string;
  title: string;
  description: string;
  icon: React.ReactNode;
  bgColor: string;
  group: NodeGroupKey;
  io?: boolean;
}
