'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import { AlertTriangle, Check, Loader2, LogOut, Pencil, Search, X } from 'lucide-react';
import { useCurrentWorkspace, usePermissions, useWorkspaceStore } from '@/store/workspace-store';
import { useWorkspaceActions } from '@/hooks/workspace/use-workspace-actions';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useWorkspaceMembers } from '@/hooks/workspace/use-workspace-members';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';

export default function WorkspaceSettingsPage() {
  const t = useT('workspace');
  const tCommon = useT('common');
  const router = useRouter();

  const currentWorkspace = useCurrentWorkspace();
  const { currentOrganization } = useOrganizations();
  const { workspaceRole, permissions } = usePermissions();
  const isOwner = workspaceRole === 'owner';
  const canManage = permissions.includes('workspace.manage');

  const {
    updateWorkspace,
    transferOwnership,
    leaveWorkspace,
    isUpdating,
    isTransferring,
    isLeaving,
  } = useWorkspaceActions();
  const workspaces = useWorkspaceStore.use.workspaces();
  const setWorkspaces = useWorkspaceStore.use.setWorkspaces();
  const setCurrentWorkspace = useWorkspaceStore.use.setCurrentWorkspace();

  // Workspace Name Editing State
  const [isEditingName, setIsEditingName] = useState(false);
  const [editedName, setEditedName] = useState('');

  // Transfer Ownership State
  const [selectedNewOwnerId, setSelectedNewOwnerId] = useState<string>('');
  const [selectedNewOwnerName, setSelectedNewOwnerName] = useState<string>('');
  const [ownerSearchKeyword, setOwnerSearchKeyword] = useState('');
  const [ownerPage, setOwnerPage] = useState(1);
  const ownerPageSize = 20;
  const debouncedOwnerSearchKeyword = useDebouncedValue(ownerSearchKeyword, 400);

  const {
    members,
    total: ownerCandidatesTotal,
    isLoading: isLoadingMembers,
  } = useWorkspaceMembers(currentOrganization?.id || null, currentWorkspace?.id || null, {
    keyword: debouncedOwnerSearchKeyword,
    page: ownerPage,
    limit: ownerPageSize,
    enabled: isOwner && !!currentWorkspace,
  });
  const ownerCandidatesTotalPages = Math.max(1, Math.ceil(ownerCandidatesTotal / ownerPageSize));

  useEffect(() => {
    if (currentWorkspace) {
      setEditedName(currentWorkspace.name);
    }
  }, [currentWorkspace]);

  useEffect(() => {
    setOwnerPage(1);
  }, [debouncedOwnerSearchKeyword, currentWorkspace?.id]);

  useEffect(() => {
    if (ownerPage > ownerCandidatesTotalPages) {
      setOwnerPage(ownerCandidatesTotalPages);
    }
  }, [ownerCandidatesTotalPages, ownerPage]);

  const handleUpdateName = async () => {
    if (
      !currentWorkspace ||
      !currentOrganization ||
      !editedName.trim() ||
      editedName === currentWorkspace.name
    ) {
      setIsEditingName(false);
      return;
    }

    try {
      await updateWorkspace({
        workspaceId: currentWorkspace.id,
        data: { name: editedName.trim() },
      });

      // Update local store
      const newName = editedName.trim();
      setCurrentWorkspace({ ...currentWorkspace, name: newName });
      setWorkspaces(
        workspaces.map(w => (w.id === currentWorkspace.id ? { ...w, name: newName } : w))
      );

      setIsEditingName(false);
    } catch (error) {
      console.error('Failed to update workspace name:', error);
    }
  };

  const handleTransferOwnership = async () => {
    if (!currentWorkspace || !currentOrganization || !selectedNewOwnerId) return;

    try {
      await transferOwnership({
        workspaceId: currentWorkspace.id,
        data: { new_owner_id: selectedNewOwnerId },
      });
      setSelectedNewOwnerId('');
      setSelectedNewOwnerName('');
    } catch (error) {
      console.error('Failed to transfer ownership:', error);
    }
  };

  const handleLeaveWorkspace = async () => {
    if (!currentWorkspace || !currentOrganization) return;

    try {
      await leaveWorkspace(currentWorkspace.id);
      // Redirect to console home after leaving
      router.push('/console');
    } catch (error) {
      console.error('Failed to leave workspace:', error);
    }
  };

  if (!currentWorkspace) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Filter out the current owner (user) from the potential new owners
  const otherMembers = members.filter(m => m.role !== 'owner');

  return (
    <div className="mx-auto max-w-4xl px-6 py-6">
      {/* Header */}
      <div className="mb-5">
        <h2 className="text-2xl font-semibold tracking-tight text-foreground">
          {t('settings.title')}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">{t('settings.description')}</p>
      </div>

      <div className="grid gap-5">
        {/* Basic Information Card */}
        <Card className="border-border/80 shadow-sm">
          <CardHeader className="space-y-1.5">
            <CardTitle className="text-base">
              {t('settings.basicInfo.title') || 'Basic Information'}
            </CardTitle>
            <CardDescription>
              {t('settings.basicInfo.description') ||
                "View and manage your workspace's basic details."}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className="flex flex-col space-y-1.5">
              <label className="text-sm font-medium leading-5 text-muted-foreground">
                {t('settings.basicInfo.workspaceName') || 'Workspace Name'}
              </label>
              <div className="flex min-h-9 items-center gap-2">
                {isEditingName ? (
                  <>
                    <Input
                      value={editedName}
                      onChange={e => setEditedName(e.target.value)}
                      className="h-9 max-w-md"
                      autoFocus
                    />
                    <Button
                      isIcon
                      variant="ghost"
                      onClick={handleUpdateName}
                      disabled={isUpdating || !editedName.trim()}
                    >
                      {isUpdating ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Check className="h-4 w-4 text-green-500" />
                      )}
                    </Button>
                    <Button
                      isIcon
                      variant="ghost"
                      onClick={() => {
                        setIsEditingName(false);
                        setEditedName(currentWorkspace.name);
                      }}
                      disabled={isUpdating}
                    >
                      <X className="h-4 w-4 text-destructive" />
                    </Button>
                  </>
                ) : (
                  <>
                    <span className="text-base font-medium leading-6 text-foreground">
                      {currentWorkspace.name}
                    </span>
                    {canManage && (
                      <Button
                        isIcon
                        variant="ghost"
                        onClick={() => setIsEditingName(true)}
                        className="h-8 w-8 text-muted-foreground hover:text-foreground"
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                    )}
                  </>
                )}
              </div>
            </div>

            <div className="flex flex-col space-y-1.5">
              <label className="text-sm font-medium leading-5 text-muted-foreground">
                {t('settings.basicInfo.workspaceId') || 'Workspace ID'}
              </label>
              <span className="select-all font-mono text-sm leading-6 text-muted-foreground">
                {currentWorkspace.id}
              </span>
            </div>
          </CardContent>
        </Card>

        {/* Transfer Ownership Card */}
        {workspaceRole && isOwner && (
          <Card className="border-border/80 shadow-sm">
            <CardHeader className="space-y-1.5">
              <CardTitle className="flex items-center gap-2 text-base text-foreground">
                <AlertTriangle className="h-4 w-4 text-muted-foreground" />
                {t('settings.transfer.title') || 'Transfer Ownership'}
              </CardTitle>
              <CardDescription>
                {t('settings.transfer.description') ||
                  'Transfer this workspace to another member. This action cannot be undone.'}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col space-y-2">
                <label className="text-sm font-medium">
                  {t('settings.transfer.selectMember') || 'Select New Owner'}
                </label>
                <div className="space-y-3">
                  <div className="relative max-w-md">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      value={ownerSearchKeyword}
                      onChange={event => setOwnerSearchKeyword(event.target.value)}
                      placeholder={t('settings.transfer.searchPlaceholder') || 'Search members'}
                      className="h-9 bg-background pl-9"
                    />
                  </div>
                  <div className="flex flex-wrap items-center gap-3">
                    <Select
                      value={selectedNewOwnerId}
                      onValueChange={value => {
                        setSelectedNewOwnerId(value);
                        setSelectedNewOwnerName(otherMembers.find(m => m.id === value)?.name || '');
                      }}
                      disabled={isLoadingMembers || isTransferring}
                    >
                      <SelectTrigger className="h-9 max-w-md bg-background">
                        <SelectValue
                          placeholder={t('settings.transfer.placeholder') || 'Choose a member...'}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {isLoadingMembers ? (
                          <div className="p-2 flex items-center justify-center">
                            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                          </div>
                        ) : otherMembers.length > 0 ? (
                          otherMembers.map(member => (
                            <SelectItem key={member.id} value={member.id}>
                              <div className="flex items-center gap-2">
                                <Avatar className="h-5 w-5">
                                  <AvatarImage
                                    src={member.avatar_url || member.avatar || undefined}
                                  />
                                  <AvatarFallback className="text-[8px]">
                                    {member.name.slice(0, 2).toUpperCase()}
                                  </AvatarFallback>
                                </Avatar>
                                <span>{member.name}</span>
                                <span className="text-xs text-muted-foreground">
                                  ({member.email})
                                </span>
                              </div>
                            </SelectItem>
                          ))
                        ) : (
                          <div className="p-2 text-sm text-center text-muted-foreground">
                            {t('settings.transfer.noMembers')}
                          </div>
                        )}
                      </SelectContent>
                    </Select>

                    <ConfirmDialog
                      title={t('settings.transfer.confirm.title')}
                      description={t('settings.transfer.confirm.description', {
                        name: selectedNewOwnerName || 'the selected member',
                      })}
                      confirmText={t('settings.transfer.confirm.button') || 'Transfer'}
                      cancelText={tCommon('cancel')}
                      variant="warning"
                      loading={isTransferring}
                      onConfirm={handleTransferOwnership}
                      trigger={
                        <Button
                          variant="outline"
                          className="border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive"
                          disabled={!selectedNewOwnerId || isTransferring}
                        >
                          {isTransferring ? (
                            <Loader2 className="h-4 w-4 animate-spin mr-2" />
                          ) : null}
                          {t('settings.transfer.button') || 'Transfer'}
                        </Button>
                      }
                    />
                  </div>
                  <Pagination
                    currentPage={ownerPage}
                    totalPages={ownerCandidatesTotalPages}
                    total={ownerCandidatesTotal}
                    pageSize={ownerPageSize}
                    onPageChange={setOwnerPage}
                    showJump={false}
                    className="max-w-md"
                  />
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Leave Workspace Card */}
        {workspaceRole && !isOwner && (
          <Card className="border-border/80 shadow-sm">
            <CardHeader className="space-y-1.5">
              <CardTitle className="flex items-center gap-2 text-base text-foreground">
                <LogOut className="h-4 w-4 text-muted-foreground" />
                {t('settings.leave.title') || 'Leave Workspace'}
              </CardTitle>
              <CardDescription>
                {t('settings.leave.description') ||
                  'Leave this workspace. You will lose access to its resources.'}
              </CardDescription>
            </CardHeader>
            <CardFooter>
              <ConfirmDialog
                title={t('settings.leave.confirm.title')}
                description={t('settings.leave.confirm.description')}
                confirmText={t('settings.leave.confirm.button') || 'Leave'}
                cancelText={tCommon('cancel')}
                variant="warning"
                loading={isLeaving}
                onConfirm={handleLeaveWorkspace}
                trigger={
                  <Button
                    variant="outline"
                    className="border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive"
                    disabled={isLeaving}
                  >
                    {isLeaving ? (
                      <Loader2 className="h-4 w-4 animate-spin mr-2" />
                    ) : (
                      <LogOut className="h-4 w-4 mr-2" />
                    )}
                    {t('settings.leave.button') || 'Leave Workspace'}
                  </Button>
                }
              />
            </CardFooter>
          </Card>
        )}
      </div>
    </div>
  );
}
