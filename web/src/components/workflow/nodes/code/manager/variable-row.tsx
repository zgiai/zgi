'use client';

import { sanitizeIdentifier } from '@/utils/validation';
import React, { useEffect } from 'react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import NodeValueSelector from '../../../common/node-value-selector';
import { Trash } from 'lucide-react';

export interface VariableRowProps {
  idx: number;
  variableName: string;
  onNameChange: (index: number, name: string) => void;
  nodeId: string;
  selectorValue?: string[];
  onSelectorChange: (
    index: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: string }
  ) => void;
  onRemove: (index: number) => void;
  isReadOnly: boolean;
  tVarPlaceholder: string;
}

const VariableRow: React.FC<VariableRowProps> = ({
  idx,
  variableName,
  onNameChange,
  nodeId,
  selectorValue,
  onSelectorChange,
  onRemove,
  isReadOnly,
  tVarPlaceholder,
}) => {
  const [localName, setLocalName] = React.useState<string>(variableName);
  const commitTimerRef = React.useRef<number | null>(null);

  const syncingRef = React.useRef(false);
  useEffect(() => {
    if (localName !== variableName) {
      // Parent sync: prevent this change from re-emitting upward
      syncingRef.current = true;
      if (commitTimerRef.current) {
        window.clearTimeout(commitTimerRef.current);
        commitTimerRef.current = null;
      }
      setLocalName(variableName);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [variableName]);

  const scheduleCommit = React.useCallback(
    (name: string) => {
      if (commitTimerRef.current) {
        window.clearTimeout(commitTimerRef.current);
        commitTimerRef.current = null;
      }
      commitTimerRef.current = window.setTimeout(() => {
        commitTimerRef.current = null;
        if (name !== variableName) onNameChange(idx, name);
      }, 200);
    },
    [idx, onNameChange, variableName]
  );

  useEffect(() => {
    if (syncingRef.current) {
      syncingRef.current = false;
      return; // do not emit when change comes from parent sync
    }
    scheduleCommit(localName);
    return () => {
      if (commitTimerRef.current) {
        window.clearTimeout(commitTimerRef.current);
        commitTimerRef.current = null;
      }
    };
  }, [localName, scheduleCommit]);

  return (
    <div className="flex items-center gap-2 w-full">
      <Input
        value={localName}
        onChange={e => setLocalName(sanitizeIdentifier(e.target.value))}
        placeholder={tVarPlaceholder}
        className="grow w-0"
        root
        disabled={isReadOnly}
      />
      <div className="min-w-36">
        <NodeValueSelector
          nodeId={nodeId}
          value={selectorValue}
          onChange={payload => {
            onSelectorChange(idx, payload);
          }}
        />
      </div>
      <Button
        variant="ghost"
        isIcon
        onClick={() => onRemove(idx)}
        disabled={isReadOnly}
        aria-label="Remove"
      >
        <Trash className="w-4 h-4" />
      </Button>
    </div>
  );
};

export default React.memo(VariableRow);
