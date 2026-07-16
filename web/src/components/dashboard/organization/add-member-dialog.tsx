'use client';

import { useEffect, useState } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import { Input, PasswordInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { Pagination } from '@/components/ui/pagination';
import { Link2, Copy, Check, ChevronsUpDown, X } from 'lucide-react';
import { toast } from 'sonner';
import { useInviteLink } from '@/hooks/organization/use-invite-link';
import { useMemberActions } from '@/hooks/organization/use-member-actions';
import { useJoinRequests } from '@/hooks/organization/use-join-requests';
import { DepartmentTreeItemDropdown } from '@/components/dashboard/organization/department-tree-item-dropdown';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { Skeleton } from '@/components/ui/skeleton';
import { IS_CLOUD } from '@/lib/config';
import { cn } from '@/lib/utils';
import type { Department } from '@/services/types/organization';

interface AddMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  departments: Department[];
  defaultDepartmentId?: string | null;
}

const SHOW_INVITE_LINK_TAB = false;

export function AddMemberDialog({
  open,
  onOpenChange,
  departments,
  defaultDepartmentId,
}: AddMemberDialogProps) {
  const t = useT('dashboard');
  const {
    directAddMember,
    isAddingMember,
    adminRegisterMember,
    isAdminRegisteringMember,
    approveJoinRequest,
    isApprovingRequest,
    rejectJoinRequest,
    isRejectingRequest,
  } = useMemberActions();
  const [activeTab, setActiveTab] = useState(SHOW_INVITE_LINK_TAB ? 'invite-link' : 'direct-add');
  const [copied, setCopied] = useState(false);
  const [pendingPage, setPendingPage] = useState(1);
  const [pendingPageSize] = useState(10);

  // Fetch join requests (only when tab is active)
  const {
    data: pendingApplications,
    total: totalPending,
    isLoading: loadingPending,
  } = useJoinRequests(pendingPage, pendingPageSize, open && activeTab === 'pending');

  // Direct add form state
  const [memberName, setMemberName] = useState('');
  const [nameError, setNameError] = useState('');
  const [memberEmail, setMemberEmail] = useState('');
  const [emailError, setEmailError] = useState('');
  const [memberPassword, setMemberPassword] = useState('');
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const [selectedDepartment, setSelectedDepartment] = useState('');
  const [sendNotification, setSendNotification] = useState(false);
  const [directAddSelectOpen, setDirectAddSelectOpen] = useState(false);
  const [directAddExpandedIds, setDirectAddExpandedIds] = useState<Set<string>>(new Set());

  // Invite link department selection
  const [inviteLinkDepartmentId, setInviteLinkDepartmentId] = useState<string>('');
  const [inviteLinkSelectOpen, setInviteLinkSelectOpen] = useState(false);
  const [inviteLinkExpandedIds, setInviteLinkExpandedIds] = useState<Set<string>>(new Set());

  // Fetch invite link (only when department is selected and invite-link tab is active)
  const { inviteLink, isLoading: isLoadingInviteLink } = useInviteLink(
    inviteLinkDepartmentId === 'ORG_ROOT' ? '' : inviteLinkDepartmentId,
    activeTab === 'invite-link' && !!inviteLinkDepartmentId
  );

  // Get selected department name for display
  const getDepartmentName = (id: string): string | null => {
    const findDept = (depts: Department[], targetId: string): string | null => {
      for (const dept of depts) {
        if (dept.id === targetId) return dept.name;
        if (dept.children) {
          const found = findDept(dept.children, targetId);
          if (found) return found;
        }
      }
      return null;
    };
    return findDept(departments, id);
  };

  useEffect(() => {
    if (
      (!SHOW_INVITE_LINK_TAB && activeTab === 'invite-link') ||
      (!IS_CLOUD && activeTab !== 'direct-add')
    ) {
      setActiveTab('direct-add');
    }
  }, [activeTab]);

  useEffect(() => {
    if (!open) return;
    const nextDepartmentId = defaultDepartmentId ?? '';
    setSelectedDepartment(nextDepartmentId);
    setInviteLinkDepartmentId(nextDepartmentId);
    setSelectedWorkspace(undefined);
  }, [defaultDepartmentId, open]);

  // Expand first level by default when select opens
  const handleInviteLinkSelectOpen = (open: boolean) => {
    setInviteLinkSelectOpen(open);
    if (open && inviteLinkExpandedIds.size === 0) {
      setInviteLinkExpandedIds(new Set(departments.map(dept => dept.id)));
    }
  };

  const handleInviteLinkToggleExpand = (id: string) => {
    setInviteLinkExpandedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const handleInviteLinkSelect = (id: string) => {
    setInviteLinkDepartmentId(id);
    setInviteLinkSelectOpen(false);
  };

  // Direct add department selection handlers
  const handleDirectAddSelectOpen = (open: boolean) => {
    setDirectAddSelectOpen(open);
    if (open && directAddExpandedIds.size === 0) {
      setDirectAddExpandedIds(new Set(departments.map(dept => dept.id)));
    }
  };

  const handleDirectAddToggleExpand = (id: string) => {
    setDirectAddExpandedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const handleDirectAddSelect = (id: string) => {
    setSelectedDepartment(id);
    setDirectAddSelectOpen(false);
  };

  // Copy invite link
  const handleCopyLink = async () => {
    if (!inviteLink) return;
    try {
      await navigator.clipboard.writeText(inviteLink);
      setCopied(true);
      toast.success(t('organization.contacts.addMember.linkCopied'));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error(t('organization.contacts.addMember.copyFailed'));
    }
  };

  // Validate email format
  const validateEmail = (email: string): boolean => {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
  };

  // Handle direct add member
  const handleDirectAdd = async () => {
    const trimmedName = memberName.trim();
    const trimmedEmail = memberEmail.trim();
    const workspaceId = selectedWorkspace?.id;
    let hasError = false;

    if (!trimmedName) {
      setNameError(t('organization.validation.nameRequired'));
      hasError = true;
    } else if (trimmedName.length > 50) {
      setNameError(t('organization.validation.nameTooLong', { max: 50 }));
      hasError = true;
    }

    if (!trimmedEmail) {
      setEmailError(t('organization.validation.emailRequired'));
      hasError = true;
    } else if (trimmedEmail.length > 100) {
      setEmailError(t('organization.validation.nameTooLong', { max: 100 }));
      hasError = true;
    } else if (!validateEmail(trimmedEmail)) {
      setEmailError(t('organization.validation.invalidEmail'));
      hasError = true;
    }

    if (IS_CLOUD && !selectedDepartment) {
      toast.error(t('organization.contacts.addMember.selectDepartment'));
      hasError = true;
    }

    if (hasError) return;

    try {
      // If organization root is selected, pass empty string or handle as appropriate
      const deptId = selectedDepartment === 'ORG_ROOT' ? '' : selectedDepartment;

      if (IS_CLOUD) {
        await directAddMember({
          name: trimmedName,
          email: trimmedEmail,
          ...(workspaceId ? { workspace_id: workspaceId } : {}),
          department_id: deptId,
          send_email: sendNotification,
        });
      } else {
        const trimmedPassword = memberPassword.trim();
        await adminRegisterMember({
          name: trimmedName,
          email: trimmedEmail,
          ...(workspaceId ? { workspace_id: workspaceId } : {}),
          ...(trimmedPassword ? { password: trimmedPassword } : {}),
          ...(deptId ? { department_id: deptId } : {}),
        });
      }

      // Reset form
      setMemberName('');
      setNameError('');
      setMemberEmail('');
      setEmailError('');
      setMemberPassword('');
      setSelectedWorkspace(undefined);
      setSelectedDepartment('');
      setSendNotification(false);
      onOpenChange(false);
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to add member:', error);
    }
  };

  // Handle approve application
  const handleApprove = async (requestId: string) => {
    try {
      await approveJoinRequest(requestId);
      // Reset to page 1 if current page becomes empty
      if (pendingApplications.length === 1 && pendingPage > 1) {
        setPendingPage(1);
      }
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to approve request:', error);
    }
  };

  // Handle reject application
  const handleReject = async (requestId: string) => {
    try {
      await rejectJoinRequest(requestId);
      // Reset to page 1 if current page becomes empty
      if (pendingApplications.length === 1 && pendingPage > 1) {
        setPendingPage(1);
      }
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to reject request:', error);
    }
  };

  // Handle batch approve
  const handleBatchApprove = async () => {
    // TODO: Call API to batch approve
    // For now, approve all requests one by one
    try {
      for (const application of pendingApplications) {
        await approveJoinRequest(application.id);
      }
      // Reset to page 1 after batch operation
      setPendingPage(1);
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to batch approve:', error);
    }
  };

  // Get status text based on status
  const getStatusText = (status: string): string => {
    switch (status) {
      case 'pending':
        return t('organization.contacts.addMember.statusPending');
      case 'approved':
        return t('organization.contacts.addMember.statusApproved');
      case 'rejected':
        return t('organization.contacts.addMember.statusRejected');
      case 'expired':
        return t('organization.contacts.addMember.statusExpired');
      default:
        return status;
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl p-0 overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('organization.contacts.addMember.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="p-0 flex flex-col overflow-visible">
          <Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1 flex flex-col">
            {IS_CLOUD && (
              <div className="px-6 pt-4">
                <TabsList
                  className={cn(
                    'grid w-full bg-muted/50 p-1 rounded-xl h-12',
                    SHOW_INVITE_LINK_TAB ? 'grid-cols-3' : 'grid-cols-2'
                  )}
                >
                  {SHOW_INVITE_LINK_TAB && (
                    <TabsTrigger
                      value="invite-link"
                      className="rounded-lg data-[state=active]:bg-card data-[state=active]:shadow-sm font-bold transition-all"
                    >
                      {t('organization.contacts.addMember.inviteLink')}
                    </TabsTrigger>
                  )}
                  <TabsTrigger
                    value="direct-add"
                    className="rounded-lg data-[state=active]:bg-white data-[state=active]:shadow-sm font-bold transition-all"
                  >
                    {t('organization.contacts.addMember.directAdd')}
                  </TabsTrigger>
                  <TabsTrigger
                    value="pending"
                    className="rounded-lg data-[state=active]:bg-white data-[state=active]:shadow-sm font-bold transition-all relative"
                  >
                    {t('organization.contacts.addMember.pending')}
                  </TabsTrigger>
                </TabsList>
              </div>
            )}

            <div className="flex-1 overflow-y-visible px-6 py-6">
              {/* Tab 1: Invite Link */}
              {SHOW_INVITE_LINK_TAB && (
                <TabsContent
                  value="invite-link"
                  className="mt-0 space-y-6 animate-in fade-in duration-300"
                >
                  <div className="flex items-start gap-4 p-5 bg-brand-subtle/50 rounded-2xl border border-brand-main/20">
                    <div className="p-2 bg-brand-main/20 rounded-xl text-brand-main">
                      <Link2 className="h-5 w-5" />
                    </div>
                    <div className="flex-1">
                      <h4 className="font-bold text-brand-strong mb-1">
                        {t('organization.contacts.addMember.useInviteLink')}
                      </h4>
                      <p className="text-sm text-brand-strong/70 font-medium leading-relaxed">
                        {t('organization.contacts.addMember.inviteLinkDescription')}
                      </p>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label
                      htmlFor="invite-link-department"
                      className="text-sm font-bold text-foreground ml-1"
                    >
                      {t('organization.contacts.addMember.memberDepartment')}
                    </Label>
                    <div className="relative">
                      <button
                        type="button"
                        id="invite-link-department"
                        onClick={() => handleInviteLinkSelectOpen(!inviteLinkSelectOpen)}
                        className={cn(
                          'flex h-12 w-full items-center justify-between rounded-xl border bg-card px-4 py-2 text-sm font-medium transition-all duration-200',
                          inviteLinkSelectOpen
                            ? 'border-brand-main ring-2 ring-brand-main/10'
                            : 'border hover:border-strong hover:bg-muted'
                        )}
                      >
                        <span className="truncate">
                          {inviteLinkDepartmentId
                            ? getDepartmentName(inviteLinkDepartmentId) ||
                              t('organization.contacts.addMember.selectDepartment')
                            : t('organization.contacts.addMember.selectDepartment')}
                        </span>
                        <ChevronsUpDown className="h-4 w-4 opacity-40 shrink-0" />
                      </button>
                      {inviteLinkSelectOpen && (
                        <>
                          <div
                            className="fixed inset-0 z-40"
                            onClick={() => handleInviteLinkSelectOpen(false)}
                          />
                          <div className="absolute z-50 mt-2 w-full max-h-[300px] overflow-y-auto rounded-xl border bg-popover shadow-premium animate-in fade-in slide-in-from-top-2 duration-200">
                            <div className="p-2">
                              {departments.map(dept => (
                                <DepartmentTreeItemDropdown
                                  key={dept.id}
                                  department={dept}
                                  level={0}
                                  selectedId={inviteLinkDepartmentId}
                                  onSelect={handleInviteLinkSelect}
                                  expandedIds={inviteLinkExpandedIds}
                                  onToggleExpand={handleInviteLinkToggleExpand}
                                  showCheckIcon
                                />
                              ))}
                            </div>
                          </div>
                        </>
                      )}
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label className="text-sm font-bold text-foreground ml-1">
                      {t('organization.contacts.addMember.inviteLinkLabel')}
                    </Label>
                    <div className="flex gap-3 grow">
                      <Input
                        value={
                          isLoadingInviteLink
                            ? t('organization.contacts.addMember.loading')
                            : inviteLink
                        }
                        readOnly
                        root
                        className="flex-1 h-full rounded-xl border font-medium"
                        disabled={isLoadingInviteLink || !inviteLinkDepartmentId}
                        placeholder={
                          !inviteLinkDepartmentId
                            ? t('organization.contacts.addMember.selectDepartment')
                            : ''
                        }
                      />
                      <Button
                        onClick={handleCopyLink}
                        variant="outline"
                        className="shrink-0 rounded-xl px-6 font-bold border hover:bg-muted transition-all active:scale-95"
                        disabled={isLoadingInviteLink || !inviteLink}
                      >
                        {copied ? (
                          <>
                            <Check className="h-4 w-4 text-success" />
                            {t('organization.contacts.addMember.copied')}
                          </>
                        ) : (
                          <>
                            <Copy className="h-4 w-4" />
                            {t('organization.contacts.addMember.copy')}
                          </>
                        )}
                      </Button>
                    </div>
                  </div>
                </TabsContent>
              )}

              {/* Tab 2: Direct Add */}
              <TabsContent
                value="direct-add"
                className="mt-0 space-y-6 animate-in fade-in duration-300"
              >
                <div className="space-y-2">
                  <Label htmlFor="member-name" className="text-sm font-bold text-foreground ml-1">
                    {t('organization.contacts.addMember.memberName')}
                  </Label>
                  <Input
                    id="member-name"
                    placeholder={t('organization.contacts.addMember.memberNamePlaceholder')}
                    value={memberName}
                    onChange={e => {
                      setMemberName(e.target.value);
                      if (nameError) setNameError('');
                    }}
                    maxLength={50}
                    errorText={nameError}
                    className="h-12 rounded-xl"
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="member-email" className="text-sm font-bold text-foreground ml-1">
                    {t('organization.contacts.addMember.memberEmail')}
                  </Label>
                  <Input
                    id="member-email"
                    type="email"
                    placeholder={t('organization.contacts.addMember.memberEmailPlaceholder')}
                    value={memberEmail}
                    onChange={e => {
                      setMemberEmail(e.target.value);
                      if (emailError) setEmailError('');
                    }}
                    maxLength={100}
                    errorText={emailError}
                    className="h-12 rounded-xl"
                  />
                </div>

                {!IS_CLOUD && (
                  <div className="space-y-2">
                    <Label
                      htmlFor="member-password"
                      className="text-sm font-bold text-foreground ml-1"
                    >
                      {t('organization.contacts.addMember.memberPassword')}
                    </Label>
                    <PasswordInput
                      id="member-password"
                      placeholder={t('organization.contacts.addMember.memberPasswordPlaceholder')}
                      value={memberPassword}
                      onChange={e => setMemberPassword(e.target.value)}
                      autoComplete="new-password"
                      className="h-12 rounded-xl"
                    />
                    <p className="text-xs font-medium text-muted-foreground ml-1">
                      {t('organization.contacts.addMember.defaultPasswordHint')}
                    </p>
                  </div>
                )}

                <div className="space-y-2">
                  <Label className="text-sm font-bold text-foreground ml-1">
                    {t('organization.contacts.addMember.memberWorkspaceOptional')}
                  </Label>
                  <div className="flex gap-2">
                    <WorkspaceSelector
                      value={selectedWorkspace}
                      onChange={setSelectedWorkspace}
                      placeholder={t('organization.contacts.addMember.workspaceOptionalPlaceholder')}
                      className="h-12 rounded-xl"
                    />
                    {selectedWorkspace ? (
                      <Button
                        type="button"
                        variant="outline"
                        isIcon
                        onClick={() => setSelectedWorkspace(undefined)}
                        className="h-12 w-12 shrink-0 rounded-xl"
                        aria-label={t('organization.contacts.addMember.clearWorkspace')}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    ) : null}
                  </div>
                  <p className="ml-1 text-xs font-medium text-muted-foreground">
                    {t('organization.contacts.addMember.workspaceOptionalHint')}
                  </p>
                </div>

                <div className="space-y-2">
                  <Label
                    htmlFor="member-department"
                    className="text-sm font-bold text-foreground ml-1"
                  >
                    {IS_CLOUD
                      ? t('organization.contacts.addMember.memberDepartment')
                      : t('organization.contacts.addMember.memberDepartmentOptional')}
                  </Label>
                  <div className="relative">
                    <button
                      type="button"
                      id="member-department"
                      onClick={() => handleDirectAddSelectOpen(!directAddSelectOpen)}
                      className={cn(
                        'flex h-12 w-full items-center justify-between rounded-xl border bg-card px-4 py-2 text-sm font-medium transition-all duration-200',
                        directAddSelectOpen
                          ? 'border-brand-main ring-2 ring-brand-main/10'
                          : 'border hover:border-strong hover:bg-muted'
                      )}
                    >
                      <span className="truncate">
                        {selectedDepartment
                          ? getDepartmentName(selectedDepartment) ||
                            t('organization.contacts.addMember.selectDepartment')
                          : t('organization.contacts.addMember.selectDepartment')}
                      </span>
                      <ChevronsUpDown className="h-4 w-4 opacity-40 shrink-0" />
                    </button>
                    {directAddSelectOpen && (
                      <>
                        <div
                          className="fixed inset-0 z-40"
                          onClick={() => handleDirectAddSelectOpen(false)}
                        />
                        <div className="absolute z-50 mt-2 w-full max-h-[300px] overflow-y-auto rounded-xl border bg-popover shadow-premium animate-in fade-in slide-in-from-top-2 duration-200">
                          <div className="p-2">
                            {departments.map(dept => (
                              <DepartmentTreeItemDropdown
                                key={dept.id}
                                department={dept}
                                level={0}
                                selectedId={selectedDepartment}
                                onSelect={handleDirectAddSelect}
                                expandedIds={directAddExpandedIds}
                                onToggleExpand={handleDirectAddToggleExpand}
                                showCheckIcon
                              />
                            ))}
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                </div>

                {IS_CLOUD && (
                  <div className="flex items-center space-x-3 p-4 bg-muted rounded-2xl border transition-colors hover:bg-accent cursor-pointer group">
                    <Checkbox
                      id="send-notification"
                      checked={sendNotification}
                      onCheckedChange={checked => setSendNotification(checked as boolean)}
                      className="h-5 w-5 rounded-md border-muted-foreground data-[state=checked]:bg-primary data-[state=checked]:border-primary"
                    />
                    <Label
                      htmlFor="send-notification"
                      className="text-sm font-bold text-muted-foreground cursor-pointer select-none grow"
                    >
                      {t('organization.contacts.addMember.sendNotification')}
                    </Label>
                  </div>
                )}
              </TabsContent>

              {/* Tab 3: Pending Applications */}
              {IS_CLOUD && (
                <TabsContent
                  value="pending"
                  className="mt-0 h-full animate-in fade-in duration-300"
                >
                  {loadingPending ? (
                    <div className="space-y-4">
                      {[...Array(3)].map((_, i) => (
                        <div
                          key={i}
                          className="flex items-center justify-between p-5 border rounded-2xl bg-card"
                        >
                          <div className="flex items-center gap-4 flex-1">
                            <Skeleton className="h-10 w-10 rounded-xl" />
                            <div className="flex-1 space-y-2">
                              <Skeleton className="h-4 w-32" />
                              <Skeleton className="h-3 w-48" />
                            </div>
                          </div>
                          <Skeleton className="h-9 w-24 rounded-lg" />
                        </div>
                      ))}
                    </div>
                  ) : pendingApplications.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-20 text-center">
                      <div className="p-4 bg-muted rounded-full mb-4">
                        <Check className="size-8 text-muted-foreground/50" />
                      </div>
                      <p className="text-sm font-bold text-muted-foreground max-w-[200px]">
                        {t('organization.contacts.addMember.noPendingApplications')}
                      </p>
                    </div>
                  ) : (
                    <div className="flex flex-col h-full space-y-6">
                      <div className="flex-1 space-y-4">
                        {pendingApplications.map(application => {
                          const isPending = application.status === 'pending';
                          return (
                            <div
                              key={application.id}
                              className="flex items-center justify-between p-5 border rounded-2xl bg-card hover:border-brand-main/20 hover:shadow-sm transition-all duration-200"
                            >
                              <div className="flex items-center gap-4">
                                <div className="h-10 w-10 rounded-xl bg-muted flex items-center justify-center font-bold text-muted-foreground uppercase">
                                  {application.account_name.charAt(0)}
                                </div>
                                <div>
                                  <p className="font-bold text-foreground">
                                    {application.account_name}
                                  </p>
                                  <p className="text-xs font-medium text-muted-foreground mt-0.5">
                                    {application.account_email} • {application.department_name}
                                  </p>
                                </div>
                              </div>
                              <div className="flex items-center gap-2">
                                {isPending ? (
                                  <div className="flex gap-2">
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => handleReject(application.id)}
                                      disabled={isRejectingRequest || isApprovingRequest}
                                      className="rounded-lg h-9 px-4 font-bold text-muted-foreground hover:text-destructive hover:bg-destructive/5"
                                    >
                                      {t('organization.contacts.addMember.reject')}
                                    </Button>
                                    <Button
                                      size="sm"
                                      onClick={() => handleApprove(application.id)}
                                      disabled={isRejectingRequest || isApprovingRequest}
                                      className="h-9 rounded-lg px-4 font-bold shadow-sm disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none"
                                    >
                                      {t('organization.contacts.addMember.approve')}
                                    </Button>
                                  </div>
                                ) : (
                                  <span className="text-[10px] font-bold uppercase tracking-wider bg-muted text-muted-foreground px-2 py-1 rounded-md">
                                    {getStatusText(application.status)}
                                  </span>
                                )}
                              </div>
                            </div>
                          );
                        })}
                      </div>

                      {/* Pagination */}
                      {totalPending > pendingPageSize && (
                        <div className="pt-4 border-t border-dashed">
                          <Pagination
                            currentPage={pendingPage}
                            totalPages={Math.ceil(totalPending / pendingPageSize)}
                            total={totalPending}
                            pageSize={pendingPageSize}
                            onPageChange={setPendingPage}
                            showInfo
                            showJump={false}
                          />
                        </div>
                      )}
                    </div>
                  )}
                </TabsContent>
              )}
            </div>
          </Tabs>
        </DialogBody>

        <DialogFooter className="bg-muted/50 pt-4 pb-6 px-6 border-t gap-3">
          {activeTab === 'pending' ? (
            <div className="flex w-full justify-between items-center">
              <span className="text-sm font-medium text-muted-foreground italic">
                {totalPending > 0
                  ? t('organization.contacts.addMember.pendingApplicationsFound', {
                      total: totalPending,
                    })
                  : t('organization.contacts.addMember.noPendingApplicationsFooter')}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="ghost"
                  size="xl"
                  onClick={() => onOpenChange(false)}
                  className="px-6 font-semibold"
                >
                  {t('organization.contacts.addMember.cancel')}
                </Button>
                <Button
                  size="xl"
                  onClick={handleBatchApprove}
                  className="px-6 font-semibold disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none"
                  disabled={
                    isApprovingRequest ||
                    pendingApplications.filter(app => app.status === 'pending').length === 0
                  }
                >
                  {t('organization.contacts.addMember.batchApprove')}
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex gap-2 w-full justify-end">
              <Button
                variant="ghost"
                size="xl"
                onClick={() => onOpenChange(false)}
                className="px-6 font-semibold"
              >
                {t('organization.contacts.addMember.cancel')}
              </Button>
              <Button
                size="xl"
                onClick={handleDirectAdd}
                disabled={
                  activeTab === 'direct-add' &&
                  (!memberName ||
                    !memberEmail ||
                    (IS_CLOUD && !selectedDepartment) ||
                    isAddingMember ||
                    isAdminRegisteringMember)
                }
                className={cn(
                  'px-8 font-semibold shadow-premium transition-all active:scale-95 disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none',
                  activeTab === 'invite-link' && 'hidden'
                )}
              >
                {isAddingMember || isAdminRegisteringMember
                  ? t('organization.contacts.addMember.adding')
                  : t('organization.contacts.addMember.confirmAdd')}
              </Button>
            </div>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
