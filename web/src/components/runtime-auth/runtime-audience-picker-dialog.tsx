'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Boxes, Building2, Search, UserRound, Users, X } from 'lucide-react';
import { SafeAvatar } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useCurrentOrganizationMember } from '@/hooks/organization/use-current-organization-member';
import { useCurrentOrganizationMembers } from '@/hooks/organization/use-current-organization-members';
import { useDepartments } from '@/hooks/organization/use-departments';
import { useWorkspaces } from '@/hooks/workspace/use-workspaces';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { Department, Member } from '@/services/types/organization';
import type { WorkspaceManagement } from '@/services/types/workspace';

export type RuntimeAudienceSubjectType = 'organization' | 'department' | 'workspace' | 'account';

export interface RuntimeAudienceGrant {
  subject_type: RuntimeAudienceSubjectType;
  subject_id: string;
}

interface RuntimeAudiencePickerDialogProps {
  open: boolean;
  title: string;
  description?: string;
  value: RuntimeAudienceGrant[];
  excludeWorkspaceId?: string | null;
  disabled?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (value: RuntimeAudienceGrant[]) => void;
}

interface DepartmentRow {
  department: Department;
  depth: number;
}

export function getRuntimeAudienceMemberLabel(member: Member): string {
  const displayName = member.member_name || member.name;
  if (displayName) {
    const email = member.email?.trim();
    if (!email) {
      return displayName;
    }
    const escapedEmail = email.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const withoutEmailSuffix = displayName
      .replace(new RegExp(`\\s*\\(${escapedEmail}\\)\\s*$`, 'i'), '')
      .trim();
    return withoutEmailSuffix || displayName;
  }
  return member.email || member.id;
}

export function dedupeRuntimeAudienceGrants(
  grants: RuntimeAudienceGrant[]
): RuntimeAudienceGrant[] {
  const seen = new Set<string>();
  const normalized: RuntimeAudienceGrant[] = [];

  for (const grant of grants) {
    const subjectId = grant.subject_type === 'organization' ? '' : grant.subject_id.trim();
    if (grant.subject_type !== 'organization' && !subjectId) {
      continue;
    }
    const key = `${grant.subject_type}:${subjectId}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    normalized.push({
      subject_type: grant.subject_type,
      subject_id: subjectId,
    });
  }

  return normalized;
}

function flattenDepartments(departments: Department[], depth = 0): DepartmentRow[] {
  return departments.flatMap(department => [
    { department, depth },
    ...flattenDepartments(department.children ?? [], depth + 1),
  ]);
}

function buildDepartmentMap(departments: Department[]): Map<string, Department> {
  const rows = flattenDepartments(departments);
  return new Map(rows.map(row => [row.department.id, row.department]));
}

function buildWorkspaceMap(workspaces: WorkspaceManagement[]): Map<string, WorkspaceManagement> {
  return new Map(workspaces.map(workspace => [workspace.id, workspace]));
}

function grantKey(grant: RuntimeAudienceGrant): string {
  return `${grant.subject_type}:${grant.subject_id}`;
}

function pickerValue(value: RuntimeAudienceGrant[]): RuntimeAudienceGrant[] {
  return dedupeRuntimeAudienceGrants(
    value.filter(
      grant =>
        grant.subject_type === 'department' ||
        grant.subject_type === 'workspace' ||
        grant.subject_type === 'account'
    )
  );
}

export function RuntimeAudiencePickerDialog({
  open,
  title,
  description,
  value,
  excludeWorkspaceId,
  disabled = false,
  onOpenChange,
  onConfirm,
}: RuntimeAudiencePickerDialogProps) {
  const t = useT('agents.runtimeAccess');
  const [draft, setDraft] = useState<RuntimeAudienceGrant[]>([]);
  const [activeTab, setActiveTab] = useState('departments');
  const [memberKeyword, setMemberKeyword] = useState('');
  const {
    departments,
    isLoading: isDepartmentsLoading,
    isFetching: isDepartmentsFetching,
  } = useDepartments({ enabled: open });
  const {
    workspaces,
    isLoading: isWorkspacesLoading,
    isFetching: isWorkspacesFetching,
  } = useWorkspaces('', 1, 1000, { keepPreviousData: true, enabled: open });
  const {
    members,
    isLoading: isMembersLoading,
    isFetching: isMembersFetching,
  } = useCurrentOrganizationMembers({
    keyword: memberKeyword,
    limit: 100,
    enabled: open,
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    setDraft(pickerValue(value));
    setMemberKeyword('');
    setActiveTab('departments');
  }, [open, value]);

  const departmentRows = useMemo(() => flattenDepartments(departments), [departments]);
  const visibleWorkspaces = useMemo(() => {
    const excludedWorkspaceId = excludeWorkspaceId?.trim();
    if (!excludedWorkspaceId) {
      return workspaces;
    }
    return workspaces.filter(workspace => workspace.id !== excludedWorkspaceId);
  }, [excludeWorkspaceId, workspaces]);
  const selectedKeys = useMemo(() => new Set(draft.map(grantKey)), [draft]);
  const selectedDepartments = draft.filter(grant => grant.subject_type === 'department').length;
  const selectedWorkspaces = draft.filter(grant => grant.subject_type === 'workspace').length;
  const selectedAccounts = draft.filter(grant => grant.subject_type === 'account').length;
  const isMembersLoadingState = isMembersLoading || isMembersFetching;
  const isDepartmentsLoadingState = isDepartmentsLoading || isDepartmentsFetching;
  const isWorkspacesLoadingState = isWorkspacesLoading || isWorkspacesFetching;

  const toggleGrant = (grant: RuntimeAudienceGrant, checked: boolean) => {
    setDraft(current => {
      if (checked) {
        return dedupeRuntimeAudienceGrants([...current, grant]);
      }
      return current.filter(item => grantKey(item) !== grantKey(grant));
    });
  };

  const removeGrant = (grant: RuntimeAudienceGrant) => {
    setDraft(current => current.filter(item => grantKey(item) !== grantKey(grant)));
  };

  const handleConfirm = () => {
    onConfirm(dedupeRuntimeAudienceGrants(draft));
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="p-0 text-left">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>

        <DialogBody className="space-y-4">
          <div className="grid min-h-[420px] gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
            <div className="min-w-0 rounded-md border border-border/80 bg-background p-3">
              <Tabs value={activeTab} onValueChange={setActiveTab} className="h-full">
                <TabsList className="grid w-full grid-cols-3">
                  <TabsTrigger value="departments">{t('picker.departmentsTab')}</TabsTrigger>
                  <TabsTrigger value="workspaces">{t('picker.workspacesTab')}</TabsTrigger>
                  <TabsTrigger value="members">{t('picker.membersTab')}</TabsTrigger>
                </TabsList>

                <TabsContent value="departments" className="mt-3">
                  <ScrollArea className="h-[350px] rounded-md border border-border/70">
                    <div className="p-2">
                      {isDepartmentsLoadingState ? (
                        <div className="px-2 py-8 text-center text-sm text-muted-foreground">
                          {t('picker.loadingDepartments')}
                        </div>
                      ) : departmentRows.length === 0 ? (
                        <div className="px-2 py-8 text-center text-sm text-muted-foreground">
                          {t('picker.noDepartments')}
                        </div>
                      ) : (
                        <div className="space-y-1">
                          {departmentRows.map(row => {
                            const grant: RuntimeAudienceGrant = {
                              subject_type: 'department',
                              subject_id: row.department.id,
                            };
                            const key = grantKey(grant);
                            const checked = selectedKeys.has(key);
                            return (
                              <div
                                key={row.department.id}
                                role="button"
                                tabIndex={0}
                                className={cn(
                                  'flex min-h-10 items-center gap-2 rounded-md px-2 py-2 text-left text-sm outline-none transition-colors hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring',
                                  checked && 'bg-primary/5 text-primary'
                                )}
                                style={{ paddingLeft: 8 + row.depth * 16 }}
                                onClick={() => toggleGrant(grant, !checked)}
                                onKeyDown={event => {
                                  if (event.key === 'Enter' || event.key === ' ') {
                                    event.preventDefault();
                                    toggleGrant(grant, !checked);
                                  }
                                }}
                              >
                                <Checkbox
                                  checked={checked}
                                  onClick={event => event.stopPropagation()}
                                  onCheckedChange={nextChecked =>
                                    toggleGrant(grant, nextChecked === true)
                                  }
                                />
                                <Building2 className="h-4 w-4 shrink-0 text-muted-foreground" />
                                <span className="min-w-0 flex-1 truncate">
                                  {row.department.name}
                                </span>
                                <span className="shrink-0 text-xs text-muted-foreground">
                                  {row.department.member_count}
                                </span>
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  </ScrollArea>
                </TabsContent>

                <TabsContent value="workspaces" className="mt-3">
                  <ScrollArea className="h-[350px] rounded-md border border-border/70">
                    <div className="p-2">
                      {isWorkspacesLoadingState ? (
                        <div className="px-2 py-8 text-center text-sm text-muted-foreground">
                          {t('picker.loadingWorkspaces')}
                        </div>
                      ) : visibleWorkspaces.length === 0 ? (
                        <div className="px-2 py-8 text-center text-sm text-muted-foreground">
                          {t('picker.noWorkspaces')}
                        </div>
                      ) : (
                        <div className="space-y-1">
                          {visibleWorkspaces.map(workspace => {
                            const grant: RuntimeAudienceGrant = {
                              subject_type: 'workspace',
                              subject_id: workspace.id,
                            };
                            const key = grantKey(grant);
                            const checked = selectedKeys.has(key);
                            return (
                              <div
                                key={workspace.id}
                                role="button"
                                tabIndex={0}
                                className={cn(
                                  'flex min-h-10 items-center gap-2 rounded-md px-2 py-2 text-left text-sm outline-none transition-colors hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring',
                                  checked && 'bg-primary/5 text-primary'
                                )}
                                onClick={() => toggleGrant(grant, !checked)}
                                onKeyDown={event => {
                                  if (event.key === 'Enter' || event.key === ' ') {
                                    event.preventDefault();
                                    toggleGrant(grant, !checked);
                                  }
                                }}
                              >
                                <Checkbox
                                  checked={checked}
                                  onClick={event => event.stopPropagation()}
                                  onCheckedChange={nextChecked =>
                                    toggleGrant(grant, nextChecked === true)
                                  }
                                />
                                <Boxes className="h-4 w-4 shrink-0 text-muted-foreground" />
                                <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
                                {typeof workspace.member_count === 'number' ? (
                                  <span className="shrink-0 text-xs text-muted-foreground">
                                    {workspace.member_count}
                                  </span>
                                ) : null}
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  </ScrollArea>
                </TabsContent>

                <TabsContent value="members" className="mt-3">
                  <div className="space-y-3">
                    <Input
                      value={memberKeyword}
                      onChange={event => setMemberKeyword(event.target.value)}
                      placeholder={t('picker.searchMembersPlaceholder')}
                      leftIcon={<Search />}
                      className="h-9 rounded-md"
                    />
                    <ScrollArea className="h-[300px] rounded-md border border-border/70">
                      <div className="p-2">
                        {isMembersLoadingState ? (
                          <div className="px-2 py-8 text-center text-sm text-muted-foreground">
                            {t('picker.loadingMembers')}
                          </div>
                        ) : members.length === 0 ? (
                          <div className="px-2 py-8 text-center text-sm text-muted-foreground">
                            {t('picker.noMembers')}
                          </div>
                        ) : (
                          <div className="space-y-1">
                            {members.map(member => {
                              const grant: RuntimeAudienceGrant = {
                                subject_type: 'account',
                                subject_id: member.id,
                              };
                              const key = grantKey(grant);
                              const checked = selectedKeys.has(key);
                              const label = getRuntimeAudienceMemberLabel(member);
                              return (
                                <div
                                  key={member.id}
                                  role="button"
                                  tabIndex={0}
                                  className={cn(
                                    'flex min-h-11 items-center gap-2 rounded-md px-2 py-2 text-left text-sm outline-none transition-colors hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring',
                                    checked && 'bg-primary/5 text-primary'
                                  )}
                                  onClick={() => toggleGrant(grant, !checked)}
                                  onKeyDown={event => {
                                    if (event.key === 'Enter' || event.key === ' ') {
                                      event.preventDefault();
                                      toggleGrant(grant, !checked);
                                    }
                                  }}
                                >
                                  <Checkbox
                                    checked={checked}
                                    onClick={event => event.stopPropagation()}
                                    onCheckedChange={nextChecked =>
                                      toggleGrant(grant, nextChecked === true)
                                    }
                                  />
                                  <SafeAvatar
                                    size="sm"
                                    src={member.avatar_url || member.avatar}
                                    alt={label}
                                    fallback={label}
                                  />
                                  <span className="min-w-0 flex-1">
                                    <span className="block truncate">{label}</span>
                                    {member.email ? (
                                      <span className="block truncate text-xs text-muted-foreground">
                                        {member.email}
                                      </span>
                                    ) : null}
                                  </span>
                                </div>
                              );
                            })}
                          </div>
                        )}
                      </div>
                    </ScrollArea>
                  </div>
                </TabsContent>
              </Tabs>
            </div>

            <div className="min-w-0 rounded-md border border-border/80 bg-muted/20 p-3">
              <div className="mb-3 flex items-center justify-between gap-2">
                <div className="min-w-0">
                  <div className="text-sm font-semibold text-foreground">
                    {t('picker.selectedTitle')}
                  </div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {t('picker.selectedCount', {
                      departments: selectedDepartments,
                      workspaces: selectedWorkspaces,
                      accounts: selectedAccounts,
                    })}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="xs"
                  className="rounded-md"
                  disabled={disabled || draft.length === 0}
                  onClick={() => setDraft([])}
                >
                  {t('actions.clear')}
                </Button>
              </div>
              <RuntimeAudienceChipList
                value={draft}
                disabled={disabled}
                emptyText={t('picker.emptySelected')}
                lookupEnabled={open}
                onRemove={removeGrant}
              />
            </div>
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={disabled}>
            {t('actions.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface RuntimeAudienceChipListProps {
  value: RuntimeAudienceGrant[];
  disabled?: boolean;
  emptyText: string;
  className?: string;
  lookupEnabled?: boolean;
  onRemove?: (grant: RuntimeAudienceGrant) => void;
}

export function RuntimeAudienceChipList({
  value,
  disabled = false,
  emptyText,
  className,
  lookupEnabled = true,
  onRemove,
}: RuntimeAudienceChipListProps) {
  const normalized = useMemo(() => dedupeRuntimeAudienceGrants(value), [value]);
  const shouldLookupDepartments =
    lookupEnabled && normalized.some(grant => grant.subject_type === 'department');
  const shouldLookupWorkspaces =
    lookupEnabled && normalized.some(grant => grant.subject_type === 'workspace');
  const { departments, isLoading, isFetching, error } = useDepartments({
    enabled: shouldLookupDepartments,
  });
  const {
    workspaces,
    isLoading: isWorkspacesLoading,
    isFetching: isWorkspacesFetching,
    error: workspacesError,
  } = useWorkspaces('', 1, 1000, {
    keepPreviousData: true,
    enabled: shouldLookupWorkspaces,
  });
  const departmentById = useMemo(() => buildDepartmentMap(departments), [departments]);
  const workspaceById = useMemo(() => buildWorkspaceMap(workspaces), [workspaces]);
  const isDepartmentLookupLoading = isLoading || isFetching;
  const isWorkspaceLookupLoading = isWorkspacesLoading || isWorkspacesFetching;

  if (normalized.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-border/80 px-3 py-6 text-center text-sm text-muted-foreground">
        {emptyText}
      </div>
    );
  }

  return (
    <div className={cn('flex flex-wrap gap-2', className)}>
      {normalized.map(grant => (
        <RuntimeAudienceChip
          key={grantKey(grant)}
          grant={grant}
          disabled={disabled}
          departmentById={departmentById}
          workspaceById={workspaceById}
          isDepartmentLookupLoading={isDepartmentLookupLoading}
          isWorkspaceLookupLoading={isWorkspaceLookupLoading}
          departmentLookupError={error}
          workspaceLookupError={workspacesError}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}

function RuntimeAudienceChip({
  grant,
  disabled,
  departmentById,
  workspaceById,
  isDepartmentLookupLoading,
  isWorkspaceLookupLoading,
  departmentLookupError,
  workspaceLookupError,
  onRemove,
}: {
  grant: RuntimeAudienceGrant;
  disabled: boolean;
  departmentById: Map<string, Department>;
  workspaceById: Map<string, WorkspaceManagement>;
  isDepartmentLookupLoading: boolean;
  isWorkspaceLookupLoading: boolean;
  departmentLookupError: string | null;
  workspaceLookupError: string | null;
  onRemove?: (grant: RuntimeAudienceGrant) => void;
}) {
  const t = useT('agents.runtimeAccess');
  const normalizedSubjectId = grant.subject_id.trim();
  const {
    member,
    isLoading: isAccountLoading,
    isFetching: isAccountFetching,
    error: accountLookupError,
  } = useCurrentOrganizationMember(normalizedSubjectId, {
    enabled: grant.subject_type === 'account' && Boolean(normalizedSubjectId),
  });
  const accountLoading = isAccountLoading || isAccountFetching;
  const department =
    grant.subject_type === 'department' ? departmentById.get(normalizedSubjectId) : null;
  const workspace =
    grant.subject_type === 'workspace' ? workspaceById.get(normalizedSubjectId) : null;
  const missingSelection = grant.subject_type !== 'organization' && !normalizedSubjectId;
  const accountLookupFailed =
    grant.subject_type === 'account' &&
    Boolean(normalizedSubjectId) &&
    Boolean(accountLookupError) &&
    !member &&
    !accountLoading;
  const accountUnresolved =
    grant.subject_type === 'account' &&
    Boolean(normalizedSubjectId) &&
    !accountLookupError &&
    !member &&
    !accountLoading;
  const departmentLookupFailed =
    grant.subject_type === 'department' &&
    Boolean(normalizedSubjectId) &&
    !isDepartmentLookupLoading &&
    Boolean(departmentLookupError);
  const departmentUnresolved =
    grant.subject_type === 'department' &&
    Boolean(normalizedSubjectId) &&
    !isDepartmentLookupLoading &&
    !departmentLookupError &&
    !department;
  const workspaceLookupFailed =
    grant.subject_type === 'workspace' &&
    Boolean(normalizedSubjectId) &&
    !isWorkspaceLookupLoading &&
    Boolean(workspaceLookupError);
  const workspaceUnresolved =
    grant.subject_type === 'workspace' &&
    Boolean(normalizedSubjectId) &&
    !isWorkspaceLookupLoading &&
    !workspaceLookupError &&
    !workspace;
  const needsAttention =
    missingSelection ||
    accountLookupFailed ||
    accountUnresolved ||
    departmentLookupFailed ||
    departmentUnresolved ||
    workspaceLookupFailed ||
    workspaceUnresolved;
  const icon =
    grant.subject_type === 'organization' ? (
      <Users className="h-4 w-4" />
    ) : grant.subject_type === 'department' ? (
      <Building2 className="h-4 w-4" />
    ) : grant.subject_type === 'workspace' ? (
      <Boxes className="h-4 w-4" />
    ) : (
      <UserRound className="h-4 w-4" />
    );
  const label =
    grant.subject_type === 'organization'
      ? t('grants.organizationWide')
      : grant.subject_type === 'department'
        ? departmentLookupFailed
          ? `${t('grants.departmentLookupFailed')}: ${normalizedSubjectId}`
          : departmentUnresolved
            ? `${t('grants.unresolvedDepartment')}: ${normalizedSubjectId}`
            : department?.name || normalizedSubjectId || t('grants.departmentPlaceholder')
        : grant.subject_type === 'workspace'
          ? workspaceLookupFailed
            ? `${t('grants.workspaceLookupFailed')}: ${normalizedSubjectId}`
            : workspaceUnresolved
              ? `${t('grants.unresolvedWorkspace')}: ${normalizedSubjectId}`
              : workspace?.name || normalizedSubjectId || t('grants.workspacePlaceholder')
          : accountLoading && !member
            ? t('grants.resolvingAccount')
            : accountLookupFailed
              ? `${t('grants.accountLookupFailed')}: ${normalizedSubjectId}`
              : accountUnresolved
                ? `${t('grants.unresolvedAccount')}: ${normalizedSubjectId}`
                : member
                  ? getRuntimeAudienceMemberLabel(member)
                  : normalizedSubjectId || t('grants.accountPlaceholder');

  return (
    <Badge
      variant={
        needsAttention ? 'warning' : grant.subject_type === 'organization' ? 'success' : 'outline'
      }
      className={cn(
        'max-w-full shrink justify-start gap-1.5 overflow-hidden rounded-md px-2 py-1 text-left',
        needsAttention && 'border-warning/30'
      )}
    >
      <span className="inline-flex h-4 w-4 shrink-0 items-center justify-center">
        {needsAttention ? <AlertCircle className="h-4 w-4" /> : icon}
      </span>
      <span className="min-w-0 max-w-[220px] truncate">{label}</span>
      {onRemove ? (
        <button
          type="button"
          disabled={disabled}
          className="ml-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-background/80 hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
          aria-label={t('actions.removeGrant')}
          onClick={() => onRemove(grant)}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      ) : null}
    </Badge>
  );
}
