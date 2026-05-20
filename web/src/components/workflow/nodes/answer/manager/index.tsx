'use client';

import React from 'react';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import type { WorkflowVariable } from '../../../store/type';
import { useLocalNodeData } from '../../../hooks';
import WorkflowValueEditor, {
  type WorkflowValueEditorHandle,
} from '../../../common/workflow-value-editor';
import WorkflowValueInserter from '../../../common/workflow-value-inserter';
import { useT } from '@/i18n';

interface AnswerManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const AnswerManager: React.FC<AnswerManagerProps> = ({ id, className, readOnly = false }) => {
  const tNodes = useT('nodes');
  const editorRef = React.useRef<WorkflowValueEditorHandle | null>(null);

  // Use store-aware useLocalNodeData for the 'answer' field with debouncing
  const { localData: answer, setLocalData: setAnswer } = useLocalNodeData<string>(id, {
    path: 'answer',
    delay: 300,
  });

  const handleVariableInsert = React.useCallback(
    (value: { sourceId: string; key: string; type: WorkflowVariable['type'] }) => {
      if (readOnly) return;
      try {
        editorRef.current?.insertToken(value.sourceId, value.key);
      } catch (_e) {
        // no-op
      }
    },
    [readOnly]
  );

  return (
    <div className={cn('space-y-3', className)}>
      <div className="mb-2">
        <WorkflowValueInserter
          nodeId={id}
          className="w-full"
          onInsert={handleVariableInsert}
          disabled={readOnly}
        />
      </div>
      <div className="text-base mb-2 flex items-center justify-between">
        <Label>{tNodes('answer.label.title')}</Label>
      </div>
      <WorkflowValueEditor
        ref={editorRef}
        editorClassName="min-h-[120px] max-h-[320px] overflow-y-auto scrollbar-thin"
        value={answer || ''}
        onChange={val => setAnswer(val)}
        nodeId={id}
        placeholder={tNodes('answer.placeholder')}
        readOnly={readOnly}
      />
    </div>
  );
};

export default AnswerManager;
