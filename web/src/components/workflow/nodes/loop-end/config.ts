export interface LoopEndNodeData {
  type: 'loop-end';
  title: string;
  desc: string;
  isInLoop: boolean;
  isInIteration: boolean;
}
/** Default loop-end node data */
export const DEFAULT_LOOP_END_NODE_DATA: LoopEndNodeData = {
  type: 'loop-end',
  title: 'Loop End',
  desc: '',
  isInLoop: false,
  isInIteration: false,
};
