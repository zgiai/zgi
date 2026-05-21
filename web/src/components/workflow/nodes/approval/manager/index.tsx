'use client';

import React from 'react';
import {
  Check,
  ChevronDown,
  CircleAlert,
  Loader2,
  Mail,
  Plus,
  Smartphone,
  Trash2,
} from 'lucide-react';

import { Button, buttonVariants } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import NodeValueSelector from '@/components/workflow/common/node-value-selector';
import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import { WorkflowValueEditor, type WorkflowValueEditorHandle } from '@/components/workflow/ui';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type { VariableInsertValue } from '@/components/workflow/common/workflow-value-inserter/variable-item';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useWorkspaceMembersInfinite } from '@/hooks/workspace/use-workspace-members';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/store/auth-store';
import { isValidEmail } from '@/utils/validation';

import { useLocalNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import { registerWorkflowPendingEditFlush } from '../../../hooks/pending-edits';
import { useWorkflowStore } from '../../../store';
import {
  APPROVAL_ACTION_MAX_LENGTH,
  APPROVAL_IDENTIFIER_PATTERN,
  APPROVAL_TIMEOUT_HANDLE,
  createApprovalActionId,
  getApprovalTimeoutMaxDuration,
  normalizeApprovalNodeData,
  type ApprovalAction,
  type ApprovalActionStyle,
  type ApprovalDefaultValue,
  type ApprovalEmailRecipient,
  type ApprovalField,
  type ApprovalFieldType,
  type ApprovalNodeData,
  type ApprovalSMSRecipient,
  type ApprovalTimeoutUnit,
} from '../config';

interface ApprovalManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

function Section({
  title,
  description,
  action,
  children,
  className,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('space-y-3', className)}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-1.5">
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          {description ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="inline-flex size-5 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
                  aria-label={description}
                >
                  <CircleAlert className="size-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent className="max-w-72 leading-5">{description}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children}
    </section>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return <Label className="text-xs font-medium text-muted-foreground">{children}</Label>;
}

function createAction(actions: ApprovalAction[], label: string): ApprovalAction {
  return {
    id: createApprovalActionId(
      actions.map(action => action.id),
      `action_${actions.length + 1}`
    ),
    label,
    style: 'secondary',
  };
}

function createField(fields: ApprovalField[], label: string): ApprovalField {
  let index = fields.length + 1;
  let key = `field_${index}`;
  const keys = new Set(fields.map(field => field.key));
  while (keys.has(key)) {
    index += 1;
    key = `field_${index}`;
  }
  return { key, label, type: 'textarea', required: false };
}

function createExternalRecipient(defaultEmail = ''): ApprovalEmailRecipient {
  return {
    type: 'external',
    email: defaultEmail,
  };
}

function createMemberRecipient(accountId = ''): ApprovalEmailRecipient {
  return {
    type: 'member',
    account_id: accountId,
  };
}

function createExternalSMSRecipient(defaultPhone = ''): ApprovalSMSRecipient {
  return {
    type: 'external',
    phone: defaultPhone,
  };
}

function createMemberSMSRecipient(accountId = ''): ApprovalSMSRecipient {
  return {
    type: 'member',
    account_id: accountId,
  };
}

function createTemplateParamKey(params: Record<string, string>): string {
  let index = Object.keys(params).length + 1;
  let key = `param_${index}`;
  while (Object.prototype.hasOwnProperty.call(params, key)) {
    index += 1;
    key = `param_${index}`;
  }
  return key;
}

function getActionButtonVariant(
  style?: ApprovalActionStyle
): React.ComponentProps<typeof Button>['variant'] {
  if (style === 'danger') return 'destructive';
  if (style === 'secondary') return 'secondary';
  return 'default';
}

const ACTION_STYLE_OPTIONS: ApprovalActionStyle[] = ['primary', 'secondary', 'danger'];
const MEMBER_PAGE_SIZE = 20;

interface ApprovalMemberOption {
  account_id: string;
  email: string;
  name?: string;
  member_name?: string;
}

function getMemberLabel(member: ApprovalMemberOption): string {
  return member.member_name || member.name || member.email;
}

function MemberRecipientSelector({
  value,
  options,
  keyword,
  disabled,
  isLoading,
  isFetching,
  isFetchingNextPage,
  hasMore,
  onKeywordChange,
  onChange,
  onLoadMore,
}: {
  value: string;
  options: ApprovalMemberOption[];
  keyword: string;
  disabled?: boolean;
  isLoading: boolean;
  isFetching: boolean;
  isFetchingNextPage: boolean;
  hasMore: boolean;
  onKeywordChange: (value: string) => void;
  onChange: (value: string) => void;
  onLoadMore: () => void;
}) {
  const t = useT('nodes');
  const [open, setOpen] = React.useState(false);
  const selectedMember = options.find(member => member.account_id === value);
  const isEmpty = !isLoading && options.length === 0;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          className="h-9 w-full justify-between px-3 font-normal"
        >
          <span className="min-w-0 truncate text-left">
            {selectedMember ? getMemberLabel(selectedMember) : t('approval.placeholders.member')}
          </span>
          <ChevronDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[var(--radix-popover-trigger-width)] p-2">
        <Input
          value={keyword}
          onChange={event => onKeywordChange(event.target.value)}
          placeholder={t('approval.placeholders.memberSearch')}
          className="mb-2 h-8"
        />
        <div className="max-h-56 overflow-y-auto">
          {options.map(member => {
            const checked = member.account_id === value;
            return (
              <button
                key={member.account_id}
                type="button"
                className={cn(
                  'flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none hover:bg-accent',
                  checked ? 'text-foreground' : 'text-muted-foreground'
                )}
                onClick={() => {
                  onChange(member.account_id);
                  setOpen(false);
                }}
              >
                <Check className={cn('size-4 shrink-0', checked ? 'opacity-100' : 'opacity-0')} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-foreground">{getMemberLabel(member)}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {member.email}
                  </span>
                </span>
              </button>
            );
          })}
          {isEmpty ? (
            <div className="px-2 py-4 text-center text-xs text-muted-foreground">
              {t('approval.empty.membersDescription')}
            </div>
          ) : null}
        </div>
        {isLoading || isFetchingNextPage ? (
          <div className="flex items-center justify-center gap-2 px-2 py-2 text-xs text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            <span>{t('approval.actions.loadingMembers')}</span>
          </div>
        ) : null}
        {hasMore ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mt-1 w-full"
            disabled={isFetching || isFetchingNextPage}
            onClick={onLoadMore}
          >
            {t('approval.actions.loadMoreMembers')}
          </Button>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}

function isApprovalEqual(a: ApprovalNodeData, b: ApprovalNodeData): boolean {
  try {
    return JSON.stringify(a) === JSON.stringify(b);
  } catch {
    return a === b;
  }
}

/**
 * @component ApprovalManager
 * @category Feature
 * @status Beta
 * @description Side panel editor for human approval workflow node configuration.
 * @usage Render inside NodeFloatingPanel when the selected workflow node type is approval.
 * @example
 * <ApprovalManager id={nodeId} />
 */
export function ApprovalManager({ id: nodeId, className, readOnly = false }: ApprovalManagerProps) {
  const t = useT('nodes');
  const contentEditorRef = React.useRef<WorkflowValueEditorHandle>(null);
  const emailBodyEditorRef = React.useRef<WorkflowValueEditorHandle>(null);
  const [emailDialogPortalRoot, setEmailDialogPortalRoot] = React.useState<HTMLDivElement | null>(
    null
  );
  const {
    localData,
    setLocalData,
    flush: flushApprovalDraft,
  } = useLocalNodeData<ApprovalNodeData>(nodeId, {
    delay: 400,
    isEqual: isApprovalEqual,
    registerPendingFlush: false,
  });
  const updateData = useNodeDataUpdate<ApprovalNodeData>(nodeId);
  const setEdges = useWorkflowStore.use.setEdges();
  const outputs = useNodeOutputVariables(nodeId);
  const currentUser = useAuthStore.use.user();
  const [timeoutDurationInput, setTimeoutDurationInput] = React.useState('');
  const [memberKeyword, setMemberKeyword] = React.useState('');
  const debouncedMemberKeyword = useDebouncedValue(memberKeyword, 300);
  const {
    members,
    isLoading: membersLoading,
    isFetching: membersFetching,
    isFetchingNextPage: membersFetchingNextPage,
    hasMore: hasMoreMembers,
    fetchNextPage: fetchNextMembersPage,
  } = useWorkspaceMembersInfinite(undefined, undefined, {
    enabled: !readOnly,
    keyword: debouncedMemberKeyword,
    limit: MEMBER_PAGE_SIZE,
  });

  const data = React.useMemo(() => normalizeApprovalNodeData(localData), [localData]);
  const dataRef = React.useRef(data);
  const localDraftDirtyRef = React.useRef(false);
  const actionHandleUpdateTimerRef = React.useRef<number | null>(null);
  const pendingActionHandleUpdatesRef = React.useRef<Map<string, string>>(new Map());
  const timeoutMaxDuration = getApprovalTimeoutMaxDuration(data.timeout.unit);
  const defaultRecipientEmail = currentUser?.email?.trim() || '';
  const defaultRecipientPhone = currentUser?.extension?.mobile?.trim() || '';
  const defaultRecipientAccountId = currentUser?.id?.trim() || '';
  const memberOptions = React.useMemo(() => {
    const memberMap = new Map<string, ApprovalMemberOption>();
    const keyword = debouncedMemberKeyword.trim().toLowerCase();

    members.forEach(member => {
      const email = member.email?.trim();
      if (!email) return;
      memberMap.set(member.id, {
        account_id: member.id,
        email,
        name: member.name,
        member_name: member.member_name,
      });
    });

    const defaultRecipientName = currentUser?.name || defaultRecipientEmail;
    const shouldIncludeDefaultRecipient =
      defaultRecipientEmail &&
      defaultRecipientAccountId &&
      !memberMap.has(defaultRecipientAccountId) &&
      (!keyword ||
        defaultRecipientEmail.toLowerCase().includes(keyword) ||
        defaultRecipientName.toLowerCase().includes(keyword));

    if (shouldIncludeDefaultRecipient) {
      memberMap.set(defaultRecipientAccountId, {
        account_id: defaultRecipientAccountId || defaultRecipientEmail,
        email: defaultRecipientEmail,
        name: defaultRecipientName,
        member_name: defaultRecipientName,
      });
    }

    return Array.from(memberMap.values());
  }, [
    debouncedMemberKeyword,
    defaultRecipientAccountId,
    defaultRecipientEmail,
    currentUser?.name,
    members,
  ]);

  const defaultMemberAccountId = React.useMemo(
    () =>
      memberOptions.find(member => member.account_id === defaultRecipientAccountId)?.account_id ||
      memberOptions[0]?.account_id ||
      '',
    [defaultRecipientAccountId, memberOptions]
  );

  React.useEffect(() => {
    dataRef.current = data;
  }, [data]);

  const flushActionHandleUpdates = React.useCallback(() => {
    if (actionHandleUpdateTimerRef.current !== null) {
      window.clearTimeout(actionHandleUpdateTimerRef.current);
      actionHandleUpdateTimerRef.current = null;
    }

    const updates = pendingActionHandleUpdatesRef.current;
    if (updates.size === 0) return;
    pendingActionHandleUpdatesRef.current = new Map();

    const currentEdges = useWorkflowStore.getState().edges;
    let hasChangedEdge = false;
    const nextEdges = currentEdges.map(edge => {
      if (edge.source !== nodeId || typeof edge.sourceHandle !== 'string') return edge;
      if (!updates.has(edge.sourceHandle)) return edge;
      const nextHandle = updates.get(edge.sourceHandle) ?? '';
      if (nextHandle === edge.sourceHandle) return edge;
      hasChangedEdge = true;
      return { ...edge, sourceHandle: nextHandle };
    });

    if (hasChangedEdge) {
      setEdges(nextEdges);
    }
  }, [nodeId, setEdges]);

  const flushApprovalPendingEdits = React.useCallback(() => {
    flushApprovalDraft();
    if (localDraftDirtyRef.current) {
      updateData(dataRef.current);
      localDraftDirtyRef.current = false;
    }
    flushActionHandleUpdates();
  }, [flushActionHandleUpdates, flushApprovalDraft, updateData]);

  React.useEffect(() => {
    return registerWorkflowPendingEditFlush(flushApprovalPendingEdits);
  }, [flushApprovalPendingEdits]);

  React.useEffect(() => flushActionHandleUpdates, [flushActionHandleUpdates]);

  React.useEffect(() => {
    setTimeoutDurationInput(String(data.timeout.duration));
  }, [data.timeout.duration]);

  const applyApprovalUpdate = React.useCallback(
    (updater: (current: ApprovalNodeData) => ApprovalNodeData): ApprovalNodeData | null => {
      const current = dataRef.current;
      const next = normalizeApprovalNodeData(updater(current));
      if (isApprovalEqual(current, next)) return null;
      dataRef.current = next;
      localDraftDirtyRef.current = true;
      setLocalData(next);
      return next;
    },
    [setLocalData]
  );

  const updateApprovalDraft = React.useCallback(
    (updater: (current: ApprovalNodeData) => ApprovalNodeData) => {
      if (readOnly) return;
      applyApprovalUpdate(updater);
    },
    [applyApprovalUpdate, readOnly]
  );

  const commitApprovalNow = React.useCallback(
    (updater: (current: ApprovalNodeData) => ApprovalNodeData): ApprovalNodeData | null => {
      if (readOnly) return null;
      const next = applyApprovalUpdate(updater);
      if (!next) return null;
      updateData(next);
      localDraftDirtyRef.current = false;
      return next;
    },
    [applyApprovalUpdate, readOnly, updateData]
  );

  const updateActionDraft = React.useCallback(
    (index: number, updater: (action: ApprovalAction) => ApprovalAction) => {
      updateApprovalDraft(current => {
        const actions = current.approval.actions.map((action, actionIndex) =>
          actionIndex === index ? updater(action) : action
        );
        return { ...current, approval: { ...current.approval, actions } };
      });
    },
    [updateApprovalDraft]
  );

  const commitActionNow = React.useCallback(
    (index: number, updater: (action: ApprovalAction) => ApprovalAction) => {
      commitApprovalNow(current => {
        const actions = current.approval.actions.map((action, actionIndex) =>
          actionIndex === index ? updater(action) : action
        );
        return { ...current, approval: { ...current.approval, actions } };
      });
    },
    [commitApprovalNow]
  );

  const scheduleActionHandleUpdate = React.useCallback(() => {
    if (actionHandleUpdateTimerRef.current !== null) {
      window.clearTimeout(actionHandleUpdateTimerRef.current);
    }
    actionHandleUpdateTimerRef.current = window.setTimeout(() => {
      flushApprovalPendingEdits();
    }, 400);
  }, [flushApprovalPendingEdits]);

  const queueActionHandleUpdate = React.useCallback(
    (previousId: string, nextId: string) => {
      if (previousId === nextId) return;
      const updates = pendingActionHandleUpdatesRef.current;
      const entries = Array.from(updates.entries());
      let chained = false;

      entries.forEach(([sourceHandle, pendingHandle]) => {
        if (pendingHandle !== previousId) return;
        chained = true;
        if (sourceHandle === nextId) {
          updates.delete(sourceHandle);
        } else {
          updates.set(sourceHandle, nextId);
        }
      });

      if (!chained) {
        updates.set(previousId, nextId);
      }

      scheduleActionHandleUpdate();
    },
    [scheduleActionHandleUpdate]
  );

  const updateActionId = React.useCallback(
    (index: number, nextId: string) => {
      if (readOnly) return;
      const action = dataRef.current.approval.actions[index];
      if (!action || action.id === nextId) return;

      const previousId = action.id;
      updateApprovalDraft(current => {
        const actions = current.approval.actions.map((item, actionIndex) =>
          actionIndex === index ? { ...item, id: nextId } : item
        );
        return { ...current, approval: { ...current.approval, actions } };
      });
      queueActionHandleUpdate(previousId, nextId);
    },
    [queueActionHandleUpdate, readOnly, updateApprovalDraft]
  );

  const deleteAction = React.useCallback(
    (index: number) => {
      const action = data.approval.actions[index];
      if (!action || readOnly) return;
      flushActionHandleUpdates();
      commitApprovalNow(current => ({
        ...current,
        approval: {
          ...current.approval,
          actions: current.approval.actions.filter((_, actionIndex) => actionIndex !== index),
        },
      }));
      const currentEdges = useWorkflowStore.getState().edges;
      setEdges(
        currentEdges.filter(edge => !(edge.source === nodeId && edge.sourceHandle === action.id))
      );
    },
    [commitApprovalNow, data.approval.actions, flushActionHandleUpdates, nodeId, readOnly, setEdges]
  );

  const updateFieldDraft = React.useCallback(
    (index: number, updater: (field: ApprovalField) => ApprovalField) => {
      updateApprovalDraft(current => ({
        ...current,
        approval: {
          ...current.approval,
          fields: current.approval.fields.map((field, fieldIndex) =>
            fieldIndex === index ? updater(field) : field
          ),
        },
      }));
    },
    [updateApprovalDraft]
  );

  const commitFieldNow = React.useCallback(
    (index: number, updater: (field: ApprovalField) => ApprovalField) => {
      commitApprovalNow(current => ({
        ...current,
        approval: {
          ...current.approval,
          fields: current.approval.fields.map((field, fieldIndex) =>
            fieldIndex === index ? updater(field) : field
          ),
        },
      }));
    },
    [commitApprovalNow]
  );

  const updateRecipientDraft = React.useCallback(
    (index: number, updater: (recipient: ApprovalEmailRecipient) => ApprovalEmailRecipient) => {
      updateApprovalDraft(current => ({
        ...current,
        submit_methods: {
          ...current.submit_methods,
          email: {
            ...current.submit_methods.email,
            recipients: current.submit_methods.email.recipients.map((recipient, recipientIndex) =>
              recipientIndex === index ? updater(recipient) : recipient
            ),
          },
        },
      }));
    },
    [updateApprovalDraft]
  );

  const commitRecipientNow = React.useCallback(
    (index: number, updater: (recipient: ApprovalEmailRecipient) => ApprovalEmailRecipient) => {
      commitApprovalNow(current => ({
        ...current,
        submit_methods: {
          ...current.submit_methods,
          email: {
            ...current.submit_methods.email,
            recipients: current.submit_methods.email.recipients.map((recipient, recipientIndex) =>
              recipientIndex === index ? updater(recipient) : recipient
            ),
          },
        },
      }));
    },
    [commitApprovalNow]
  );

  const updateSMSRecipientDraft = React.useCallback(
    (index: number, updater: (recipient: ApprovalSMSRecipient) => ApprovalSMSRecipient) => {
      updateApprovalDraft(current => ({
        ...current,
        submit_methods: {
          ...current.submit_methods,
          sms: {
            ...current.submit_methods.sms,
            recipients: current.submit_methods.sms.recipients.map((recipient, recipientIndex) =>
              recipientIndex === index ? updater(recipient) : recipient
            ),
          },
        },
      }));
    },
    [updateApprovalDraft]
  );

  const commitSMSRecipientNow = React.useCallback(
    (index: number, updater: (recipient: ApprovalSMSRecipient) => ApprovalSMSRecipient) => {
      commitApprovalNow(current => ({
        ...current,
        submit_methods: {
          ...current.submit_methods,
          sms: {
            ...current.submit_methods.sms,
            recipients: current.submit_methods.sms.recipients.map((recipient, recipientIndex) =>
              recipientIndex === index ? updater(recipient) : recipient
            ),
          },
        },
      }));
    },
    [commitApprovalNow]
  );

  const updateSMSTemplateParamKey = React.useCallback(
    (previousKey: string, nextKey: string) => {
      updateApprovalDraft(current => {
        const entries = Object.entries(current.submit_methods.sms.template_params).map(
          ([key, value]) => (key === previousKey ? [nextKey, value] : [key, value])
        );
        return {
          ...current,
          submit_methods: {
            ...current.submit_methods,
            sms: {
              ...current.submit_methods.sms,
              template_params: Object.fromEntries(entries),
            },
          },
        };
      });
    },
    [updateApprovalDraft]
  );

  const updateSMSTemplateParamValue = React.useCallback(
    (key: string, value: string) => {
      updateApprovalDraft(current => ({
        ...current,
        submit_methods: {
          ...current.submit_methods,
          sms: {
            ...current.submit_methods.sms,
            template_params: {
              ...current.submit_methods.sms.template_params,
              [key]: value,
            },
          },
        },
      }));
    },
    [updateApprovalDraft]
  );

  const fieldKeys = React.useMemo(
    () => data.approval.fields.map(field => field.key),
    [data.approval.fields]
  );
  const approvalUrlVariable = React.useMemo(
    () => ({
      sourceId: 'url',
      sourceTitle: t('approval.emailDialog.urlVariable'),
      key: '',
      label: t('approval.emailDialog.urlVariable'),
      type: 'string' as const,
    }),
    [t]
  );
  const approvalUrlSuggestItems = React.useMemo(
    () => [
      {
        sourceId: approvalUrlVariable.sourceId,
        sourceTitle: approvalUrlVariable.sourceTitle,
        key: approvalUrlVariable.key,
        type: approvalUrlVariable.type,
      },
    ],
    [approvalUrlVariable]
  );

  const handleContentVariableInsert = React.useCallback(
    (value: VariableInsertValue) => {
      if (readOnly) return;
      const key =
        value.sourceId === 'sys' && value.key.startsWith('sys.') ? value.key.slice(4) : value.key;
      contentEditorRef.current?.insertToken(value.sourceId, key);
      contentEditorRef.current?.focus();
    },
    [readOnly]
  );

  const handleEmailBodyVariableInsert = React.useCallback(
    (value: VariableInsertValue) => {
      if (readOnly) return;
      const key =
        value.sourceId === 'sys' && value.key.startsWith('sys.') ? value.key.slice(4) : value.key;
      emailBodyEditorRef.current?.insertToken(value.sourceId, key);
      emailBodyEditorRef.current?.focus();
    },
    [readOnly]
  );

  const addReplyField = React.useCallback(() => {
    commitApprovalNow(current => ({
      ...current,
      approval: {
        ...current.approval,
        fields: [
          ...current.approval.fields,
          createField(
            current.approval.fields,
            t('approval.defaults.newReplyLabel', {
              index: current.approval.fields.length + 1,
            })
          ),
        ],
      },
    }));
  }, [commitApprovalNow, t]);

  const addAction = React.useCallback(() => {
    commitApprovalNow(current => ({
      ...current,
      approval: {
        ...current.approval,
        actions: [
          ...current.approval.actions,
          createAction(
            current.approval.actions,
            t('approval.defaults.newActionLabel', {
              index: current.approval.actions.length + 1,
            })
          ),
        ],
      },
    }));
  }, [commitApprovalNow, t]);

  const updateTimeoutDuration = React.useCallback(
    (rawValue: string) => {
      setTimeoutDurationInput(rawValue);
      if (rawValue.trim() === '') return;
      const nextValue = Number(rawValue);
      if (!Number.isFinite(nextValue)) return;
      const nextDuration = Math.min(Math.max(Math.trunc(nextValue), 1), timeoutMaxDuration);
      setTimeoutDurationInput(String(nextDuration));
      updateApprovalDraft(current => ({
        ...current,
        timeout: {
          ...current.timeout,
          duration: nextDuration,
        },
      }));
    },
    [timeoutMaxDuration, updateApprovalDraft]
  );

  const restoreTimeoutDurationInput = React.useCallback(() => {
    if (timeoutDurationInput.trim() === '') {
      setTimeoutDurationInput(String(data.timeout.duration));
    }
    flushApprovalPendingEdits();
  }, [data.timeout.duration, flushApprovalPendingEdits, timeoutDurationInput]);

  const smsConfigIncomplete =
    data.submit_methods.sms.enabled &&
    (!data.submit_methods.sms.notification_title.trim() ||
      data.submit_methods.sms.recipients.length === 0);
  const smsTemplateParams = Object.entries(data.submit_methods.sms.template_params);

  return (
    <div className={cn('space-y-6', className)}>
      <Section title={t('approval.section.submitMethods')}>
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-3 rounded-lg border p-3 text-sm">
            <span>{t('approval.submit.webapp')}</span>
            <Switch
              checked={data.submit_methods.webapp.enabled}
              disabled={readOnly}
              onCheckedChange={enabled =>
                commitApprovalNow(current => ({
                  ...current,
                  submit_methods: {
                    ...current.submit_methods,
                    webapp: { enabled },
                  },
                }))
              }
            />
          </div>

          <div className="flex items-center justify-between gap-3 rounded-lg border p-3 text-sm">
            <div className="flex min-w-0 items-center gap-2">
              <Mail className="size-4 text-muted-foreground" />
              <span>{t('approval.submit.email')}</span>
              {data.submit_methods.email.enabled &&
              !data.submit_methods.email.body.includes('{{#url#}}') ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <CircleAlert className="size-3.5 text-amber-600" />
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72 leading-5">
                    {t('approval.validation.emailBodyUrlRecommended')}
                  </TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            <div className="flex items-center gap-2">
              <Dialog
                onOpenChange={open => {
                  if (!open) {
                    flushApprovalPendingEdits();
                  }
                }}
              >
                <DialogTrigger asChild>
                  <Button type="button" variant="outline" size="sm" disabled={readOnly}>
                    {t('approval.actions.editEmail')}
                  </Button>
                </DialogTrigger>
                <DialogContent size="xl" ref={setEmailDialogPortalRoot}>
                  <DialogHeader>
                    <DialogTitle>{t('approval.emailDialog.title')}</DialogTitle>
                  </DialogHeader>
                  <DialogBody className="space-y-5">
                    <div className="space-y-1.5">
                      <FieldLabel>{t('approval.submit.subject')}</FieldLabel>
                      <Input
                        value={data.submit_methods.email.subject}
                        disabled={readOnly}
                        onChange={event =>
                          updateApprovalDraft(current => ({
                            ...current,
                            submit_methods: {
                              ...current.submit_methods,
                              email: {
                                ...current.submit_methods.email,
                                subject: event.target.value,
                              },
                            },
                          }))
                        }
                        placeholder={t('approval.placeholders.emailSubject')}
                      />
                    </div>

                    <div className="space-y-2">
                      <FieldLabel>{t('approval.submit.body')}</FieldLabel>
                      <WorkflowValueInserter
                        nodeId={nodeId}
                        className="w-full"
                        onInsert={handleEmailBodyVariableInsert}
                        disabled={readOnly}
                        extraVariables={[approvalUrlVariable]}
                        extraGroupTitle={t('approval.emailDialog.specialVariables')}
                      />
                      <WorkflowValueEditor
                        ref={emailBodyEditorRef}
                        value={data.submit_methods.email.body}
                        onChange={body =>
                          updateApprovalDraft(current => ({
                            ...current,
                            submit_methods: {
                              ...current.submit_methods,
                              email: { ...current.submit_methods.email, body },
                            },
                          }))
                        }
                        readOnly={readOnly}
                        nodeId={nodeId}
                        extraSuggestItems={approvalUrlSuggestItems}
                        extraGroupTitle={t('approval.emailDialog.specialVariables')}
                        portalRoot={emailDialogPortalRoot}
                        placeholder={t('approval.placeholders.emailBody')}
                        editorClassName="min-h-[120px] max-h-[240px] overflow-y-auto"
                      />
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-center justify-between gap-2">
                        <FieldLabel>{t('approval.submit.recipients')}</FieldLabel>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          isIcon
                          disabled={readOnly}
                          onClick={() =>
                            commitApprovalNow(current => ({
                              ...current,
                              submit_methods: {
                                ...current.submit_methods,
                                email: {
                                  ...current.submit_methods.email,
                                  recipients: [
                                    ...current.submit_methods.email.recipients,
                                    createExternalRecipient(defaultRecipientEmail),
                                  ],
                                },
                              },
                            }))
                          }
                          aria-label={t('approval.actions.addRecipient')}
                          title={t('approval.actions.addRecipient')}
                        >
                          <Plus className="size-4" />
                        </Button>
                      </div>
                      {data.submit_methods.email.recipients.length === 0 ? (
                        <div className="rounded-lg border border-dashed bg-muted/20 px-4 py-5 text-center text-xs text-muted-foreground">
                          {t('approval.empty.recipientsDescription')}
                        </div>
                      ) : null}
                      {data.submit_methods.email.recipients.map((recipient, index) => {
                        const recipientType = recipient.type;
                        const externalEmail = recipient.type === 'external' ? recipient.email : '';
                        const selectedMemberAccountId =
                          recipient.type === 'member'
                            ? recipient.account_id
                            : defaultMemberAccountId;
                        const invalidEmail =
                          recipient.type === 'external' &&
                          Boolean(externalEmail.trim()) &&
                          !isValidEmail(externalEmail);
                        return (
                          <div key={index} className="grid grid-cols-[110px_1fr_32px] gap-2">
                            <Select
                              value={recipientType}
                              disabled={readOnly}
                              onValueChange={value =>
                                commitRecipientNow(index, item => {
                                  if (value === 'member') {
                                    return createMemberRecipient(
                                      item.type === 'member'
                                        ? item.account_id || defaultMemberAccountId
                                        : defaultMemberAccountId
                                    );
                                  }
                                  if (item.type === 'external') {
                                    return item;
                                  }
                                  const member = memberOptions.find(
                                    option => option.account_id === item.account_id
                                  );
                                  return createExternalRecipient(
                                    member?.email || defaultRecipientEmail
                                  );
                                })
                              }
                            >
                              <SelectTrigger>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="member">
                                  {t('approval.recipientTypes.member')}
                                </SelectItem>
                                <SelectItem value="external">
                                  {t('approval.recipientTypes.external')}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                            {recipientType === 'member' ? (
                              <MemberRecipientSelector
                                value={selectedMemberAccountId}
                                disabled={readOnly}
                                options={memberOptions}
                                keyword={memberKeyword}
                                isLoading={membersLoading}
                                isFetching={membersFetching}
                                isFetchingNextPage={membersFetchingNextPage}
                                hasMore={hasMoreMembers}
                                onKeywordChange={setMemberKeyword}
                                onLoadMore={() => {
                                  void fetchNextMembersPage();
                                }}
                                onChange={value =>
                                  commitRecipientNow(index, () => createMemberRecipient(value))
                                }
                              />
                            ) : (
                              <Input
                                value={externalEmail}
                                disabled={readOnly}
                                onChange={event =>
                                  updateRecipientDraft(index, () =>
                                    createExternalRecipient(event.target.value)
                                  )
                                }
                                placeholder={t('approval.placeholders.externalEmail')}
                                error={invalidEmail}
                              />
                            )}
                            <Button
                              variant="ghost"
                              isIcon
                              disabled={readOnly}
                              onClick={() =>
                                commitApprovalNow(current => ({
                                  ...current,
                                  submit_methods: {
                                    ...current.submit_methods,
                                    email: {
                                      ...current.submit_methods.email,
                                      recipients: current.submit_methods.email.recipients.filter(
                                        (_, recipientIndex) => recipientIndex !== index
                                      ),
                                    },
                                  },
                                }))
                              }
                              aria-label={t('approval.actions.deleteRecipient')}
                              title={t('approval.actions.deleteRecipient')}
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  </DialogBody>
                  <DialogFooter>
                    <DialogClose asChild>
                      <Button type="button" variant="outline">
                        {t('approval.emailDialog.done')}
                      </Button>
                    </DialogClose>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
              <Switch
                checked={data.submit_methods.email.enabled}
                disabled={readOnly}
                onCheckedChange={enabled =>
                  commitApprovalNow(current => {
                    const shouldAddCurrentUser =
                      enabled &&
                      defaultRecipientEmail &&
                      current.submit_methods.email.recipients.length === 0;
                    return {
                      ...current,
                      submit_methods: {
                        ...current.submit_methods,
                        email: {
                          ...current.submit_methods.email,
                          enabled,
                          recipients: shouldAddCurrentUser
                            ? [createExternalRecipient(defaultRecipientEmail)]
                            : current.submit_methods.email.recipients,
                        },
                      },
                    };
                  })
                }
              />
            </div>
          </div>

          <div className="flex items-center justify-between gap-3 rounded-lg border p-3 text-sm">
            <div className="flex min-w-0 items-center gap-2">
              <Smartphone className="size-4 text-muted-foreground" />
              <span>{t('approval.submit.sms')}</span>
              {smsConfigIncomplete ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <CircleAlert className="size-3.5 text-amber-600" />
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72 leading-5">
                    {t('approval.validation.smsConfigIncomplete')}
                  </TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            <div className="flex items-center gap-2">
              <Dialog
                onOpenChange={open => {
                  if (!open) {
                    flushApprovalPendingEdits();
                  }
                }}
              >
                <DialogTrigger asChild>
                  <Button type="button" variant="outline" size="sm" disabled={readOnly}>
                    {t('approval.actions.editSMS')}
                  </Button>
                </DialogTrigger>
                <DialogContent size="xl">
                  <DialogHeader>
                    <DialogTitle>{t('approval.smsDialog.title')}</DialogTitle>
                  </DialogHeader>
                  <DialogBody className="space-y-5">
                    <div className="space-y-1.5">
                      <FieldLabel>{t('approval.submit.smsTitle')}</FieldLabel>
                      <Input
                        value={data.submit_methods.sms.notification_title}
                        disabled={readOnly}
                        onChange={event =>
                          updateApprovalDraft(current => ({
                            ...current,
                            submit_methods: {
                              ...current.submit_methods,
                              sms: {
                                ...current.submit_methods.sms,
                                notification_title: event.target.value,
                              },
                            },
                          }))
                        }
                        placeholder={t('approval.placeholders.smsTitle')}
                        error={
                          data.submit_methods.sms.enabled &&
                          !data.submit_methods.sms.notification_title.trim()
                        }
                        errorText={
                          data.submit_methods.sms.enabled &&
                          !data.submit_methods.sms.notification_title.trim()
                            ? t('approval.validation.smsTitleRequired')
                            : undefined
                        }
                      />
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-center justify-between gap-2">
                        <FieldLabel>{t('approval.submit.templateParams')}</FieldLabel>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          isIcon
                          disabled={readOnly}
                          onClick={() =>
                            commitApprovalNow(current => {
                              const key = createTemplateParamKey(
                                current.submit_methods.sms.template_params
                              );
                              return {
                                ...current,
                                submit_methods: {
                                  ...current.submit_methods,
                                  sms: {
                                    ...current.submit_methods.sms,
                                    template_params: {
                                      ...current.submit_methods.sms.template_params,
                                      [key]: '',
                                    },
                                  },
                                },
                              };
                            })
                          }
                          aria-label={t('approval.actions.addTemplateParam')}
                          title={t('approval.actions.addTemplateParam')}
                        >
                          <Plus className="size-4" />
                        </Button>
                      </div>
                      {smsTemplateParams.length === 0 ? (
                        <div className="rounded-lg border border-dashed bg-muted/20 px-4 py-5 text-center text-xs text-muted-foreground">
                          {t('approval.empty.templateParamsDescription')}
                        </div>
                      ) : null}
                      {smsTemplateParams.map(([key, value]) => (
                        <div
                          key={key}
                          className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_32px] gap-2"
                        >
                          <Input
                            value={key}
                            disabled={readOnly}
                            onChange={event => updateSMSTemplateParamKey(key, event.target.value)}
                            onBlur={flushApprovalPendingEdits}
                            placeholder={t('approval.placeholders.templateParamKey')}
                            className="h-8 font-mono text-xs"
                          />
                          <Input
                            value={value}
                            disabled={readOnly}
                            onChange={event => updateSMSTemplateParamValue(key, event.target.value)}
                            onBlur={flushApprovalPendingEdits}
                            placeholder={t('approval.placeholders.templateParamValue')}
                            className="h-8 text-xs"
                          />
                          <Button
                            variant="ghost"
                            isIcon
                            disabled={readOnly}
                            onClick={() =>
                              commitApprovalNow(current => {
                                const nextParams = {
                                  ...current.submit_methods.sms.template_params,
                                };
                                delete nextParams[key];
                                return {
                                  ...current,
                                  submit_methods: {
                                    ...current.submit_methods,
                                    sms: {
                                      ...current.submit_methods.sms,
                                      template_params: nextParams,
                                    },
                                  },
                                };
                              })
                            }
                            aria-label={t('approval.actions.deleteTemplateParam')}
                            title={t('approval.actions.deleteTemplateParam')}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      ))}
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-center justify-between gap-2">
                        <FieldLabel>{t('approval.submit.recipients')}</FieldLabel>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          isIcon
                          disabled={readOnly}
                          onClick={() =>
                            commitApprovalNow(current => ({
                              ...current,
                              submit_methods: {
                                ...current.submit_methods,
                                sms: {
                                  ...current.submit_methods.sms,
                                  recipients: [
                                    ...current.submit_methods.sms.recipients,
                                    createExternalSMSRecipient(defaultRecipientPhone),
                                  ],
                                },
                              },
                            }))
                          }
                          aria-label={t('approval.actions.addRecipient')}
                          title={t('approval.actions.addRecipient')}
                        >
                          <Plus className="size-4" />
                        </Button>
                      </div>
                      {data.submit_methods.sms.recipients.length === 0 ? (
                        <div className="rounded-lg border border-dashed bg-muted/20 px-4 py-5 text-center text-xs text-muted-foreground">
                          {t('approval.empty.smsRecipientsDescription')}
                        </div>
                      ) : null}
                      {data.submit_methods.sms.recipients.map((recipient, index) => {
                        const recipientType = recipient.type;
                        const externalPhone = recipient.type === 'external' ? recipient.phone : '';
                        const selectedMemberAccountId =
                          recipient.type === 'member'
                            ? recipient.account_id
                            : defaultMemberAccountId;
                        const missingPhone =
                          recipient.type === 'external' &&
                          data.submit_methods.sms.enabled &&
                          !externalPhone.trim();
                        return (
                          <div key={index} className="grid grid-cols-[110px_1fr_32px] gap-2">
                            <Select
                              value={recipientType}
                              disabled={readOnly}
                              onValueChange={value =>
                                commitSMSRecipientNow(index, item => {
                                  if (value === 'member') {
                                    return createMemberSMSRecipient(
                                      item.type === 'member'
                                        ? item.account_id || defaultMemberAccountId
                                        : defaultMemberAccountId
                                    );
                                  }
                                  if (item.type === 'external') {
                                    return item;
                                  }
                                  return createExternalSMSRecipient(defaultRecipientPhone);
                                })
                              }
                            >
                              <SelectTrigger>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="member">
                                  {t('approval.recipientTypes.member')}
                                </SelectItem>
                                <SelectItem value="external">
                                  {t('approval.recipientTypes.externalPhone')}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                            {recipientType === 'member' ? (
                              <MemberRecipientSelector
                                value={selectedMemberAccountId}
                                disabled={readOnly}
                                options={memberOptions}
                                keyword={memberKeyword}
                                isLoading={membersLoading}
                                isFetching={membersFetching}
                                isFetchingNextPage={membersFetchingNextPage}
                                hasMore={hasMoreMembers}
                                onKeywordChange={setMemberKeyword}
                                onLoadMore={() => {
                                  void fetchNextMembersPage();
                                }}
                                onChange={value =>
                                  commitSMSRecipientNow(index, () =>
                                    createMemberSMSRecipient(value)
                                  )
                                }
                              />
                            ) : (
                              <Input
                                value={externalPhone}
                                disabled={readOnly}
                                onChange={event =>
                                  updateSMSRecipientDraft(index, () =>
                                    createExternalSMSRecipient(event.target.value)
                                  )
                                }
                                placeholder={t('approval.placeholders.externalPhone')}
                                error={missingPhone}
                                errorText={
                                  missingPhone
                                    ? t('approval.validation.smsExternalRecipientRequired', {
                                        index: index + 1,
                                      })
                                    : undefined
                                }
                              />
                            )}
                            <Button
                              variant="ghost"
                              isIcon
                              disabled={readOnly}
                              onClick={() =>
                                commitApprovalNow(current => ({
                                  ...current,
                                  submit_methods: {
                                    ...current.submit_methods,
                                    sms: {
                                      ...current.submit_methods.sms,
                                      recipients: current.submit_methods.sms.recipients.filter(
                                        (_, recipientIndex) => recipientIndex !== index
                                      ),
                                    },
                                  },
                                }))
                              }
                              aria-label={t('approval.actions.deleteRecipient')}
                              title={t('approval.actions.deleteRecipient')}
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  </DialogBody>
                  <DialogFooter>
                    <DialogClose asChild>
                      <Button type="button" variant="outline">
                        {t('approval.emailDialog.done')}
                      </Button>
                    </DialogClose>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
              <Switch
                checked={data.submit_methods.sms.enabled}
                disabled={readOnly}
                onCheckedChange={enabled =>
                  commitApprovalNow(current => {
                    const shouldAddCurrentUser =
                      enabled &&
                      defaultMemberAccountId &&
                      current.submit_methods.sms.recipients.length === 0;
                    return {
                      ...current,
                      submit_methods: {
                        ...current.submit_methods,
                        sms: {
                          ...current.submit_methods.sms,
                          enabled,
                          notification_title:
                            current.submit_methods.sms.notification_title ||
                            current.title ||
                            t('approval.defaults.smsTitle'),
                          recipients: shouldAddCurrentUser
                            ? [createMemberSMSRecipient(defaultMemberAccountId)]
                            : current.submit_methods.sms.recipients,
                        },
                      },
                    };
                  })
                }
              />
            </div>
          </div>
        </div>
      </Section>

      <Section
        title={t('approval.section.content')}
        description={t('approval.description.content')}
      >
        <WorkflowValueInserter
          nodeId={nodeId}
          className="w-full"
          onInsert={handleContentVariableInsert}
          disabled={readOnly}
        />
        <WorkflowValueEditor
          ref={contentEditorRef}
          value={data.approval.content}
          onChange={content =>
            updateApprovalDraft(current => ({
              ...current,
              approval: { ...current.approval, content },
            }))
          }
          readOnly={readOnly}
          nodeId={nodeId}
          placeholder={t('approval.placeholders.content')}
          editorClassName="min-h-[160px]"
        />
      </Section>

      <Section
        title={t('approval.section.actions')}
        description={t('approval.description.actions')}
        action={
          <Button
            variant="ghost"
            size="sm"
            isIcon
            disabled={readOnly}
            onClick={addAction}
            aria-label={t('approval.actions.addAction')}
            title={t('approval.actions.addAction')}
          >
            <Plus className="size-4" />
          </Button>
        }
      >
        <div className="space-y-2">
          {data.approval.actions.length === 0 ? (
            <div className="rounded-lg border border-dashed bg-muted/20 px-4 py-6 text-center">
              <div className="text-sm font-medium text-foreground">
                {t('approval.empty.actionsTitle')}
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('approval.empty.actionsDescription')}
              </p>
            </div>
          ) : null}
          {data.approval.actions.map((action, index) => {
            const duplicate =
              Boolean(action.id) &&
              data.approval.actions.findIndex(item => item.id === action.id) !== index;
            const reserved = action.id === APPROVAL_TIMEOUT_HANDLE;
            const invalidId = !action.id.trim() || !APPROVAL_IDENTIFIER_PATTERN.test(action.id);
            const actionIdTooLong = action.id.length > APPROVAL_ACTION_MAX_LENGTH;
            const actionLabelTooLong = action.label.length > APPROVAL_ACTION_MAX_LENGTH;

            return (
              <div key={index} className="space-y-3 rounded-lg border p-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 grow space-y-1.5">
                    <FieldLabel>{t('approval.actionsConfig.id')}</FieldLabel>
                    <Input
                      value={action.id}
                      disabled={readOnly}
                      onChange={event => updateActionId(index, event.target.value)}
                      onBlur={flushApprovalPendingEdits}
                      className="h-8 font-mono text-xs"
                      maxLength={APPROVAL_ACTION_MAX_LENGTH}
                      aria-label={t('approval.actionsConfig.id')}
                      error={Boolean(duplicate || reserved || actionIdTooLong || invalidId)}
                      errorText={
                        duplicate
                          ? t('approval.validation.actionIdDuplicate', { index: index + 1 })
                          : reserved
                            ? t('approval.validation.actionIdReserved', { index: index + 1 })
                            : actionIdTooLong
                              ? t('approval.validation.actionIdTooLong', { index: index + 1 })
                              : invalidId
                                ? t('approval.validation.actionIdInvalid', { index: index + 1 })
                                : undefined
                      }
                    />
                  </div>
                  <Button
                    variant="ghost"
                    isIcon
                    disabled={readOnly}
                    onClick={() => deleteAction(index)}
                    aria-label={t('approval.actions.deleteAction')}
                    title={t('approval.actions.deleteAction')}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
                <div className="grid grid-cols-[minmax(0,1fr)_128px] items-start gap-2">
                  <div className="space-y-1.5">
                    <FieldLabel>{t('approval.actionsConfig.label')}</FieldLabel>
                    <Input
                      value={action.label}
                      disabled={readOnly}
                      className="h-8 text-xs"
                      maxLength={APPROVAL_ACTION_MAX_LENGTH}
                      onChange={event =>
                        updateActionDraft(index, item => ({ ...item, label: event.target.value }))
                      }
                      aria-label={t('approval.actionsConfig.label')}
                      error={actionLabelTooLong}
                      errorText={
                        actionLabelTooLong
                          ? t('approval.validation.actionLabelTooLong', { index: index + 1 })
                          : undefined
                      }
                    />
                  </div>
                  <div className="space-y-1.5">
                    <FieldLabel>{t('approval.actionsConfig.style')}</FieldLabel>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild disabled={readOnly}>
                        <Button
                          type="button"
                          size="sm"
                          variant={getActionButtonVariant(action.style)}
                          className="w-32 truncate px-3"
                          title={t('approval.actionsConfig.style')}
                        >
                          <span className="truncate">
                            {action.label || t('approval.actionsConfig.preview')}
                          </span>
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-40">
                        {ACTION_STYLE_OPTIONS.map(style => (
                          <DropdownMenuItem
                            key={style}
                            onSelect={() =>
                              commitActionNow(index, item => ({
                                ...item,
                                style,
                              }))
                            }
                            className="p-1"
                          >
                            <span
                              className={cn(
                                buttonVariants({
                                  variant: getActionButtonVariant(style),
                                  size: 'sm',
                                  interactive: false,
                                }),
                                'w-full truncate px-3'
                              )}
                            >
                              {t(`approval.actionStyles.${style}`)}
                            </span>
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </Section>

      <Section
        title={t('approval.section.fields')}
        description={t('approval.description.fields')}
        action={
          <Button
            variant="ghost"
            size="sm"
            isIcon
            disabled={readOnly}
            onClick={addReplyField}
            aria-label={t('approval.actions.addField')}
            title={t('approval.actions.addField')}
          >
            <Plus className="size-4" />
          </Button>
        }
      >
        <div className="space-y-3">
          {data.approval.fields.length === 0 ? (
            <div className="rounded-lg border border-dashed bg-muted/20 px-4 py-6 text-center">
              <div className="text-sm font-medium text-foreground">
                {t('approval.empty.fieldsTitle')}
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('approval.empty.fieldsDescription')}
              </p>
            </div>
          ) : null}
          {data.approval.fields.map((field, index) => {
            const duplicate = field.key && fieldKeys.indexOf(field.key) !== index;
            const invalidKey =
              field.key.trim().length > 0 && !APPROVAL_IDENTIFIER_PATTERN.test(field.key);
            const defaultValue = field.default;

            return (
              <div key={`${field.key}-${index}`} className="space-y-3 rounded-lg border p-3">
                <div className="flex items-start gap-2">
                  <div className="grid min-w-0 grow grid-cols-2 gap-2">
                    <div className="space-y-1.5">
                      <FieldLabel>{t('approval.fields.key')}</FieldLabel>
                      <Input
                        value={field.key}
                        onChange={event =>
                          updateFieldDraft(index, item => ({ ...item, key: event.target.value }))
                        }
                        disabled={readOnly}
                        error={Boolean(duplicate || invalidKey)}
                        errorText={
                          duplicate
                            ? t('approval.validation.fieldKeyDuplicate', { index: index + 1 })
                            : invalidKey
                              ? t('approval.validation.fieldKeyInvalid', { index: index + 1 })
                              : undefined
                        }
                      />
                    </div>
                    <div className="space-y-1.5">
                      <FieldLabel>{t('approval.fields.label')}</FieldLabel>
                      <Input
                        value={field.label}
                        onChange={event =>
                          updateFieldDraft(index, item => ({ ...item, label: event.target.value }))
                        }
                        disabled={readOnly}
                      />
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    isIcon
                    disabled={readOnly}
                    onClick={() =>
                      commitApprovalNow(current => ({
                        ...current,
                        approval: {
                          ...current.approval,
                          fields: current.approval.fields.filter(
                            (_, fieldIndex) => fieldIndex !== index
                          ),
                        },
                      }))
                    }
                    aria-label={t('approval.actions.deleteField')}
                    title={t('approval.actions.deleteField')}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>

                <div className="grid grid-cols-2 gap-2">
                  <div className="space-y-1.5">
                    <FieldLabel>{t('approval.fields.type')}</FieldLabel>
                    <Select
                      value={field.type}
                      disabled={readOnly}
                      onValueChange={value =>
                        commitFieldNow(index, item => ({
                          ...item,
                          type: value as ApprovalFieldType,
                        }))
                      }
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="text">{t('approval.fieldTypes.text')}</SelectItem>
                        <SelectItem value="textarea">
                          {t('approval.fieldTypes.textarea')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <label className="flex items-end gap-2 pb-2 text-sm">
                    <Checkbox
                      checked={field.required}
                      disabled={readOnly}
                      onCheckedChange={checked =>
                        commitFieldNow(index, item => ({ ...item, required: Boolean(checked) }))
                      }
                    />
                    {t('approval.fields.required')}
                  </label>
                </div>

                <div className="space-y-1.5">
                  <FieldLabel>{t('approval.fields.defaultValue')}</FieldLabel>
                  <div className="grid grid-cols-[120px_1fr] gap-2">
                    <Select
                      value={defaultValue?.type ?? 'none'}
                      disabled={readOnly}
                      onValueChange={value =>
                        commitFieldNow(index, item => ({
                          ...item,
                          default:
                            value === 'variable'
                              ? ({ type: 'variable', selector: [] } as ApprovalDefaultValue)
                              : value === 'constant'
                                ? ({ type: 'constant', value: '' } as ApprovalDefaultValue)
                                : undefined,
                        }))
                      }
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">{t('approval.defaultTypes.none')}</SelectItem>
                        <SelectItem value="constant">
                          {t('approval.defaultTypes.constant')}
                        </SelectItem>
                        <SelectItem value="variable">
                          {t('approval.defaultTypes.variable')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    {defaultValue?.type === 'variable' ? (
                      <NodeValueSelector
                        nodeId={nodeId}
                        value={defaultValue.selector}
                        disabled={readOnly}
                        onChange={payload =>
                          commitFieldNow(index, item => ({
                            ...item,
                            default: { type: 'variable', selector: payload.valuePath },
                          }))
                        }
                        placeholder={t('approval.placeholders.defaultSelector')}
                      />
                    ) : defaultValue?.type === 'constant' ? (
                      <Input
                        value={defaultValue.value}
                        disabled={readOnly}
                        onChange={event =>
                          updateFieldDraft(index, item => ({
                            ...item,
                            default: { type: 'constant', value: event.target.value },
                          }))
                        }
                        placeholder={t('approval.placeholders.defaultValue')}
                      />
                    ) : (
                      <div className="flex h-9 items-center rounded-lg border px-3 text-sm text-muted-foreground">
                        {t('approval.defaultTypes.none')}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </Section>

      <Section title={t('approval.section.timeout')}>
        <div className="grid grid-cols-[1fr_120px] gap-2">
          <Input
            type="number"
            min={1}
            max={timeoutMaxDuration}
            step={1}
            value={timeoutDurationInput}
            disabled={readOnly}
            onChange={event => updateTimeoutDuration(event.target.value)}
            onBlur={restoreTimeoutDurationInput}
            error={
              timeoutDurationInput.trim() !== '' &&
              Number(timeoutDurationInput) > timeoutMaxDuration
            }
            errorText={
              timeoutDurationInput.trim() !== '' &&
              Number(timeoutDurationInput) > timeoutMaxDuration
                ? t('approval.validation.timeoutDurationTooLong')
                : undefined
            }
          />
          <Select
            value={data.timeout.unit}
            disabled={readOnly}
            onValueChange={value => {
              const nextUnit = value as ApprovalTimeoutUnit;
              const nextMax = getApprovalTimeoutMaxDuration(nextUnit);
              const nextDuration = Math.min(data.timeout.duration, nextMax);
              setTimeoutDurationInput(String(nextDuration));
              commitApprovalNow(current => ({
                ...current,
                timeout: { ...current.timeout, duration: nextDuration, unit: nextUnit },
              }));
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="minute">{t('approval.timeout.minute')}</SelectItem>
              <SelectItem value="hour">{t('approval.timeout.hour')}</SelectItem>
              <SelectItem value="day">{t('approval.timeout.day')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </Section>

      <OutputVariablesView variables={outputs} title={t('common.outputVariables')} />
    </div>
  );
}

export default ApprovalManager;
