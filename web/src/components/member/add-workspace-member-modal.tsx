'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Loader2, Search, Users } from 'lucide-react';
import { toast } from 'sonner';

import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  DepartmentSelector,
  isDepartmentSelectorContent,
} from '@/components/dashboard/organization/department-selector';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useAvailableWorkspaceMembers } from '@/hooks/workspace/use-available-workspace-members';
import { useDepartments } from '@/hooks/organization/use-departments';
import { useOrganizationRoles } from '@/hooks/organization/use-organization-roles';
import type { AvailableWorkspaceMember, BatchAddMembersResponse } from '@/services/types/workspace';

interface AddWorkspaceMemberModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  workspaceName?: string;
  initialSearchQuery?: string;
  isLoading?: boolean;
  onAdd: (memberIds: string[], roleId?: string) => Promise<BatchAddMembersResponse | void>;
}

/**
 * @component AddWorkspaceMemberModal
 * @category Feature
 * @status Stable
 * @description Paginated workspace member candidate picker backed by the workspace available-members API
 * @usage Use in workspace member management pages to add organization members into a workspace
 * @example
 * <AddWorkspaceMemberModal open={open} workspaceId={workspaceId} onAdd={handleAdd} />
 */
export function AddWorkspaceMemberModal({
  open,
  onOpenChange,
  workspaceId,
  workspaceName,
  initialSearchQuery,
  isLoading = false,
  onAdd,
}: AddWorkspaceMemberModalProps) {
  const t = useT('dashboard');
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedMemberIds, setSelectedMemberIds] = useState<string[]>([]);
  const [selectedDepartmentId, setSelectedDepartmentId] = useState<string>('all');
  const [selectedRoleId, setSelectedRoleId] = useState<string>('');
  const [currentPage, setCurrentPage] = useState(1);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const debouncedSearchQuery = useDebouncedValue(searchQuery, 400);
  const pageSize = 20;

  const { departments } = useDepartments();
  const { roles, isLoading: isLoadingRoles } = useOrganizationRoles();

  const {
    members,
    total,
    isLoading: isLoadingCandidates,
    isFetching: isFetchingCandidates,
  } = useAvailableWorkspaceMembers(workspaceId, {
    enabled: open,
    department_id: selectedDepartmentId !== 'all' ? selectedDepartmentId : undefined,
    include_sub_depts: 'true',
    keyword: debouncedSearchQuery,
    page: currentPage,
    limit: pageSize,
  });

  const availableMembers = useMemo(() => {
    const deduped = new Map<string, AvailableWorkspaceMember>();
    members.forEach(member => {
      if (!deduped.has(member.account_id)) {
        deduped.set(member.account_id, member);
      }
    });
    return Array.from(deduped.values());
  }, [members]);

  const selectableRoles = useMemo(
    () =>
      roles.filter(
        role =>
          role.status === 'active' &&
          role.id.toLowerCase() !== 'owner' &&
          role.name.toLowerCase() !== 'owner'
      ),
    [roles]
  );

  const currentPageMemberIds = useMemo(
    () => availableMembers.map(member => member.account_id),
    [availableMembers]
  );
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const isPageFullySelected =
    currentPageMemberIds.length > 0 &&
    currentPageMemberIds.every(memberId => selectedMemberIds.includes(memberId));
  const isBusy = isSubmitting || isLoading;
  const shouldShowLoading =
    isLoadingCandidates || (isFetchingCandidates && availableMembers.length === 0);

  useEffect(() => {
    setCurrentPage(1);
  }, [debouncedSearchQuery, selectedDepartmentId]);

  useEffect(() => {
    if (currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, totalPages]);

  useEffect(() => {
    if (!open) return;
    setSearchQuery(initialSearchQuery ?? '');
    setSelectedMemberIds([]);
    setSelectedDepartmentId('all');
    setSelectedRoleId('');
    setCurrentPage(1);
  }, [initialSearchQuery, open]);

  const showAddSummary = useCallback(
    (result?: BatchAddMembersResponse | void) => {
      if (!result) {
        return;
      }

      const added = result.added_count ?? 0;
      const skipped = result.skipped_count ?? 0;
      const failed = result.failed_count ?? 0;
      const summary = t('organization.workspaceManagement.detail.addMemberModal.addSummary', {
        added,
        skipped,
        failed,
      });

      if (failed > 0) {
        toast.error(summary);
      } else {
        toast.success(summary);
      }

      result.invitation_results
        ?.filter(item => item.status === 'failed')
        .slice(0, 5)
        .forEach(item => {
          toast.error(
            item.message ||
              item.reason ||
              t('organization.workspaceManagement.detail.addMemberModal.addItemFailed')
          );
        });
    },
    [t]
  );

  const handleSelectPage = useCallback(() => {
    setSelectedMemberIds(prev => {
      if (currentPageMemberIds.every(memberId => prev.includes(memberId))) {
        return prev.filter(memberId => !currentPageMemberIds.includes(memberId));
      }
      return Array.from(new Set([...prev, ...currentPageMemberIds]));
    });
  }, [currentPageMemberIds]);

  const handleSelectMember = useCallback((memberId: string) => {
    setSelectedMemberIds(prev => {
      if (prev.includes(memberId)) {
        return prev.filter(id => id !== memberId);
      }
      return [...prev, memberId];
    });
  }, []);

  const resetAndClose = useCallback(() => {
    setSelectedMemberIds([]);
    setSearchQuery('');
    setSelectedDepartmentId('all');
    setSelectedRoleId('');
    setCurrentPage(1);
    onOpenChange(false);
  }, [onOpenChange]);

  const handleSubmit = useCallback(async () => {
    if (selectedMemberIds.length === 0 || !selectedRoleId) return;

    setIsSubmitting(true);
    try {
      const result = await onAdd(selectedMemberIds, selectedRoleId);
      showAddSummary(result);
      resetAndClose();
    } catch (_error) {
      // Error toast is handled by the mutation hook.
    } finally {
      setIsSubmitting(false);
    }
  }, [onAdd, resetAndClose, selectedMemberIds, selectedRoleId, showAddSummary]);

  const handleClose = useCallback(() => {
    if (isSubmitting) return;
    resetAndClose();
  }, [isSubmitting, resetAndClose]);

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent
        className="w-[800px] max-w-[800px] max-h-[80vh] flex flex-col overflow-hidden"
        onInteractOutside={event => {
          if (isDepartmentSelectorContent(event.target)) {
            event.preventDefault();
          }
        }}
      >
        <DialogHeader className="flex-shrink-0">
          <DialogTitle>
            {workspaceName
              ? t('organization.workspaceManagement.detail.addMemberModal.titleWithWorkspace', {
                  workspaceName,
                })
              : t('organization.workspaceManagement.detail.addMemberModal.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="flex flex-col space-y-4 min-h-0 flex-1">
          <div className="flex items-center gap-2 flex-shrink-0">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder={t(
                  'organization.workspaceManagement.detail.addMemberModal.searchPlaceholder'
                )}
                value={searchQuery}
                onChange={event => setSearchQuery(event.target.value)}
                className="pl-10"
              />
            </div>
            <DepartmentSelector
              departments={departments}
              value={selectedDepartmentId}
              onValueChange={setSelectedDepartmentId}
              placeholder={t(
                'organization.workspaceManagement.detail.addMemberModal.allDepartments'
              )}
              emptyLabel={t(
                'organization.workspaceManagement.detail.addMemberModal.allDepartments'
              )}
              emptyValue="all"
              allowEmpty
              containerOpen={open}
              className="w-[200px]"
              contentClassName="max-h-[400px]"
            />
          </div>

          {availableMembers.length > 0 ? (
            <div className="flex items-center gap-3 p-2 flex-shrink-0">
              <Checkbox checked={isPageFullySelected} onCheckedChange={handleSelectPage} />
              <span className="text-sm font-medium">
                {t('organization.workspaceManagement.detail.addMemberModal.selectAll', {
                  count: availableMembers.length,
                })}
              </span>
            </div>
          ) : null}

          <div className="flex-1 min-h-0 overflow-y-auto">
            {shouldShowLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 6 }).map((_, index) => (
                  <div key={index} className="flex items-center gap-3 p-3 border rounded">
                    <div className="w-4 h-4 bg-muted animate-pulse rounded" />
                    <div className="w-10 h-10 bg-muted animate-pulse rounded-full" />
                    <div className="flex-1 space-y-2">
                      <div className="w-24 h-4 bg-muted animate-pulse rounded" />
                      <div className="w-48 h-3 bg-muted animate-pulse rounded" />
                    </div>
                  </div>
                ))}
              </div>
            ) : availableMembers.length > 0 ? (
              <div className="space-y-1">
                {availableMembers.map(member => {
                  const isSelected = selectedMemberIds.includes(member.account_id);
                  return (
                    <div
                      key={member.account_id}
                      className="flex items-center gap-3 p-3 hover:bg-muted/50 rounded cursor-pointer"
                      onClick={() => handleSelectMember(member.account_id)}
                    >
                      <div onClick={event => event.stopPropagation()}>
                        <Checkbox
                          checked={isSelected}
                          onCheckedChange={() => handleSelectMember(member.account_id)}
                        />
                      </div>
                      <div className="flex-1 flex items-center gap-3 min-w-0">
                        <span className="font-medium flex-1 min-w-0 truncate">
                          {member.member_name || member.account_name}
                        </span>
                        <span className="text-sm text-muted-foreground flex-1 min-w-0 truncate">
                          {member.account_email}
                        </span>
                        <span className="text-sm text-muted-foreground flex-1 min-w-0 truncate">
                          {member.department_name || '-'}
                        </span>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="text-center py-8 text-muted-foreground">
                <Users className="mx-auto h-12 w-12 mb-4 opacity-50" />
                <p>
                  {t('organization.workspaceManagement.detail.addMemberModal.noAvailableMembers')}
                </p>
              </div>
            )}
          </div>

          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            total={total}
            pageSize={pageSize}
            onPageChange={setCurrentPage}
            showJump={false}
            className="flex-shrink-0 border-t pt-3"
          />

          <div className="space-y-2 pt-4 border-t flex-shrink-0">
            <label className="text-sm font-medium">
              {t('organization.workspaceManagement.detail.addMemberModal.assignRole')}
            </label>
            <Select
              value={selectedRoleId}
              onValueChange={setSelectedRoleId}
              disabled={isLoadingRoles}
            >
              <SelectTrigger>
                <SelectValue
                  placeholder={t(
                    'organization.workspaceManagement.detail.addMemberModal.selectRole'
                  )}
                />
              </SelectTrigger>
              <SelectContent>
                {selectableRoles.map(role => (
                  <SelectItem key={role.id} value={role.id}>
                    {role.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </DialogBody>

        <DialogFooter className="gap-2 flex-shrink-0">
          <Button variant="outline" onClick={handleClose} disabled={isBusy}>
            {t('organization.workspaceManagement.detail.addMemberModal.cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={selectedMemberIds.length === 0 || !selectedRoleId || isBusy}
          >
            {isBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t('organization.workspaceManagement.detail.addMemberModal.add', {
              count: selectedMemberIds.length,
            })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
