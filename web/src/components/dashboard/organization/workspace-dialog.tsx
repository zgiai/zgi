'use client';

import { useState, useCallback, useEffect, useMemo } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { User } from 'lucide-react';
import { cn } from '@/lib/utils';
// import { User, Info, ChevronsUpDown } from 'lucide-react';

// import { DepartmentTreeItemDropdown } from '@/components/dashboard/organization/department-tree-item-dropdown';
// import { cn } from '@/lib/utils';
// import { useDepartments } from '@/hooks/enterprise/use-departments';
import { useDepartmentMembers } from '@/hooks/organization/use-department-members';
// import { useApiKeys } from '@/hooks/apikey/use-apikey';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useWorkspaceDetail } from '@/hooks/workspace/use-workspace-detail';
import type { CreateWorkspaceRequest, UpdateWorkspaceRequest } from '@/services/types/workspace';
import type { WorkspaceManagement } from '@/services/types/workspace';

interface WorkspaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreateWorkspaceRequest) => Promise<void>;
  onUpdate?: (data: UpdateWorkspaceRequest) => Promise<void>;
  initialData?: WorkspaceManagement | null;
  isLoading?: boolean;
}

export function WorkspaceDialog({
  open,
  onOpenChange,
  onCreate,
  onUpdate,
  initialData,
  isLoading = false,
}: WorkspaceDialogProps) {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();

  const isEditMode = !!initialData;
  const workspaceId = open && isEditMode ? initialData?.id ?? '' : '';
  const { workspaceDetail } = useWorkspaceDetail(workspaceId);
  const resolvedInitialData = workspaceDetail ?? initialData;

  // Form state
  const [workspaceName, setWorkspaceName] = useState(resolvedInitialData?.name || '');
  const [nameError, setNameError] = useState('');
  const [departmentId, setDepartmentId] = useState<string>(resolvedInitialData?.department_id || '');
  const [leaderId, setLeaderId] = useState<string>(resolvedInitialData?.leader_id || '');
  const [leaderError, setLeaderError] = useState('');
  const [apiKeyId, setApiKeyId] = useState<string>(resolvedInitialData?.api_key_id || '');
  // const [departmentSelectOpen, setDepartmentSelectOpen] = useState(false);
  // const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  // Initialize form when initialData changes or dialog opens
  useEffect(() => {
    if (open) {
      if (resolvedInitialData) {
        setWorkspaceName(resolvedInitialData.name || '');
        setDepartmentId(resolvedInitialData.department_id || '');
        setApiKeyId(resolvedInitialData.api_key_id || '');
        setLeaderId(resolvedInitialData.leader_id || '');
        // Clear errors
        setNameError('');
        setLeaderError('');
      } else {
        // Reset form when opening in create mode
        setWorkspaceName('');
        setDepartmentId('');
        setLeaderId('');
        setApiKeyId('');
        // Clear errors
        setNameError('');
        setLeaderError('');
      }
    }
  }, [open, resolvedInitialData]);

  // Fetch data
  // const { departments, isLoading: isLoadingDepartments } = useDepartments();
  const { members: departmentMembers, isLoading: isLoadingMembers } = useDepartmentMembers({
    deptId: departmentId || null,
    includeSubDepts: true,
    limit: 100,
    page: 1,
    enabled: open,
  });
  // const { items: apiKeys, isLoading: isLoadingApiKeys } = useApiKeys(undefined);

  // Filter members to only include active ones
  const activeDepartmentMembers = useMemo(() => {
    if (!departmentMembers) return [];
    return departmentMembers.filter(member => member.group_status === 'active');
  }, [departmentMembers]);

  // Search state for leader dropdown
  const [leaderSearch, setLeaderSearch] = useState('');

  // Filter members by search keyword
  const filteredMembers = useMemo(() => {
    if (!leaderSearch.trim()) return activeDepartmentMembers;
    const keyword = leaderSearch.toLowerCase();
    return activeDepartmentMembers.filter(
      member =>
        member.account_name?.toLowerCase().includes(keyword) ||
        member.account_email?.toLowerCase().includes(keyword)
    );
  }, [activeDepartmentMembers, leaderSearch]);

  // Find leader_id from leader_name when in edit mode
  useEffect(() => {
    if (!isEditMode || leaderId || !resolvedInitialData?.leader_name || !activeDepartmentMembers) {
      return;
    }

    const leader =
      activeDepartmentMembers.find(member => member.account_id === resolvedInitialData.leader_id) ||
      activeDepartmentMembers.find(member => member.account_name === resolvedInitialData.leader_name);

    if (leader) {
      setLeaderId(leader.account_id);
    }
  }, [
    isEditMode,
    leaderId,
    resolvedInitialData?.leader_id,
    resolvedInitialData?.leader_name,
    activeDepartmentMembers,
  ]);

  // // Get selected department name for display
  // const getDepartmentName = (id: string): string | null => {
  //   const findDept = (depts: Department[], targetId: string): string | null => {
  //     for (const dept of depts) {
  //       if (dept.id === targetId) return dept.name;
  //       if (dept.children) {
  //         const found = findDept(dept.children, targetId);
  //         if (found) return found;
  //       }
  //     }
  //     return null;
  //   };
  //   return findDept(departments, id);
  // };

  // // Expand first level by default when select opens
  // const handleDepartmentSelectOpen = (open: boolean) => {
  //   setDepartmentSelectOpen(open);
  //   if (open && expandedIds.size === 0) {
  //     setExpandedIds(new Set(departments.map(dept => dept.id)));
  //   }
  // };

  // const handleToggleExpand = (id: string) => {
  //   setExpandedIds(prev => {
  //     const next = new Set(prev);
  //     if (next.has(id)) {
  //       next.delete(id);
  //     } else {
  //       next.add(id);
  //     }
  //     return next;
  //   });
  // };

  // const handleDepartmentSelect = (id: string) => {
  //   setDepartmentId(id);
  //   setDepartmentSelectOpen(false);
  // };

  // Reset form when dialog closes
  const handleOpenChange = useCallback(
    (newOpen: boolean) => {
      if (!newOpen) {
        if (!initialData) {
          setWorkspaceName('');
          setDepartmentId('');
          setLeaderId('');
          setApiKeyId('');
        }
        // setDepartmentSelectOpen(false);
        // setExpandedIds(new Set());
      }
      onOpenChange(newOpen);
    },
    [onOpenChange, initialData]
  );

  // Handle submit
  const handleSubmit = useCallback(
    async (e?: React.FormEvent) => {
      e?.preventDefault();
      const trimmedName = workspaceName.trim();
      if (!currentOrganization?.id) return;

      if (!trimmedName) {
        setNameError(t('organization.validation.nameRequired'));
        return;
      }

      if (!leaderId) {
        setLeaderError(t('organization.validation.leaderRequired'));
        return;
      }

      if (trimmedName.length > 30) {
        setNameError(t('organization.validation.nameTooLong', { max: 30 }));
        return;
      }

      const name = trimmedName;

      try {
        if (isEditMode && onUpdate) {
          // Find original leader_id from leader_name
          const originalLeaderId =
            resolvedInitialData?.leader_id ||
            activeDepartmentMembers.find(m => m.account_name === resolvedInitialData?.leader_name)
              ?.account_id ||
            '';

          // Check if any values have changed
          const hasNameChanged = name !== (resolvedInitialData?.name || '');
          const hasDepartmentChanged = departmentId !== (resolvedInitialData?.department_id || '');
          const hasLeaderChanged = leaderId !== originalLeaderId;
          const hasApiKeyChanged = apiKeyId !== (resolvedInitialData?.api_key_id || '');

          const hasAnyChange =
            hasNameChanged || hasDepartmentChanged || hasLeaderChanged || hasApiKeyChanged;

          // Skip API call if nothing changed
          if (!hasAnyChange) {
            onOpenChange(false);
            return;
          }

          const payload: UpdateWorkspaceRequest = {
            name,
            department_id: departmentId || '',
            leader_id: leaderId || '',
            api_key_id: apiKeyId || '',
          };
          await onUpdate(payload);
        } else {
          const payload: CreateWorkspaceRequest = {
            name,
            ...(departmentId ? { department_id: departmentId } : {}),
            ...(leaderId ? { leader_id: leaderId } : {}),
            ...(apiKeyId ? { api_key_id: apiKeyId } : {}),
          };
          await onCreate(payload);
        }
        onOpenChange(false);
      } catch (error) {
        console.error('Failed to submit workspace:', error);
      }
    },
    [
      workspaceName,
      departmentId,
      leaderId,
      apiKeyId,
      currentOrganization?.id,
      isEditMode,
      resolvedInitialData,
      activeDepartmentMembers,
      onCreate,
      onUpdate,
      onOpenChange,
      t,
    ]
  );

  const canSubmit = workspaceName.trim().length > 0 && !!leaderId && !isLoading;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-[560px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {isEditMode
              ? t('organization.workspaceManagement.createWorkspace.editTitle')
              : t('organization.workspaceManagement.createWorkspace.title')}
          </DialogTitle>
          <DialogDescription className="text-sm text-muted-foreground font-medium">
            {isEditMode
              ? t('organization.workspaceManagement.createWorkspace.editDescription')
              : t('organization.workspaceManagement.createWorkspace.description')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6 transition-all duration-300">
          <form id="workspace-form" onSubmit={handleSubmit} className="space-y-6">
            {/* Workspace Name */}
            <div className="space-y-2.5">
              <Label htmlFor="workspace-name" className="text-sm font-semibold">
                {t('organization.workspaceManagement.createWorkspace.workspaceName')}
              </Label>
              <Input
                id="workspace-name"
                className="h-11 shadow-sm"
                placeholder={t(
                  'organization.workspaceManagement.createWorkspace.workspaceNamePlaceholder'
                )}
                value={workspaceName}
                onChange={e => {
                  setWorkspaceName(e.target.value);
                  if (nameError) setNameError('');
                }}
                maxLength={30}
                errorText={nameError}
              />
            </div>

            {/* Leader */}
            <div className="space-y-2.5">
              <Label htmlFor="leader" className="text-sm font-semibold flex items-center gap-1">
                {t('organization.workspaceManagement.createWorkspace.leader')}
                <span className="text-destructive">*</span>
              </Label>
              <Select
                value={leaderId}
                onValueChange={value => {
                  setLeaderId(value);
                  if (leaderError) setLeaderError('');
                }}
                disabled={isLoadingMembers}
              >
                <SelectTrigger
                  id="leader"
                  className={cn('h-16 shadow-sm', leaderError && 'border-destructive')}
                >
                  <SelectValue
                    placeholder={t(
                      'organization.workspaceManagement.createWorkspace.leaderPlaceholder'
                    )}
                  />
                </SelectTrigger>
                <SelectContent>
                  <div className="p-2 border-b border-border/40">
                    <Input
                      placeholder={t(
                        'organization.workspaceManagement.createWorkspace.leaderSearchPlaceholder'
                      )}
                      value={leaderSearch}
                      onChange={e => setLeaderSearch(e.target.value)}
                      className="h-9"
                      onKeyDown={e => e.stopPropagation()}
                    />
                  </div>
                  <div className="max-h-[200px] overflow-y-auto p-1">
                    {filteredMembers && filteredMembers.length > 0 ? (
                      filteredMembers.map(member => (
                        <SelectItem
                          key={member.account_id}
                          value={member.account_id}
                          className="rounded-md"
                        >
                          <div className="flex items-center gap-3 py-1">
                            <div className="size-8 rounded-full bg-muted flex items-center justify-center shrink-0">
                              <User className="h-4 w-4 text-muted-foreground" />
                            </div>
                            <div className="flex flex-col text-start">
                              <span className="font-semibold text-sm leading-none mb-1">
                                {member.account_name}
                              </span>
                              <span className="text-[10px] text-muted-foreground tracking-tight">
                                {member.account_email}
                              </span>
                            </div>
                          </div>
                        </SelectItem>
                      ))
                    ) : (
                      <div className="px-3 py-6 text-center text-muted-foreground text-sm">
                        {t('organization.workspaceManagement.createWorkspace.noMembersFound')}
                      </div>
                    )}
                  </div>
                </SelectContent>
              </Select>
              {leaderError && (
                <p className="text-xs text-destructive font-medium ml-1">{leaderError}</p>
              )}
            </div>
          </form>
        </DialogBody>

        <DialogFooter className="bg-muted/50 px-6 pb-6 pt-4 border-t gap-3">
          <Button
            variant="ghost"
            size="xl"
            onClick={() => handleOpenChange(false)}
            disabled={isLoading}
            className="px-6 font-semibold"
          >
            {t('organization.workspaceManagement.createWorkspace.cancel')}
          </Button>
          <Button
            form="workspace-form"
            onClick={handleSubmit}
            disabled={!canSubmit}
            size="xl"
            className="px-8 font-semibold"
          >
            {isLoading
              ? isEditMode
                ? t('organization.workspaceManagement.createWorkspace.updating')
                : t('organization.workspaceManagement.createWorkspace.creating')
              : isEditMode
                ? t('organization.workspaceManagement.createWorkspace.update')
                : t('organization.workspaceManagement.createWorkspace.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
