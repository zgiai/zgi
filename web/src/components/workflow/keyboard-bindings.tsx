'use client';

import { useWorkflowKeyboard } from './hooks';

const WorkflowKeyboardBindings: React.FC<{ onSave: () => void; disabled: boolean }> = ({
  onSave,
  disabled,
}) => {
  useWorkflowKeyboard({ onSave, disabled });
  return null;
};

export default WorkflowKeyboardBindings;
