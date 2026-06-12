'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { Building2, Search, Save, ShieldCheck, UserPlus, X } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { SafeAvatar } from '@/components/ui/avatar';
import { useCurrentOrganizationMembers } from '@/hooks/organization/use-current-organization-members';
import { useOrganizationActions } from '@/hooks/organization/use-organization-actions';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import type { Member } from '@/services/types/organization';

const ORGANIZATION_NAME_MAX_LENGTH = 255;

export default function OrganizationSettingsPage() {
  const t = useT('dashboard');
  const { currentOrganization, isLoading } = useOrganizations(true);
  const {
    updateOrganization,
    isUpdatingOrganization,
    updateCurrentOrganizationMemberRole,
    isUpdatingCurrentOrganizationMemberRole,
  } = useOrganizationActions();
  const [name, setName] = useState('');
  const [nameError, setNameError] = useState('');
  const [saveFeedbackVisible, setSaveFeedbackVisible] = useState(false);
  const [memberKeyword, setMemberKeyword] = useState('');
  const [adminToDemote, setAdminToDemote] = useState<Member | null>(null);
  const saveFeedbackTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const debouncedMemberKeyword = useDebouncedValue(memberKeyword, 300);

  const canEdit = useMemo(
    () => ['owner', 'admin'].includes(currentOrganization?.organization_role ?? ''),
    [currentOrganization?.organization_role]
  );
  const isOwner = currentOrganization?.organization_role === 'owner';
  const canViewAdminManagement = ['owner', 'admin'].includes(
    currentOrganization?.organization_role ?? ''
  );
  const {
    members: roleMembers,
    isLoading: isLoadingRoleMembers,
    isFetching: isFetchingRoleMembers,
  } = useCurrentOrganizationMembers({
    limit: 1000,
    enabled: canViewAdminManagement,
  });
  const {
    members: candidateMemberResults,
    isLoading: isLoadingCandidateMembers,
    isFetching: isFetchingCandidateMembers,
  } = useCurrentOrganizationMembers({
    keyword: debouncedMemberKeyword,
    limit: 1000,
    enabled: isOwner,
  });

  const owners = useMemo(
    () => roleMembers.filter(member => member.organization_role === 'owner'),
    [roleMembers]
  );
  const admins = useMemo(
    () => roleMembers.filter(member => member.organization_role === 'admin'),
    [roleMembers]
  );
  const candidateMembers = useMemo(
    () =>
      candidateMemberResults
        .filter(member => member.organization_role === 'normal' && member.status === 'active')
        .slice(0, 8),
    [candidateMemberResults]
  );
  const isFetchingMembers = isFetchingRoleMembers || (isOwner && isFetchingCandidateMembers);

  useEffect(() => {
    setName(currentOrganization?.name ?? '');
    setNameError('');
  }, [currentOrganization?.id, currentOrganization?.name]);

  useEffect(() => {
    return () => {
      if (saveFeedbackTimerRef.current) {
        clearTimeout(saveFeedbackTimerRef.current);
      }
    };
  }, []);

  const trimmedName = name.trim();
  const isNameEmpty = trimmedName.length === 0;
  const isNameTooLong = trimmedName.length > ORGANIZATION_NAME_MAX_LENGTH;
  const isDirty = trimmedName !== (currentOrganization?.name ?? '');
  const saveDisabledReason = useMemo(() => {
    if (isUpdatingOrganization) {
      return t('organization.settings.saveDisabledReasons.saving');
    }
    if (!canEdit) {
      return t('organization.settings.saveDisabledReasons.noPermission');
    }
    if (isNameEmpty) {
      return t('organization.settings.saveDisabledReasons.nameRequired');
    }
    if (isNameTooLong) {
      return t('organization.settings.saveDisabledReasons.nameTooLong', {
        max: ORGANIZATION_NAME_MAX_LENGTH,
      });
    }
    if (!isDirty) {
      return t('organization.settings.saveDisabledReasons.noChanges');
    }
    return '';
  }, [canEdit, isDirty, isNameEmpty, isNameTooLong, isUpdatingOrganization, t]);
  const isSaveDisabled = Boolean(saveDisabledReason);
  const currentRoleLabel = useMemo(() => {
    switch (currentOrganization?.organization_role) {
      case 'owner':
        return t('organization.settings.roles.owner');
      case 'admin':
        return t('organization.settings.roles.admin');
      case 'normal':
        return t('organization.settings.roles.normal');
      default:
        return '-';
    }
  }, [currentOrganization?.organization_role, t]);

  const validate = () => {
    const nextName = name.trim();
    if (!nextName) {
      setNameError(t('organization.validation.nameRequired'));
      return false;
    }
    if (nextName.length > ORGANIZATION_NAME_MAX_LENGTH) {
      setNameError(t('organization.validation.nameTooLong', { max: ORGANIZATION_NAME_MAX_LENGTH }));
      return false;
    }
    return true;
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setNameError('');
    if (!canEdit || !isDirty || !validate()) {
      return;
    }
    await updateOrganization({
      name: trimmedName,
    });
    setSaveFeedbackVisible(true);
    if (saveFeedbackTimerRef.current) {
      clearTimeout(saveFeedbackTimerRef.current);
    }
    saveFeedbackTimerRef.current = setTimeout(() => {
      setSaveFeedbackVisible(false);
    }, 1800);
  };

  const handlePromoteAdmin = async (member: Member) => {
    await updateCurrentOrganizationMemberRole({ memberId: member.id, role: 'admin' });
  };

  const handleConfirmDemoteAdmin = async () => {
    if (!adminToDemote) return;
    await updateCurrentOrganizationMemberRole({ memberId: adminToDemote.id, role: 'normal' });
    setAdminToDemote(null);
  };

  const renderMemberIdentity = (member: Member) => (
    <div className="flex min-w-0 items-center gap-2.5">
      <SafeAvatar
        src={member.avatar_url || member.avatar}
        alt={member.name}
        fallback={member.name || member.email}
        size="sm"
        className="rounded-[4px]"
      />
      <div className="min-w-0">
        <div className="truncate text-[13px] font-semibold text-foreground">{member.name}</div>
        <div className="truncate text-xs text-muted-foreground">{member.email}</div>
      </div>
    </div>
  );

  if (isLoading && !currentOrganization) {
    return (
      <div className="flex h-full flex-col gap-4 overflow-hidden bg-bg-canvas/50 p-4 text-foreground lg:p-6">
        <div className="shrink-0 space-y-2">
          <Skeleton className="h-7 w-40 rounded-[4px]" />
          <Skeleton className="h-4 w-80 rounded" />
        </div>
        <Skeleton className="h-52 rounded-[6px]" />
        <Skeleton className="min-h-0 flex-1 rounded-[6px]" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto bg-bg-canvas/50 p-4 text-foreground lg:p-6">
      <header className="flex shrink-0 flex-col gap-1 border-b border-border/60 pb-4">
        <h1 className="text-[22px] font-semibold leading-7 text-text-primary">
          {t('organization.settings.title')}
        </h1>
        <p className="mt-1 max-w-2xl text-sm text-text-secondary">
          {t('organization.settings.subtitle')}
        </p>
      </header>

      <section className="shrink-0 overflow-hidden rounded-[6px] border border-border/80 bg-background shadow-sm">
        <div className="flex flex-col gap-3 border-b border-border/60 bg-muted/20 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-[4px] border border-primary/20 bg-primary/10 text-primary">
              <Building2 className="size-4" />
            </div>
            <div className="min-w-0">
              <h2 className="text-[15px] font-semibold leading-5 text-text-primary">
                {t('organization.settings.profileTitle')}
              </h2>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('organization.settings.profileDescription')}
              </p>
            </div>
          </div>
          <Badge variant={canEdit ? 'info' : 'secondary'} className="h-6 w-fit rounded-[3px] px-2">
            {t('organization.settings.currentRole')}: {currentRoleLabel}
          </Badge>
        </div>

        <form className="space-y-3 p-4" onSubmit={handleSubmit}>
          <div className="space-y-2">
            <Label htmlFor="organization-name" className="text-xs font-semibold text-text-primary">
              {t('organization.settings.name')}
            </Label>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
              <div className="min-w-0 flex-1">
                <Input
                  id="organization-name"
                  value={name}
                  onChange={event => {
                    setName(event.target.value);
                    setSaveFeedbackVisible(false);
                    if (nameError) setNameError('');
                  }}
                  placeholder={t('organization.settings.namePlaceholder')}
                  maxLength={ORGANIZATION_NAME_MAX_LENGTH}
                  disabled={!canEdit || isUpdatingOrganization}
                  errorText={nameError}
                  showCharacterCount
                  className="h-9 rounded-[4px] bg-bg-canvas/40 shadow-none transition-all focus:border-primary/50 focus:ring-0"
                />
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-flex sm:pt-0" tabIndex={isSaveDisabled ? 0 : -1}>
                    <Button
                      type="submit"
                      size="sm"
                      disabled={isSaveDisabled}
                      className="h-9 w-full gap-1.5 rounded-[3px] px-3 text-xs sm:w-auto"
                    >
                      <Save className="size-3.5" />
                      {isUpdatingOrganization
                        ? t('organization.settings.saving')
                        : saveFeedbackVisible
                          ? t('organization.settings.saved')
                          : t('organization.settings.save')}
                    </Button>
                  </span>
                </TooltipTrigger>
                {isSaveDisabled ? (
                  <TooltipContent side="top" align="end" className="max-w-64 text-xs">
                    {saveDisabledReason}
                  </TooltipContent>
                ) : null}
              </Tooltip>
            </div>
          </div>

          <div className="flex flex-col gap-1.5 border-t border-border/60 pt-3 sm:flex-row sm:items-center sm:justify-between">
            <span
              className={
                saveFeedbackVisible ? 'text-xs text-success' : 'text-xs text-muted-foreground'
              }
            >
              {saveFeedbackVisible
                ? t('organization.settings.saved')
                : saveDisabledReason || t('organization.settings.readyToSave')}
            </span>
            <span className="text-xs leading-5 text-muted-foreground">
              {canEdit
                ? t('organization.settings.permissionHint')
                : t('organization.settings.noPermission')}
            </span>
          </div>
        </form>
      </section>

      {canViewAdminManagement ? (
        <section className="min-h-[420px] overflow-hidden rounded-[6px] border border-border/80 bg-background shadow-sm">
          <div className="flex flex-col gap-3 border-b border-border/60 bg-muted/20 px-4 py-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="flex min-w-0 gap-3">
              <div className="flex size-8 shrink-0 items-center justify-center rounded-[4px] border border-primary/20 bg-primary/10 text-primary">
                <ShieldCheck className="size-4" />
              </div>
              <div className="min-w-0">
                <h2 className="text-[15px] font-semibold leading-5 text-text-primary">
                  {t('organization.settings.adminManagement.title')}
                </h2>
                <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
                  {t('organization.settings.adminManagement.description')}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-3">
              {isFetchingMembers ? (
                <span className="text-xs text-muted-foreground">
                  {t('organization.settings.adminManagement.refreshing')}
                </span>
              ) : null}
              <Badge variant="info" className="h-6 rounded-[3px] px-2">
                {t('organization.settings.adminManagement.adminCount', {
                  count: admins.length,
                })}
              </Badge>
            </div>
          </div>

          <div
            className={
              isOwner
                ? 'grid min-h-0 gap-0 lg:grid-cols-[minmax(0,1fr)_minmax(320px,400px)]'
                : 'grid min-h-0 gap-0'
            }
          >
            <div className="space-y-4 p-4">
              <div className="space-y-2">
                <div className="text-xs font-semibold uppercase text-muted-foreground">
                  {t('organization.settings.adminManagement.ownerTitle')}
                </div>
                <div className="overflow-hidden rounded-[4px] border border-border/80">
                  {owners.length > 0 ? (
                    owners.map(owner => (
                      <div
                        key={owner.id}
                        className="flex items-center justify-between gap-3 bg-background px-3 py-2.5"
                      >
                        {renderMemberIdentity(owner)}
                        <Badge
                          variant="warning"
                          className="h-5 shrink-0 rounded-[3px] px-1.5 text-[11px]"
                        >
                          {t('organization.settings.adminManagement.ownerBadge')}
                        </Badge>
                      </div>
                    ))
                  ) : (
                    <div className="px-3 py-5 text-xs text-muted-foreground">
                      {t('organization.settings.adminManagement.noOwner')}
                    </div>
                  )}
                </div>
              </div>

              <div className="space-y-2">
                <div className="text-xs font-semibold uppercase text-muted-foreground">
                  {t('organization.settings.adminManagement.adminTitle')}
                </div>
                <div className="overflow-hidden rounded-[4px] border border-border/80">
                  {isLoadingRoleMembers ? (
                    <>
                      <div className="border-b border-border/60 px-3 py-2.5">
                        <Skeleton className="h-8 rounded-[4px]" />
                      </div>
                      <div className="px-3 py-2.5">
                        <Skeleton className="h-8 rounded-[4px]" />
                      </div>
                    </>
                  ) : admins.length > 0 ? (
                    admins.map(admin => (
                      <div
                        key={admin.id}
                        className="flex flex-col gap-2 border-b border-border/60 bg-background px-3 py-2.5 last:border-b-0 sm:flex-row sm:items-center sm:justify-between"
                      >
                        {renderMemberIdentity(admin)}
                        {isOwner ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            className="h-[28px] gap-1.5 self-start rounded-[3px] px-2.5 sm:self-auto"
                            disabled={isUpdatingCurrentOrganizationMemberRole}
                            onClick={() => setAdminToDemote(admin)}
                          >
                            <X className="size-3.5" />
                            {t('organization.settings.adminManagement.demote')}
                          </Button>
                        ) : (
                          <Badge
                            variant="info"
                            className="h-5 shrink-0 self-start rounded-[3px] px-1.5 text-[11px] sm:self-auto"
                          >
                            {t('organization.settings.adminManagement.adminTitle')}
                          </Badge>
                        )}
                      </div>
                    ))
                  ) : (
                    <div className="px-3 py-5 text-xs text-muted-foreground">
                      {t('organization.settings.adminManagement.emptyAdmins')}
                    </div>
                  )}
                </div>
              </div>
            </div>

            {isOwner ? (
              <aside className="border-t border-border/60 bg-muted/20 p-4 lg:border-l lg:border-t-0">
                <div className="space-y-3">
                  <Label
                    htmlFor="organization-admin-search"
                    className="text-xs font-semibold text-text-primary"
                  >
                    {t('organization.settings.adminManagement.addTitle')}
                  </Label>
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      id="organization-admin-search"
                      value={memberKeyword}
                      onChange={event => setMemberKeyword(event.target.value)}
                      placeholder={t('organization.settings.adminManagement.searchPlaceholder')}
                      className="h-9 rounded-[4px] bg-background pl-8 shadow-none transition-all focus:border-primary/50 focus:ring-0"
                    />
                  </div>

                  <div className="overflow-hidden rounded-[4px] border border-border/80 bg-background">
                    {isLoadingCandidateMembers ? (
                      <>
                        <div className="border-b border-border/60 px-3 py-2.5">
                          <Skeleton className="h-8 rounded-[4px]" />
                        </div>
                        <div className="px-3 py-2.5">
                          <Skeleton className="h-8 rounded-[4px]" />
                        </div>
                      </>
                    ) : candidateMembers.length > 0 ? (
                      candidateMembers.map(member => (
                        <div
                          key={member.id}
                          className="flex flex-col gap-2 border-b border-border/60 px-3 py-2.5 last:border-b-0 sm:flex-row sm:items-center sm:justify-between lg:flex-col lg:items-stretch xl:flex-row xl:items-center"
                        >
                          {renderMemberIdentity(member)}
                          <Button
                            type="button"
                            size="sm"
                            className="h-[28px] gap-1.5 self-start rounded-[3px] px-2.5 sm:self-auto lg:self-start xl:self-auto"
                            disabled={isUpdatingCurrentOrganizationMemberRole}
                            onClick={() => handlePromoteAdmin(member)}
                          >
                            <UserPlus className="size-3.5" />
                            {t('organization.settings.adminManagement.promote')}
                          </Button>
                        </div>
                      ))
                    ) : (
                      <div className="px-3 py-6 text-xs leading-5 text-muted-foreground">
                        {t('organization.settings.adminManagement.emptyCandidates')}
                      </div>
                    )}
                  </div>
                </div>
              </aside>
            ) : null}
          </div>
        </section>
      ) : null}

      <ConfirmDialog
        open={!!adminToDemote}
        onOpenChange={open => {
          if (!open) setAdminToDemote(null);
        }}
        title={t('organization.settings.adminManagement.demoteConfirmTitle')}
        description={t('organization.settings.adminManagement.demoteConfirmDescription', {
          name: adminToDemote?.name ?? '',
        })}
        confirmText={t('organization.settings.adminManagement.demoteConfirm')}
        cancelText={t('organization.settings.adminManagement.cancel')}
        loading={isUpdatingCurrentOrganizationMemberRole}
        onConfirm={handleConfirmDemoteAdmin}
        variant="warning"
      />
    </div>
  );
}
