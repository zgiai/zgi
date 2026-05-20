'use client';

import React, { useState, useEffect, useRef } from 'react';
import { useWorkflowStore } from '../store';
import { useT } from '@/i18n';
import { Check, X } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';

export function EdgeDescriptionEditor() {
  const edgeDescId = useWorkflowStore.use.edgeDescId();
  const edgeDescPosition = useWorkflowStore.use.edgeDescPosition();
  const setEdgeDescId = useWorkflowStore.use.setEdgeDescId();
  const edges = useWorkflowStore.use.edges();
  const updateEdgeData = useWorkflowStore.use.updateEdgeData();
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const { common, agents } = useT();

  const [value, setValue] = useState('');
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const activeEdge = edges.find(e => e.id === edgeDescId);
  const isReadOnly = mode === 'history' || !canEdit;

  useEffect(() => {
    if (isReadOnly && edgeDescId) {
      setEdgeDescId(null);
    }
  }, [edgeDescId, isReadOnly, setEdgeDescId]);

  useEffect(() => {
    if (activeEdge) {
      setValue((activeEdge.data?.desc as string) || '');
      // Focus after a tiny delay to ensure render
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [activeEdge, edgeDescId]);

  if (isReadOnly || !edgeDescId || !edgeDescPosition || !activeEdge) return null;

  const handleSave = () => {
    updateEdgeData(edgeDescId, { desc: value });
    setEdgeDescId(null);
  };

  const handleCancel = () => {
    setEdgeDescId(null);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      handleSave();
    } else if (e.key === 'Escape') {
      handleCancel();
    }
  };

  const handleBlur = (e: React.FocusEvent) => {
    // If we click inside the editor (buttons), don't close yet
    if (e.relatedTarget && (e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) {
      return;
    }
    handleSave();
  };

  return (
    <div
      className="fixed z-[100] w-64 rounded-lg border bg-popover p-2 shadow-xl ring-1 ring-black/5 dark:ring-white/10"
      onMouseDown={e => e.stopPropagation()} // Prevent canvas from taking focus
      style={{
        left: edgeDescPosition.x,
        top: edgeDescPosition.y,
        transform: 'translate(-50%, -100%) translateY(-10px)',
      }}
    >
      <div className="mb-2 flex items-center justify-between text-xs font-medium text-muted-foreground uppercase tracking-wider">
        <span>{agents('workflow.edgeDescription')}</span>
      </div>
      <Textarea
        ref={inputRef}
        value={value}
        onChange={e => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        placeholder={agents('workflow.edgeDescriptionPlaceholder')}
        className="resize-none"
        rows={3}
      />
      <div className="mt-2 flex justify-end gap-2">
        <button
          onClick={handleCancel}
          className="flex h-7 items-center gap-1.5 rounded-md px-3 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          <X size={14} />
          {common('cancel')}
        </button>
        <button
          onClick={handleSave}
          className="flex h-7 items-center gap-1.5 rounded-md bg-blue-600 px-3 text-xs font-medium text-white transition-colors hover:bg-blue-700"
        >
          <Check size={14} />
          {common('save')}
        </button>
      </div>
    </div>
  );
}
