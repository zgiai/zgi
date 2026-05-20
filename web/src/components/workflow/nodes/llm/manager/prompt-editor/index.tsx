'use client';

import React, { forwardRef, useImperativeHandle, useRef, useState } from 'react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import type { LLMNodeData } from '@/components/workflow/store/type';
import { ChevronDown, Maximize2 } from 'lucide-react';
import { WorkflowValueEditor, type WorkflowValueEditorHandle } from '@/components/workflow/ui';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { useTranslations } from 'next-intl';

// Public handle for imperative operations from parent
export interface PromptEditorHandle {
  insertToken: (sourceId: string, name: string) => void;
  focus: () => void;
  openVariableSelector: () => void;
}

interface PromptEditorProps {
  className?: string;
  role: LLMNodeData['prompt_template'][number]['role'];
  value: string;
  onChangeRole: (role: LLMNodeData['prompt_template'][number]['role']) => void;
  onChange: (value: string) => void;
  readOnly?: boolean;
  actions?: React.ReactNode; // right aligned actions (e.g. Insert variable dropdown)
  placeholder?: string;
  roleLocked?: boolean; // when true, role cannot be changed (first block fixed to system)
  nodeId?: string; // current node id to load upstream variables
  // Allowed roles for selection dropdown; defaults to all roles
  allowedRoles?: Array<LLMNodeData['prompt_template'][number]['role']>;
  // Bubble focus changes to parent with the active editor handle
  onFocused?: (handle: WorkflowValueEditorHandle) => void;
}

type Role = LLMNodeData['prompt_template'][number]['role'];
const ROLE_OPTIONS: Role[] = ['system', 'user', 'assistant'];

const PromptEditor = forwardRef<PromptEditorHandle, PromptEditorProps>(
  (
    {
      className,
      role,
      value,
      onChangeRole,
      onChange,
      readOnly = false,
      actions,
      placeholder,
      roleLocked = false,
      nodeId,
      allowedRoles = ROLE_OPTIONS,
      onFocused,
    },
    ref
  ) => {
    const t = useT('nodes');
    const commonT = useTranslations('common');
    const roleLabels: Record<Role, string> = {
      system: t('llm.roles.system'),
      user: t('llm.roles.user'),
      assistant: t('llm.roles.assistant'),
    };
    // Bridge to inner WorkflowValueEditor imperative API
    const innerRef = useRef<WorkflowValueEditorHandle | null>(null);
    // Expanded modal state and editor ref
    const [expanded, setExpanded] = useState(false);
    const modalEditorRef = useRef<WorkflowValueEditorHandle | null>(null);

    useImperativeHandle(ref, () => ({
      insertToken: (sourceId: string, name: string) => {
        if (readOnly) return;
        innerRef.current?.insertToken(sourceId, name);
      },
      focus: () => {
        innerRef.current?.focus();
      },
      openVariableSelector: () => {
        if (readOnly) return;
        innerRef.current?.openVariableSelector();
      },
    }), [readOnly]);

    return (
      <div className={cn('space-y-0.5', className)}>
        {/* Role selector and actions */}
        <div className="flex items-center justify-between border-b bg-muted rounded-t-md px-1 py-0.5">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="px-2 text-sm font-bold"
                aria-label="Role"
                disabled={roleLocked || readOnly}
              >
                {roleLabels[role]}
                {!roleLocked && !readOnly && <ChevronDown size={16} />}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-28 px-1 text-xs">
              {allowedRoles.map(r => (
                <DropdownMenuItem
                  key={r}
                  className="text-xs py-0s.5"
                  onClick={() => {
                    if (roleLocked || readOnly) return;
                    onChangeRole(r);
                  }}
                >
                  {roleLabels[r]}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          <div className="flex items-center gap-1.5">
            {actions}
            {/* Expand editor button at far right */}

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="xs"
                  className="hover:bg-background"
                  isIcon
                  aria-label={t('llm.actions.expandEditor')}
                  onClick={() => setExpanded(true)}
                >
                  <Maximize2 className="w-4 h-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('llm.actions.expandEditor')}</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* Value editor */}
        <WorkflowValueEditor
          ref={innerRef}
          value={value}
          editorClassName="min-h-[72px] max-h-[174px] overflow-y-auto border-none p-1.5"
          onChange={onChange}
          readOnly={readOnly}
          placeholder={placeholder}
          nodeId={nodeId}
          suggestEnabled={!expanded && !readOnly}
          onFocus={() => {
            const h = innerRef.current;
            if (h) onFocused?.(h);
          }}
        />

        {/* Expanded floating editor modal */}
        <Dialog
          open={expanded}
          onOpenChange={next => {
            setExpanded(next);
            if (!next) {
              // Refocus inline editor when modal closes so focus state stays consistent
              setTimeout(() => innerRef.current?.focus(), 0);
            }
          }}
        >
          <DialogContent className="max-w-5xl w-full p-0 overflow-hidden">
            <DialogHeader className="pb-2">
              <DialogTitle className="flex items-center gap-4 text-xl font-bold tracking-tight">
                <div className="px-3 py-1 bg-primary/10 text-primary rounded-lg text-sm uppercase tracking-widest font-black">
                  {roleLabels[role]}
                </div>
                {t('llm.actions.expandEditor')}
              </DialogTitle>
            </DialogHeader>

            <DialogBody className="p-0">
              <div className="flex flex-col h-[70vh]">
                <div className="h-0 grow p-6 flex flex-col gap-6 overflow-y-auto scrollbar-thin">
                  {/* Value inserter at top to insert variables at caret */}
                  <div className="bg-neutral-50/50 p-4 rounded-2xl border border-neutral-100 shadow-sm">
                    <WorkflowValueInserter
                      nodeId={nodeId}
                      className="w-full"
                      disabled={readOnly}
                      onInsert={val => {
                        const { sourceId } = val;
                        let key = val.key;
                        // Normalize sys variable key by removing 'sys.' prefix
                        if (sourceId === 'sys' && key.startsWith('sys.')) {
                          key = key.slice(4);
                        }
                        // Insert variable token at current caret in the modal editor
                        modalEditorRef.current?.insertToken(sourceId, key);
                        modalEditorRef.current?.focus();
                      }}
                    />
                  </div>

                  {/* Larger editor area */}
                  <div className="grow h-0 overflow-hidden flex flex-col">
                    <WorkflowValueEditor
                      ref={modalEditorRef}
                      value={value}
                      className="h-full"
                      editorClassName="h-full p-6 overflow-y-auto font-medium leading-relaxed selection:bg-primary/20 scrollbar-thin"
                      onChange={onChange}
                      readOnly={readOnly}
                      placeholder={placeholder}
                      nodeId={nodeId}
                      suggestEnabled={!readOnly}
                      onFocus={() => {
                        const h = modalEditorRef.current;
                        if (h) onFocused?.(h);
                      }}
                    />
                  </div>
                </div>
              </div>
            </DialogBody>

            <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
              <Button
                onClick={() => setExpanded(false)}
                size="lg"
                className="px-10 font-bold shadow-sm"
              >
                {commonT('confirm')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    );
  }
);
PromptEditor.displayName = 'PromptEditor';

export default PromptEditor;
