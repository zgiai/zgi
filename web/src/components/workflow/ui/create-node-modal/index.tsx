'use client';

import React from 'react';
import CreateNodeModal from './create-node-modal';
import { useCreateNodeModal } from '../../hooks/use-create-node-modal';
import { useShallow } from 'zustand/react/shallow';

const CreateNodeModalHost: React.FC = () => {
  const { open, position, anchorClientPosition, closeModal, originatingHandle } =
    useCreateNodeModal(
      useShallow(state => ({
        open: state.open,
        position: state.position,
        anchorClientPosition: state.anchorClientPosition,
        closeModal: state.closeModal,
        originatingHandle: state.originatingHandle,
      }))
    );

  return (
    <CreateNodeModal
      isOpen={open}
      onClose={closeModal}
      position={position}
      anchorClientPosition={anchorClientPosition}
      originatingHandle={originatingHandle}
    />
  );
};

export default CreateNodeModalHost;
