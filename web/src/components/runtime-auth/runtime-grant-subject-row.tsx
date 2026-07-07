'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Check, ChevronsUpDown, Search, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { DepartmentSelector } from '@/components/dashboard/organization/department-selector';
import { useCurrentOrganizationMember } from '@/hooks/organization/use-current-organization-member';
import { useCurrentOrganizationMembers } from '@/hooks/organization/use-current-organization-members';
import { useDepartments } from '@/hooks/organization/use-departments';
import { cn } from '@/lib/utils';
import type { Department, Member } from '@/services/types/organization';

export const RUNTIME_GRANT_SUBJECT_TYPES = ['organization', 'department', 'account'] as const;

export type RuntimeGrantSubjectType = (typeof RUNTIME_GRANT_SUBJECT_TYPES)[number];

export interface RuntimeGrantSubjectLabels {
  subjectLabels: Record<RuntimeGrantSubjectType, string>;
  organizationWide: string;
  departmentPlaceholder: string;
  accountPlaceholder: string;
  searchMembersPlaceholder: string;
  noMembers: string;
  loadingMembers: string;
  resolvingAccount: string;
  selectionRequired: string;
  accountLookupFailed: string;
  departmentLookupFailed: string;
  unresolvedAccount: string;
  unresolvedDepartment: string;
  removeGrant: string;
}

interface RuntimeGrantSubjectRowProps {
  subjectType: RuntimeGrantSubjectType;
  subjectId: string;
  disabled?: boolean;
  canRemove?: boolean;
  labels: RuntimeGrantSubjectLabels;
  onChange: (next: { subject_type: RuntimeGrantSubjectType; subject_id: string }) => void;
  onRemove: () => void;
}

function memberLabel(member: Member): string {
  const name = member.member_name || member.name || member.email || member.id;
  return member.email && member.email !== name ? `${name} (${member.email})` : name;
}

function departmentIncludesId(departments: Department[], departmentId: string): boolean {
  for (const department of departments) {
    if (department.id === departmentId) {
      return true;
    }
    if (department.children?.length && departmentIncludesId(department.children, departmentId)) {
      return true;
    }
  }
  return false;
}

export function RuntimeGrantSubjectRow({
  subjectType,
  subjectId,
  disabled = false,
  canRemove = true,
  labels,
  onChange,
  onRemove,
}: RuntimeGrantSubjectRowProps) {
  const [accountOpen, setAccountOpen] = useState(false);
  const [memberKeyword, setMemberKeyword] = useState('');
  const [selectedAccountLabel, setSelectedAccountLabel] = useState('');
  const {
    departments,
    isLoading: isDepartmentsLoading,
    isFetching: isDepartmentsFetching,
    error: departmentsError,
  } = useDepartments();
  const {
    members,
    isLoading: isMembersLoading,
    isFetching: isMembersFetching,
  } = useCurrentOrganizationMembers({
    keyword: memberKeyword,
    limit: 30,
    enabled: accountOpen && subjectType === 'account' && !disabled,
  });
  const {
    member: hydratedAccountGrantMember,
    isLoading: isAccountGrantLoading,
    isFetching: isAccountGrantFetching,
    error: accountGrantError,
  } = useCurrentOrganizationMember(subjectId, {
    enabled: subjectType === 'account' && Boolean(subjectId),
  });

  const normalizedSubjectId = subjectId.trim();
  const selectedMember = useMemo(
    () => members.find(member => member.id === normalizedSubjectId),
    [members, normalizedSubjectId]
  );
  const selectedDepartmentExists = useMemo(
    () => Boolean(normalizedSubjectId) && departmentIncludesId(departments, normalizedSubjectId),
    [departments, normalizedSubjectId]
  );

  useEffect(() => {
    if (!normalizedSubjectId) {
      setSelectedAccountLabel('');
      return;
    }
    if (selectedMember) {
      setSelectedAccountLabel(memberLabel(selectedMember));
      return;
    }
    if (hydratedAccountGrantMember) {
      setSelectedAccountLabel(memberLabel(hydratedAccountGrantMember));
      return;
    }
    setSelectedAccountLabel('');
  }, [hydratedAccountGrantMember, normalizedSubjectId, selectedMember]);

  const handleSubjectTypeChange = (value: string) => {
    const nextType = value as RuntimeGrantSubjectType;
    onChange({
      subject_type: nextType,
      subject_id: nextType === 'organization' ? '' : subjectId,
    });
    if (nextType !== 'account') {
      setAccountOpen(false);
      setMemberKeyword('');
    }
  };

  const accountGrantLoading = isAccountGrantLoading || isAccountGrantFetching;
  const missingSubjectSelection = subjectType !== 'organization' && !normalizedSubjectId;
  const accountGrantLookupFailed =
    subjectType === 'account' &&
    Boolean(normalizedSubjectId) &&
    Boolean(accountGrantError) &&
    !selectedMember &&
    !hydratedAccountGrantMember &&
    !accountGrantLoading;
  const accountGrantUnresolved =
    subjectType === 'account' &&
    Boolean(normalizedSubjectId) &&
    !accountGrantError &&
    !selectedMember &&
    !hydratedAccountGrantMember &&
    !accountGrantLoading;
  const departmentGrantLookupFailed =
    subjectType === 'department' &&
    Boolean(normalizedSubjectId) &&
    !isDepartmentsLoading &&
    !isDepartmentsFetching &&
    Boolean(departmentsError);
  const departmentGrantUnresolved =
    subjectType === 'department' &&
    Boolean(normalizedSubjectId) &&
    !isDepartmentsLoading &&
    !isDepartmentsFetching &&
    !departmentsError &&
    !selectedDepartmentExists;
  const grantStateMessage = missingSubjectSelection
    ? labels.selectionRequired
    : accountGrantLookupFailed
      ? `${labels.accountLookupFailed}: ${normalizedSubjectId}`
      : departmentGrantLookupFailed
        ? `${labels.departmentLookupFailed}: ${normalizedSubjectId}`
        : accountGrantUnresolved
          ? `${labels.unresolvedAccount}: ${normalizedSubjectId}`
          : departmentGrantUnresolved
            ? `${labels.unresolvedDepartment}: ${normalizedSubjectId}`
            : null;
  const accountNeedsAttention =
    subjectType === 'account' &&
    (missingSubjectSelection || accountGrantLookupFailed || accountGrantUnresolved);
  const departmentNeedsAttention =
    subjectType === 'department' &&
    (missingSubjectSelection || departmentGrantLookupFailed || departmentGrantUnresolved);
  const accountDisplay = accountGrantLoading && !selectedAccountLabel
    ? labels.resolvingAccount
    : accountGrantLookupFailed
      ? labels.accountLookupFailed
      : accountGrantUnresolved
        ? labels.unresolvedAccount
        : selectedAccountLabel || normalizedSubjectId || labels.accountPlaceholder;
  const membersLoading = isMembersLoading || isMembersFetching;

  return (
    <div
      className={cn(
        'rounded-md bg-muted/30 p-2',
        grantStateMessage &&
          'border border-amber-300/70 bg-amber-50/60 dark:border-amber-900/70 dark:bg-amber-950/20'
      )}
    >
      <div className="grid gap-2 sm:grid-cols-[minmax(0,180px)_minmax(0,1fr)_32px]">
        <Select value={subjectType} disabled={disabled} onValueChange={handleSubjectTypeChange}>
          <SelectTrigger className="h-8 rounded-md">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RUNTIME_GRANT_SUBJECT_TYPES.map(subject => (
              <SelectItem key={subject} value={subject}>
                {labels.subjectLabels[subject]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {subjectType === 'organization' ? (
          <Input className="h-8 rounded-md" disabled value={labels.organizationWide} />
        ) : null}

        {subjectType === 'department' ? (
          <DepartmentSelector
            departments={departments}
            value={subjectId}
            onValueChange={value => onChange({ subject_type: 'department', subject_id: value })}
            placeholder={labels.departmentPlaceholder}
            disabled={disabled || isDepartmentsLoading}
            className={cn(
              'h-8 rounded-md',
              departmentNeedsAttention &&
                'border-amber-300 text-amber-700 dark:border-amber-900 dark:text-amber-300'
            )}
            contentClassName="max-h-72"
          />
        ) : null}

        {subjectType === 'account' ? (
          <Popover open={accountOpen} onOpenChange={setAccountOpen}>
            <PopoverTrigger asChild>
              <Button
                type="button"
                variant="outline"
                role="combobox"
                aria-expanded={accountOpen}
                disabled={disabled}
                className={cn(
                  'h-8 w-full justify-between rounded-md px-3 font-normal',
                  !subjectId && 'text-muted-foreground',
                  accountNeedsAttention &&
                    'border-amber-300 text-amber-700 dark:border-amber-900 dark:text-amber-300'
                )}
              >
                <span className="truncate text-left">{accountDisplay}</span>
                <ChevronsUpDown className="size-4 opacity-50" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-[var(--radix-popover-trigger-width)] p-2">
              <Input
                value={memberKeyword}
                onChange={event => setMemberKeyword(event.target.value)}
                placeholder={labels.searchMembersPlaceholder}
                leftIcon={<Search />}
                className="h-8 rounded-md"
              />
              <div className="mt-2 max-h-60 overflow-y-auto">
                {membersLoading ? (
                  <div className="px-2 py-3 text-sm text-muted-foreground">
                    {labels.loadingMembers}
                  </div>
                ) : members.length === 0 ? (
                  <div className="px-2 py-3 text-sm text-muted-foreground">
                    {labels.noMembers}
                  </div>
                ) : (
                  members.map(member => {
                    const label = memberLabel(member);
                    const selected = member.id === subjectId;
                    return (
                      <button
                        key={member.id}
                        type="button"
                        className={cn(
                          'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground',
                          selected && 'bg-accent text-accent-foreground'
                        )}
                        onClick={() => {
                          setSelectedAccountLabel(label);
                          onChange({ subject_type: 'account', subject_id: member.id });
                          setAccountOpen(false);
                        }}
                      >
                        <span className="min-w-0 flex-1 truncate">{label}</span>
                        {selected ? <Check className="size-4 text-primary" /> : null}
                      </button>
                    );
                  })
                )}
              </div>
            </PopoverContent>
          </Popover>
        ) : null}

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              isIcon
              className="h-8 w-8 rounded-md text-muted-foreground hover:text-destructive"
              disabled={disabled || !canRemove}
              aria-label={labels.removeGrant}
              onClick={onRemove}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent className="text-xs">{labels.removeGrant}</TooltipContent>
        </Tooltip>
      </div>

      {grantStateMessage ? (
        <div className="mt-2 flex items-start gap-1.5 rounded-md border border-amber-300/70 bg-background/80 px-2 py-1.5 text-xs leading-5 text-amber-700 dark:border-amber-900/70 dark:text-amber-300">
          <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span className="min-w-0 break-all">{grantStateMessage}</span>
        </div>
      ) : null}
    </div>
  );
}
