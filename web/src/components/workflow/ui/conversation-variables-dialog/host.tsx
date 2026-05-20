'use client';

import React from 'react';
import ConversationVariablesDialog from './index';
import { useActivePanel } from '../../hooks/use-active-panel';

const ConversationVariablesDialogHost: React.FC = () => {
  const active = useActivePanel(state => state.active);
  const closeAll = useActivePanel(state => state.closeAll);
  const open = active === 'conversation-variables';
  return <ConversationVariablesDialog open={open} onClose={closeAll} />;
};

export default ConversationVariablesDialogHost;
