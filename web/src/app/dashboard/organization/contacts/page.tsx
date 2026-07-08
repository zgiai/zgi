'use client';

import { useState, useMemo, useEffect } from 'react';
import { useT } from '@/i18n';
import {
  Search,
  Plus,
  Users,
  Pencil,
  Trash2,
  KeyRound,
  UserPlus,
  MoreHorizontal,
  Power,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useDepartments } from '@/hooks/organization/use-departments';
import { useDepartmentMembers } from '@/hooks/organization/use-department-members';
import { DepartmentTreeItem } from '@/components/dashboard/organization/department-tree-item';
import { AddMemberDialog } from '@/components/dashboard/organization/add-member-dialog';
import { EditMemberDialog } from '@/components/dashboard/organization/edit-member-dialog';
import { ResetMemberPasswordDialog } from '@/components/dashboard/organization/reset-member-password-dialog';
import { CreateDepartmentDialog } from '@/components/dashboard/organization/create-department-dialog';
import { EditDepartmentDialog } from '@/components/dashboard/organization/edit-department-dialog';
import { AssignWorkspaceDialog } from '@/components/dashboard/organization/assign-workspace-dialog';
import { IS_CLOUD } from '@/lib/config';
import { StickyDataTable } from '@/components/common/sticky-data-table';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useMemberActions } from '@/hooks/organization/use-member-actions';
import { useDeleteDepartment } from '@/hooks/organization/use-department-actions';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { toast } from 'sonner';
import type { Department, DepartmentMember, JoinedWorkspace } from '@/services/types/organization';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Pagination } from '@/components/ui/pagination';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Input } from '@/components/ui/input';
import { useAuthStore } from '@/store/auth-store';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { normalizeOrganizationRole } from '@/utils/role-labels';
import { getOrganizationDisplayName } from '@/utils/organization-display';

export default function ContactsPage() {
  const t = useT('dashboard.organization.contacts');
  const tRoot = useT();
  const currentUser = useAuthStore.use.user();
  const [searchKeyword, setSearchKeyword] = useState('');
  const [memberSearchKeyword, setMemberSearchKeyword] = useState('');
  const [selectedDeptId, setSelectedDeptId] = useState<string | null>(null);
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize] = useState(20);
  const [addMemberDialogOpen, setAddMemberDialogOpen] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [memberToDelete, setMemberToDelete] = useState<DepartmentMember | null>(null);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [memberToEdit, setMemberToEdit] = useState<DepartmentMember | null>(null);
  const [editDepartmentDialogOpen, setEditDepartmentDialogOpen] = useState(false);
  const [departmentToEdit, setDepartmentToEdit] = useState<Department | null>(null);
  const [deleteDepartmentConfirmOpen, setDeleteDepartmentConfirmOpen] = useState(false);
  const [departmentToDelete, setDepartmentToDelete] = useState<Department | null>(null);
  const [createDepartmentDialogOpen, setCreateDepartmentDialogOpen] = useState(false);
  const [parentDepartmentForCreate, setParentDepartmentForCreate] = useState<Department | null>(
    null
  );
  const [statusConfirmOpen, setStatusConfirmOpen] = useState(false);
  const [memberToToggle, setMemberToToggle] = useState<DepartmentMember | null>(null);
  const [resetPasswordDialogOpen, setResetPasswordDialogOpen] = useState(false);
  const [memberToResetPassword, setMemberToResetPassword] = useState<DepartmentMember | null>(null);
  const [assignWorkspaceDialogOpen, setAssignWorkspaceDialogOpen] = useState(false);
  const [memberToAssignWorkspace, setMemberToAssignWorkspace] =
    useState<DepartmentMember | null>(null);

  // Member actions hook
  const {
    updateMemberStatus,
    isUpdatingStatus,
    removeMember,
    isRemoving,
    removeMemberFromOrganization,
    isRemovingFromOrg,
    updateMemberDepartment,
    isUpdatingDepartment,
    updateMemberNickname,
    isUpdatingNickname,
    resetMemberPassword,
    isResettingPassword,
  } = useMemberActions();

  // Department actions hooks
  const { deleteDepartment, isDeleting } = useDeleteDepartment();

  // Handle department actions
  const handleAddSubDepartment = (department: Department) => {
    setParentDepartmentForCreate(department);
    setCreateDepartmentDialogOpen(true);
  };

  const handleEditDepartment = (department: Department) => {
    setDepartmentToEdit(department);
    setEditDepartmentDialogOpen(true);
  };

  const handleDeleteDepartment = (department: Department) => {
    setDepartmentToDelete(department);
    setDeleteDepartmentConfirmOpen(true);
  };

  const handleConfirmDeleteDepartment = async () => {
    if (!departmentToDelete) return;

    // Check if department has members
    if (departmentToDelete.member_count > 0) {
      toast.error(
        t('deleteDepartment.hasMembersDescription', {
          departmentName: departmentToDelete.name,
          memberCount: departmentToDelete.member_count,
        })
      );
      setDeleteDepartmentConfirmOpen(false);
      setDepartmentToDelete(null);
      return;
    }

    try {
      await deleteDepartment(departmentToDelete.id);
      setDeleteDepartmentConfirmOpen(false);
      setDepartmentToDelete(null);
      // Clear selection if deleted department was selected
      if (selectedDeptId === departmentToDelete.id) {
        setSelectedDeptId(null);
      }
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to delete department:', error);
    }
  };

  // Debounce member search keyword to avoid frequent API calls (500ms delay)
  const debouncedMemberSearchKeyword = useDebouncedValue(memberSearchKeyword, 500);

  // Get current organization
  const { currentOrganization } = useOrganizations();
  const currentOrganizationRole =
    currentOrganization?.organization_role ?? currentUser?.organization_role ?? null;

  // Fetch departments
  const { departments, isLoading: loadingDepartments } = useDepartments();
  const currentOrganizationDisplayName = useMemo(
    () => getOrganizationDisplayName(currentOrganization),
    [currentOrganization]
  );

  // Determine if organization root is selected
  const isOrgRootSelected = selectedDeptId === 'ORG_ROOT';

  // Fetch members based on selection
  const {
    members,
    total,
    isLoading: loadingMembers,
    refetch: refetchMembers,
  } = useDepartmentMembers({
    deptId: isOrgRootSelected ? null : selectedDeptId,
    keyword: debouncedMemberSearchKeyword,
    page: currentPage,
    limit: pageSize,
    enabled: !!selectedDeptId,
  });

  // Calculate total pages
  const totalPages = Math.ceil(total / pageSize);

  useEffect(() => {
    if (!selectedDeptId && currentOrganization && !loadingDepartments) {
      // Select organization root by default
      setSelectedDeptId('ORG_ROOT');
    }
  }, [currentOrganization, loadingDepartments, selectedDeptId]);

  // Reset to page 1 when department or search keyword changes
  useEffect(() => {
    setCurrentPage(1);
  }, [selectedDeptId, debouncedMemberSearchKeyword]);

  // Handle case where current page > total pages (e.g. after deletion)
  useEffect(() => {
    const maxPage = Math.max(1, totalPages);
    if (currentPage > maxPage) {
      setCurrentPage(maxPage);
    }
  }, [currentPage, totalPages]);

  // Hide skeleton when departments are loaded (first time only)
  useEffect(() => {
    if (isInitialLoad && !loadingDepartments) {
      setIsInitialLoad(false);
    }
  }, [isInitialLoad, loadingDepartments]);

  // Create virtual organization root node
  const organizationNode: Department | null = useMemo(() => {
    if (!currentOrganization) return null;

    return {
      id: 'ORG_ROOT',
      organization_id: currentOrganization.id,
      parent_id: null,
      name: currentOrganizationDisplayName,
      sort_order: 0,
      status: 'active' as const,
      member_count: departments.reduce((sum, dept) => sum + dept.member_count, 0),
      children: departments,
    };
  }, [currentOrganization, currentOrganizationDisplayName, departments]);

  // Filter departments based on search
  const filteredDepartments = useMemo(() => {
    if (!organizationNode) return [];

    if (!searchKeyword) return [organizationNode];

    const filterDepts = (depts: Department[]): Department[] => {
      return depts
        .map(dept => {
          const matchesSearch = dept.name.toLowerCase().includes(searchKeyword.toLowerCase());
          const filteredChildren = dept.children ? filterDepts(dept.children) : [];

          if (matchesSearch || filteredChildren.length > 0) {
            return {
              ...dept,
              children: filteredChildren,
            };
          }
          return null;
        })
        .filter(Boolean) as Department[];
    };

    // Filter the organization's children (departments)
    const filteredChildren = filterDepts(organizationNode.children || []);

    // Always return the organization node with filtered children
    return [
      {
        ...organizationNode,
        children: filteredChildren,
      },
    ];
  }, [organizationNode, searchKeyword]);

  // Create unified departments tree for dialogs (always includes organization root)
  const departmentsTree = useMemo(() => {
    return organizationNode ? [organizationNode] : departments;
  }, [organizationNode, departments]);

  // Get selected department name
  const selectedDeptName = useMemo(() => {
    if (!selectedDeptId) return null;

    // Check if organization root is selected
    if (selectedDeptId === 'ORG_ROOT' && currentOrganization) {
      return currentOrganizationDisplayName;
    }

    const findDept = (depts: Department[]): string | null => {
      for (const dept of depts) {
        if (dept.id === selectedDeptId) return dept.name;
        if (dept.children) {
          const found = findDept(dept.children);
          if (found) return found;
        }
      }
      return null;
    };

    return findDept(departments);
  }, [selectedDeptId, departments, currentOrganization, currentOrganizationDisplayName]);

  const selectedScopeDescription = selectedDeptId
    ? isOrgRootSelected
      ? t('allMembersDescription')
      : t('departmentMembersDescription')
    : t('selectDepartment');
  const selectedScopeLabel = selectedDeptId
    ? isOrgRootSelected
      ? t('scopeOrganization')
      : t('scopeDepartment')
    : null;
  const selectedScopeHint = selectedDeptId
    ? isOrgRootSelected
      ? t('scopeOrganizationHint')
      : t('scopeDepartmentHint')
    : null;
  const hasMemberSearch = debouncedMemberSearchKeyword.trim().length > 0;

  const getOrganizationRoleLabel = (role?: DepartmentMember['organization_role']) => {
    const normalizedRole = normalizeOrganizationRole(role);
    if (!normalizedRole) return '-';
    return t(
      `organizationRoles.${normalizedRole}` as
        | 'organizationRoles.owner'
        | 'organizationRoles.admin'
        | 'organizationRoles.normal'
    );
  };

  const canResetMemberPassword = (member: DepartmentMember) => {
    if (IS_CLOUD || !currentUser?.id || !member.organization_role) {
      return false;
    }
    if (member.account_id === currentUser.id) {
      return false;
    }
    if (currentOrganizationRole === 'owner') {
      return member.organization_role !== 'owner';
    }
    if (currentOrganizationRole === 'admin') {
      return member.organization_role === 'normal';
    }
    return false;
  };

  // Handle toggle member status
  const handleToggleStatus = (member: DepartmentMember) => {
    setMemberToToggle(member);
    setStatusConfirmOpen(true);
  };

  const handleConfirmToggleStatus = async () => {
    if (!memberToToggle) return;
    const currentStatus = memberToToggle.group_status || 'active';
    const newStatus = currentStatus === 'active' ? 'inactive' : 'active';
    await updateMemberStatus({ memberId: memberToToggle.account_id, status: newStatus });
    setStatusConfirmOpen(false);
    setMemberToToggle(null);
  };

  // Handle remove member
  const handleRemoveClick = (member: DepartmentMember) => {
    setMemberToDelete(member);
    setDeleteConfirmOpen(true);
  };

  const handleConfirmRemove = async () => {
    if (memberToDelete) {
      if (isOrgRootSelected) {
        // Remove from organization when org root is selected
        await removeMemberFromOrganization({
          accountId: memberToDelete.account_id,
        });
      } else {
        // Remove from department when a specific department is selected
        const deptId = memberToDelete.department_id || selectedDeptId;
        if (!deptId) return;

        await removeMember({
          deptId,
          accountId: memberToDelete.account_id,
        });
      }
      setDeleteConfirmOpen(false);
      setMemberToDelete(null);
    }
  };

  // Handle edit member
  const handleEditClick = (member: DepartmentMember) => {
    setMemberToEdit(member);
    setEditDialogOpen(true);
  };

  const handleResetPasswordClick = (member: DepartmentMember) => {
    setMemberToResetPassword(member);
    setResetPasswordDialogOpen(true);
  };

  const handleAssignWorkspaceClick = (member: DepartmentMember) => {
    setMemberToAssignWorkspace(member);
    setAssignWorkspaceDialogOpen(true);
  };

  const handleResetPassword = async (email: string, password?: string) => {
    await resetMemberPassword({
      email,
      ...(password ? { password } : {}),
    });
  };

  const handleSaveMemberDepartment = async (newDeptId: string, newMemberName?: string) => {
    if (!memberToEdit) return;

    const promises = [];

    if (memberToEdit.department_id !== newDeptId) {
      promises.push(
        updateMemberDepartment({
          deptId: memberToEdit.department_id,
          accountId: memberToEdit.account_id,
          newDeptId,
        })
      );
    }

    if (newMemberName) {
      promises.push(
        updateMemberNickname({
          memberId: memberToEdit.account_id,
          member_name: newMemberName,
        })
      );
    }

    if (promises.length > 0) {
      await Promise.all(promises);
      // If we are in the organization root view (viewing members without department or all members),
      // we need to refetch to update the list as the member might have been moved to a department
      // or their status might have changed relative to the current view.
      if (isOrgRootSelected) {
        refetchMembers();
      }
    }
  };

  // Show initial loading state for both sides
  if (isInitialLoad) {
    return (
      <div className="flex h-full flex-col space-y-5 overflow-hidden bg-bg-canvas/50 p-4 lg:p-6">
        <div className="flex shrink-0 flex-col gap-1">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-text-primary">
              {t('title')}
            </h1>
            <p className="text-sm text-text-secondary mt-1">{t('subtitle')}</p>
          </div>
        </div>
        <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
          {/* Left sidebar loading */}
          <div className="flex min-h-[220px] flex-col overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm md:min-h-0">
            <div className="border-b p-4">
              <Skeleton className="mb-3 h-4 w-24 rounded" />
              <Skeleton className="h-8 w-full rounded-md" />
            </div>
            <div className="flex-1 p-2 space-y-1.5">
              {[...Array(8)].map((_, i) => (
                <Skeleton key={i} className="h-8 w-full rounded-lg opacity-40" />
              ))}
            </div>
          </div>

          {/* Right content loading */}
          <div className="flex flex-col overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm">
            <div className="border-b p-5">
              <div className="flex items-center justify-between">
                <Skeleton className="h-8 w-48 rounded-lg" />
                <Skeleton className="h-10 w-64 rounded-md" />
              </div>
            </div>
            <div className="flex-1 p-5 space-y-4">
              {[...Array(6)].map((_, i) => (
                <div key={i} className="flex items-center gap-4">
                  <Skeleton className="h-12 w-12 rounded-lg opacity-50" />
                  <div className="flex-1 space-y-2">
                    <Skeleton className="h-4 w-32 rounded" />
                    <Skeleton className="h-3 w-48 rounded opacity-60" />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col space-y-5 overflow-hidden bg-bg-canvas/50 p-4 lg:p-6">
      <div className="shrink-0">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-text-primary">{t('title')}</h1>
          <p className="text-sm text-text-secondary mt-1 max-w-2xl">{t('subtitle')}</p>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
        {/* Left sidebar - Department tree */}
        <div className="flex min-h-[220px] flex-col overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm md:min-h-0">
          <div className="border-b border-border/60 bg-background px-4 py-3">
            <div className="mb-3 flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-foreground">
                  {t('departmentPanelTitle')}
                </h2>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {t('departmentPanelDescription')}
                </p>
              </div>
              <Button
                variant="outline"
                isIcon
                onClick={() => setCreateDepartmentDialogOpen(true)}
                className="h-8 w-8 shrink-0 rounded-md bg-background shadow-none"
                aria-label={t('createDepartment.title')}
              >
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                <Input
                  placeholder={t('searchDepartment')}
                  value={searchKeyword}
                  onChange={e => setSearchKeyword(e.target.value)}
                  className="h-9 rounded-md bg-bg-canvas/50 pl-8 text-xs shadow-none transition-all focus:border-primary/40 focus:ring-0"
                />
              </div>
            </div>
          </div>

          <div className="scrollbar-thin scrollbar-thumb-border/50 flex-1 overflow-y-auto p-2">
            {loadingDepartments ? (
              <div className="space-y-1.5 p-1.5">
                {[...Array(5)].map((_, i) => (
                  <Skeleton key={i} className="h-8 w-full rounded-xl opacity-40" />
                ))}
              </div>
            ) : filteredDepartments.length === 0 ? (
              <div className="px-4 py-10 text-center">
                <p className="text-sm font-medium text-muted-foreground">{t('noDepartments')}</p>
                <p className="mt-1 text-xs text-muted-foreground/70">
                  {t('noDepartmentsDescription')}
                </p>
              </div>
            ) : (
              <div className="space-y-1">
                {filteredDepartments.map(dept => (
                  <DepartmentTreeItem
                    key={dept.id}
                    department={dept}
                    selectedId={selectedDeptId}
                    onSelect={setSelectedDeptId}
                    searchKeyword={searchKeyword}
                    onAddSubDepartment={handleAddSubDepartment}
                    onEditDepartment={handleEditDepartment}
                    onDeleteDepartment={handleDeleteDepartment}
                  />
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Right content - Members list */}
        <div className="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm">
          <div className="flex flex-col gap-4 border-b border-border/60 bg-background px-5 py-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="flex items-center gap-2 text-xl font-semibold tracking-tight text-text-primary">
                  <span className="truncate">{selectedDeptName || t('allMembers')}</span>
                  {!loadingMembers && selectedDeptName && (
                    <span className="shrink-0 rounded-full bg-bg-subtle px-2.5 py-1 text-xs font-medium text-muted-foreground">
                      {t('memberCount', { count: total })}
                    </span>
                  )}
                </h2>
                {selectedScopeLabel ? (
                  <Badge variant={isOrgRootSelected ? 'warning' : 'info'} className="font-medium">
                    {selectedScopeLabel}
                  </Badge>
                ) : null}
              </div>
              <p className="mt-1 text-sm text-muted-foreground">{selectedScopeDescription}</p>
              {selectedScopeHint ? (
                <p className="mt-1 text-xs text-muted-foreground/80">{selectedScopeHint}</p>
              ) : null}
            </div>
            <div className="flex w-full flex-col gap-2 sm:flex-row lg:w-auto">
              <div className="relative w-full sm:w-72">
                <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t('searchMembers')}
                  className="h-10 rounded-md bg-bg-canvas/50 pl-9 text-xs shadow-none transition-all focus:border-primary/40 focus:ring-0"
                  value={memberSearchKeyword}
                  onChange={e => setMemberSearchKeyword(e.target.value)}
                />
              </div>
              <Button
                onClick={() => setAddMemberDialogOpen(true)}
                className="h-10 shrink-0 rounded-md bg-primary px-4 font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary-hover hover:text-primary-foreground"
              >
                <Plus className="h-4 w-4" />
                {selectedDeptId
                  ? isOrgRootSelected
                    ? t('addMember.addOrganizationMember')
                    : t('addMember.addDepartmentMember')
                  : t('addMember.title')}
              </Button>
            </div>
          </div>

          <StickyDataTable
            columns={[
              { key: 'name', header: t('name'), className: 'pl-6' },
              { key: 'email', header: t('email') },
              { key: 'organizationRole', header: t('organizationRole') },
              { key: 'department', header: t('editDialog.department') },
              { key: 'workspaces', header: t('workspaces') },
              { key: 'status', header: t('status') },
              { key: 'actions', header: t('actions'), className: 'w-[120px]' },
            ]}
            data={members}
            getRowKey={member => member.account_id}
            isLoading={loadingMembers}
            loadingRows={6}
            renderSkeletonRow={index => (
              <tr key={`member-skeleton-${index}`} className="border-b border-border/10">
                <td colSpan={7} className="px-6 py-4">
                  <div className="flex items-center gap-4">
                    <Skeleton className="h-12 w-12 rounded-lg opacity-40" />
                    <div className="flex-1 space-y-2">
                      <Skeleton className="h-4 w-32 rounded" />
                      <Skeleton className="h-3 w-48 rounded opacity-20" />
                    </div>
                  </div>
                </td>
              </tr>
            )}
            emptyState={
              !selectedDeptId ? (
                <div className="flex flex-col items-center justify-center flex-1 py-20 text-text-placeholder">
                  <Search className="h-12 w-12 opacity-10 mb-4" />
                  <p className="text-sm font-medium">{t('selectDepartment')}</p>
                </div>
              ) : hasMemberSearch ? (
                <div className="flex flex-col items-center justify-center flex-1 py-20 text-text-placeholder">
                  <Search className="mb-4 h-12 w-12 opacity-10" />
                  <p className="text-sm font-medium">{t('noMemberSearchResults')}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('noMemberSearchResultsDescription')}
                  </p>
                </div>
              ) : (
                <div className="flex flex-col items-center justify-center flex-1 py-20 text-text-placeholder">
                  <Users className="h-12 w-12 opacity-10 mb-4" />
                  <p className="text-sm font-medium">{t('noMembers')}</p>
                </div>
              )
            }
            pagination={
              totalPages > 1 ? (
                <div className="p-4 bg-bg-canvas/30 border-t border-border/20 backdrop-blur-sm">
                  <Pagination
                    currentPage={currentPage}
                    totalPages={totalPages}
                    total={total}
                    pageSize={pageSize}
                    onPageChange={setCurrentPage}
                    showInfo
                    showJump={false}
                    className="justify-center md:justify-end"
                  />
                </div>
              ) : null
            }
            renderRow={(member: DepartmentMember) => (
              <>
                <td className="py-4 pl-6">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-lg bg-primary text-primary-foreground flex items-center justify-center font-bold text-xs shadow-sm">
                      {member.account_name.charAt(0).toUpperCase()}
                    </div>
                    <span className="font-semibold text-text-primary text-[13px] group-hover:text-primary transition-colors truncate max-w-[150px]">
                      {member.member_name || member.account_name}
                    </span>
                  </div>
                </td>
                <td className="py-4 text-[13px] text-text-secondary font-medium">
                  {member.account_email}
                </td>
                <td className="py-4">
                  <Badge
                    variant={
                      member.organization_role === 'owner'
                        ? 'warning'
                        : member.organization_role === 'admin'
                          ? 'info'
                          : 'subtle'
                    }
                    className="rounded-md px-2 py-px text-[10px] font-medium"
                  >
                    {getOrganizationRoleLabel(member.organization_role)}
                  </Badge>
                </td>
                <td className="py-4">
                  <div className="flex">
                    <Badge
                      variant={member.department_name ? 'info' : 'outline'}
                      className={cn(
                        'text-[10px] py-px font-medium',
                        !member.department_name && 'border-dashed'
                      )}
                    >
                      {member.department_name || currentOrganizationDisplayName}
                    </Badge>
                  </div>
                </td>
                <td className="py-4">
                  <div className="flex flex-wrap gap-1.5 max-w-[320px] max-h-[50px] overflow-auto">
                    {member.joined_workspaces?.length ? (
                      <>
                        {member.joined_workspaces?.slice(0, 2).map((workspace: JoinedWorkspace) => (
                          <Badge
                            variant="secondary"
                            key={workspace.workspace_id}
                            title={workspace.workspace_name}
                            className="text-[10px] py-px font-medium max-w-[100px] overflow-ellipsis line-clamp-1"
                          >
                            {workspace.workspace_name}
                          </Badge>
                        ))}
                        {member.joined_workspaces?.length > 2 && (
                          <Tooltip delayDuration={200}>
                            <TooltipTrigger asChild>
                              <Badge className="text-[10px] py-0 px-2 rounded-md bg-bg-subtle hover:bg-bg-surface border-border/30 text-text-placeholder font-bold shadow-none cursor-pointer">
                                +{member.joined_workspaces?.length - 2}
                              </Badge>
                            </TooltipTrigger>
                            <TooltipContent side="top" align="center" className="text-xs">
                              <div className="flex flex-wrap gap-1.5 p-1 max-w-[200px]">
                                {member.joined_workspaces
                                  ?.slice(2)
                                  .map((workspace: JoinedWorkspace) => (
                                    <Badge
                                      key={workspace.workspace_id}
                                      variant="secondary"
                                      className="text-[10px]"
                                    >
                                      {workspace.workspace_name}
                                    </Badge>
                                  ))}
                              </div>
                            </TooltipContent>
                          </Tooltip>
                        )}
                      </>
                    ) : (
                      <div className="flex items-center gap-1">
                        <Tooltip delayDuration={200}>
                          <TooltipTrigger asChild>
                            <Badge
                              variant="warning"
                              className="border-warning/30 bg-warning/10 text-[10px] font-medium text-warning"
                            >
                              {t('unassignedWorkspace')}
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-56 text-xs">
                            {t('unassignedWorkspaceHint')}
                          </TooltipContent>
                        </Tooltip>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="xs"
                              isIcon
                              onClick={() => handleAssignWorkspaceClick(member)}
                              className="h-6 w-6 rounded-md text-primary"
                            >
                              <UserPlus className="h-3.5 w-3.5" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent className="text-xs">
                            {t('assignWorkspace')}
                          </TooltipContent>
                        </Tooltip>
                      </div>
                    )}
                  </div>
                </td>
                <td className="py-4">
                  <div className="flex">
                    <Badge
                      variant="secondary"
                      className={cn(
                        'px-2.5 py-0.5 rounded-md font-bold text-[10px] uppercase tracking-tight border-none shadow-none',
                        member.group_status === 'active'
                          ? 'bg-green-500/10 text-green-600'
                          : 'bg-border text-muted-foreground'
                      )}
                    >
                      <span
                        className={cn(
                          'w-1.5 h-1.5 rounded-full mr-1.5',
                          member.group_status === 'active' ? 'bg-green-500' : 'bg-border-strong'
                        )}
                      />
                      {member.group_status === 'active' ? t('active') : t('disabled')}
                    </Badge>
                  </div>
                </td>
                <td className="py-4">
                  <div className="flex items-center gap-2 transition-opacity">
                    {member.group_status === 'active' && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="xs"
                            isIcon
                            onClick={() => handleEditClick(member)}
                            className="rounded-md"
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent className="text-xs">{t('edit')}</TooltipContent>
                      </Tooltip>
                    )}

                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="xs"
                          isIcon
                          className="rounded-md text-text-placeholder shadow-none"
                        >
                          <MoreHorizontal className="h-4 w-4" />
                          <span className="sr-only">{t('actions')}</span>
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-44">
                        <DropdownMenuItem
                          disabled={isUpdatingStatus}
                          onSelect={() => handleToggleStatus(member)}
                        >
                          <Power className="h-4 w-4" />
                          {member.group_status === 'active' ? t('disable') : t('enable')}
                        </DropdownMenuItem>
                        {canResetMemberPassword(member) ? (
                          <DropdownMenuItem onSelect={() => handleResetPasswordClick(member)}>
                            <KeyRound className="h-4 w-4" />
                            {t('resetPassword.action')}
                          </DropdownMenuItem>
                        ) : null}
                        <DropdownMenuItem
                          variant="destructive"
                          onSelect={() => handleRemoveClick(member)}
                        >
                          <Trash2 className="h-4 w-4" />
                          {isOrgRootSelected
                            ? t('removeFromOrganization')
                            : t('removeFromDepartment')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </td>
              </>
            )}
          />
        </div>
      </div>

      {/* Add Member Dialog */}
      <AddMemberDialog
        open={addMemberDialogOpen}
        onOpenChange={setAddMemberDialogOpen}
        departments={departmentsTree}
        defaultDepartmentId={selectedDeptId}
      />

      {/* Remove Member Confirmation Dialog */}
      <ConfirmDialog
        variant="danger"
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={isOrgRootSelected ? t('removeOrganizationConfirm.title') : t('removeConfirm.title')}
        description={
          isOrgRootSelected
            ? t('removeOrganizationConfirm.description', {
                memberName: memberToDelete?.member_name || memberToDelete?.account_name || '',
              })
            : t('removeConfirm.description', {
                memberName: memberToDelete?.member_name || memberToDelete?.account_name || '',
              })
        }
        confirmText={
          isOrgRootSelected ? t('removeOrganizationConfirm.confirm') : t('removeConfirm.confirm')
        }
        cancelText={
          isOrgRootSelected ? t('removeOrganizationConfirm.cancel') : t('removeConfirm.cancel')
        }
        loading={isOrgRootSelected ? isRemovingFromOrg : isRemoving}
        onConfirm={handleConfirmRemove}
      />

      {/* Edit Member Dialog */}
      <EditMemberDialog
        open={editDialogOpen}
        onOpenChange={open => {
          if (!open) {
            setMemberToEdit(null);
          }
          setEditDialogOpen(open);
        }}
        member={memberToEdit}
        departments={departmentsTree}
        onSave={handleSaveMemberDepartment}
        isSaving={isUpdatingDepartment || isUpdatingNickname}
      />

      {!IS_CLOUD && (
        <ResetMemberPasswordDialog
          open={resetPasswordDialogOpen}
          onOpenChange={open => {
            if (!open) {
              setMemberToResetPassword(null);
            }
            setResetPasswordDialogOpen(open);
          }}
          member={memberToResetPassword}
          onReset={handleResetPassword}
          isResetting={isResettingPassword}
        />
      )}

      {memberToAssignWorkspace ? (
        <AssignWorkspaceDialog
          open={assignWorkspaceDialogOpen}
          member={memberToAssignWorkspace}
          onOpenChange={open => {
            setAssignWorkspaceDialogOpen(open);
            if (!open) {
              setMemberToAssignWorkspace(null);
            }
          }}
          onAssigned={() => {
            refetchMembers();
          }}
        />
      ) : null}

      {/* Create Department Dialog */}
      <CreateDepartmentDialog
        open={createDepartmentDialogOpen}
        onOpenChange={open => {
          if (!open) {
            setParentDepartmentForCreate(null);
          }
          setCreateDepartmentDialogOpen(open);
        }}
        departments={departmentsTree}
        parentDepartmentId={parentDepartmentForCreate?.id}
      />

      {/* Edit Department Dialog */}
      <EditDepartmentDialog
        open={editDepartmentDialogOpen}
        onOpenChange={open => {
          if (!open) {
            setDepartmentToEdit(null);
          }
          setEditDepartmentDialogOpen(open);
        }}
        department={departmentToEdit}
        departments={departmentsTree}
      />

      {/* Delete Department Confirm Dialog */}
      <ConfirmDialog
        open={deleteDepartmentConfirmOpen}
        onOpenChange={setDeleteDepartmentConfirmOpen}
        title={t('deleteDepartment.title')}
        description={t('deleteDepartment.description', {
          departmentName: departmentToDelete?.name || '',
        })}
        confirmText={t('deleteDepartment.confirm')}
        cancelText={t('deleteDepartment.cancel')}
        loading={isDeleting}
        onConfirm={handleConfirmDeleteDepartment}
        variant="danger"
      />

      <ConfirmDialog
        open={statusConfirmOpen}
        onOpenChange={setStatusConfirmOpen}
        title={t('toggleStatusConfirm.title')}
        description={
          memberToToggle?.group_status === 'active'
            ? t('toggleStatusConfirm.disableDescription', {
                memberName: memberToToggle?.account_name || '',
              })
            : t('toggleStatusConfirm.enableDescription', {
                memberName: memberToToggle?.account_name || '',
              })
        }
        confirmText={
          memberToToggle?.group_status === 'active'
            ? t('toggleStatusConfirm.disableConfirm')
            : t('toggleStatusConfirm.enableConfirm')
        }
        cancelText={tRoot('common.cancel')}
        loading={isUpdatingStatus}
        onConfirm={handleConfirmToggleStatus}
        variant={memberToToggle?.group_status === 'active' ? 'warning' : 'default'}
      />
    </div>
  );
}
