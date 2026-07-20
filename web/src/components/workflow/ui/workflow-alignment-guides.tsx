import React from 'react';
import { ViewportPortal } from '@xyflow/react';
import type { AlignmentGuideState } from '../store/helpers/alignment-guides';

interface WorkflowAlignmentGuidesProps {
  guides: AlignmentGuideState | null;
}

const GUIDE_COLOR = '#3b82f6';

export const WorkflowAlignmentGuides: React.FC<WorkflowAlignmentGuidesProps> = ({ guides }) => {
  if (!guides?.vertical && !guides?.horizontal) return null;

  return (
    <ViewportPortal>
      <div className="pointer-events-none absolute left-0 top-0 z-[1000]">
        {guides.vertical && (
          <div
            className="absolute border-l border-dashed"
            style={{
              borderColor: GUIDE_COLOR,
              height: Math.max(1, guides.vertical.to - guides.vertical.from),
              transform: `translate(${guides.vertical.value}px, ${guides.vertical.from}px)`,
            }}
          />
        )}
        {guides.horizontal && (
          <div
            className="absolute border-t border-dashed"
            style={{
              borderColor: GUIDE_COLOR,
              width: Math.max(1, guides.horizontal.to - guides.horizontal.from),
              transform: `translate(${guides.horizontal.from}px, ${guides.horizontal.value}px)`,
            }}
          />
        )}
      </div>
    </ViewportPortal>
  );
};

export default React.memo(WorkflowAlignmentGuides);
