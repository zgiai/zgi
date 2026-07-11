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
import { useCurrentOrganizationMembers } from '@/hooks/organization/use-current-organization-members';
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
  const workspaceId = open && isEditMode ? (initialData?.id ?? '') : '';
  const { workspaceDetail } = useWorkspaceDetail(workspaceId);
  const resolvedInitialData = workspaceDetail ?? initialData;

  // Form state
  const [workspaceName, setWorkspaceName] = useState(resolvedInitialData?.name || '');
  const [nameError, setNameError] = useState('');
  const [leaderId, setLeaderId] = useState<string>(resolvedInitialData?.leader_id || '');
  const [leaderError, setLeaderError] = useState('');
  const [apiKeyId, setApiKeyId] = useState<string>(resolvedInitialData?.api_key_id || '');
  // Initialize form when initialData changes or dialog opens
  useEffect(() => {
    if (open) {
      if (resolvedInitialData) {
        setWorkspaceName(resolvedInitialData.name || '');
        setApiKeyId(resolvedInitialData.api_key_id || '');
        setLeaderId(resolvedInitialData.leader_id || '');
        // Clear errors
        setNameError('');
        setLeaderError('');
      } else {
        // Reset form when opening in create mode
        setWorkspaceName('');
        setLeaderId('');
        setApiKeyId('');
        // Clear errors
        setNameError('');
        setLeaderError('');
      }
    }
  }, [open, resolvedInitialData]);

  // Fetch organization members for workspace owner selection.
  const { members: organizationMembers, isLoading: isLoadingMembers } =
    useCurrentOrganizationMembers({
      limit: 100,
      page: 1,
      enabled: open,
    });
  // const { items: apiKeys, isLoading: isLoadingApiKeys } = useApiKeys(undefined);

  // Filter members to only include active ones
  const activeOrganizationMembers = useMemo(() => {
    if (!organizationMembers) return [];
    return organizationMembers.filter(member => member.status === 'active');
  }, [organizationMembers]);

  // Search state for leader dropdown
  const [leaderSearch, setLeaderSearch] = useState('');

  // Filter members by search keyword
  const filteredMembers = useMemo(() => {
    if (!leaderSearch.trim()) return activeOrganizationMembers;
    const keyword = leaderSearch.toLowerCase();
    return activeOrganizationMembers.filter(
      member =>
        member.name?.toLowerCase().includes(keyword) ||
        member.member_name?.toLowerCase().includes(keyword) ||
        member.email?.toLowerCase().includes(keyword)
    );
  }, [activeOrganizationMembers, leaderSearch]);

  // Find leader_id from leader_name when in edit mode
  useEffect(() => {
    if (
      !isEditMode ||
      leaderId ||
      !resolvedInitialData?.leader_name ||
      !activeOrganizationMembers
    ) {
      return;
    }

    const leader =
      activeOrganizationMembers.find(member => member.id === resolvedInitialData.leader_id) ||
      activeOrganizationMembers.find(
        member =>
          member.name === resolvedInitialData.leader_name ||
          member.member_name === resolvedInitialData.leader_name
      );

    if (leader) {
      setLeaderId(leader.id);
    }
  }, [
    isEditMode,
    leaderId,
    resolvedInitialData?.leader_id,
    resolvedInitialData?.leader_name,
    activeOrganizationMembers,
  ]);

  // Reset form when dialog closes
  const handleOpenChange = useCallback(
    (newOpen: boolean) => {
      if (!newOpen) {
        if (!initialData) {
          setWorkspaceName('');
          setLeaderId('');
          setApiKeyId('');
        }
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
            activeOrganizationMembers.find(
              m =>
                m.name === resolvedInitialData?.leader_name ||
                m.member_name === resolvedInitialData?.leader_name
            )?.id ||
            '';

          // Check if any values have changed
          const hasNameChanged = name !== (resolvedInitialData?.name || '');
          const hasLeaderChanged = leaderId !== originalLeaderId;
          const hasApiKeyChanged = apiKeyId !== (resolvedInitialData?.api_key_id || '');

          const payload: UpdateWorkspaceRequest = {};
          if (hasNameChanged) {
            payload.name = name;
          }
          if (hasLeaderChanged && leaderId) {
            payload.leader_id = leaderId;
          }
          if (hasApiKeyChanged && apiKeyId) {
            payload.api_key_id = apiKeyId;
          }

          // Skip API call if nothing changed or hidden optional fields only became empty locally
          if (Object.keys(payload).length === 0) {
            onOpenChange(false);
            return;
          }
          await onUpdate(payload);
        } else {
          const payload: CreateWorkspaceRequest = {
            name,
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
      leaderId,
      apiKeyId,
      currentOrganization?.id,
      isEditMode,
      resolvedInitialData,
      activeOrganizationMembers,
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
              <p className="text-right text-xs text-muted-foreground">
                {workspaceName.length}/30
              </p>
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
                        <SelectItem key={member.id} value={member.id} className="rounded-md">
                          <div className="flex items-center gap-3 py-1">
                            <div className="size-8 rounded-full bg-muted flex items-center justify-center shrink-0">
                              <User className="h-4 w-4 text-muted-foreground" />
                            </div>
                            <div className="flex flex-col text-start">
                              <span className="font-semibold text-sm leading-none mb-1">
                                {member.member_name || member.name}
                              </span>
                              <span className="text-[10px] text-muted-foreground tracking-tight">
                                {member.email}
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
              {isEditMode && !leaderError && (
                <p className="text-xs text-muted-foreground font-medium ml-1">
                  {t('organization.workspaceManagement.createWorkspace.leaderTransferHint')}
                </p>
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
