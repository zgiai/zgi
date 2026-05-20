'use client';

import React from 'react';
import WorkflowFeaturesModal from './index';
import { useActivePanel } from '../../hooks/use-active-panel';

const WorkflowFeaturesModalHost: React.FC = () => {
  const active = useActivePanel(state => state.active);
  const closeAll = useActivePanel(state => state.closeAll);
  const open = active === 'features';
  return <WorkflowFeaturesModal open={open} onClose={closeAll} />;
};

export default WorkflowFeaturesModalHost;
