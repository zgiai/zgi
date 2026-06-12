'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { Search, Save, ShieldCheck, UserPlus, X } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
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
  const {
    members: roleMembers,
    isLoading: isLoadingRoleMembers,
    isFetching: isFetchingRoleMembers,
  } = useCurrentOrganizationMembers({
    limit: 1000,
    enabled: isOwner,
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
  const isFetchingMembers = isFetchingRoleMembers || isFetchingCandidateMembers;

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

  const isDirty = name.trim() !== (currentOrganization?.name ?? '');
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
    if (!canEdit || !validate()) {
      return;
    }
    await updateOrganization({
      name: name.trim(),
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
    <div className="flex min-w-0 items-center gap-3">
      <SafeAvatar
        src={member.avatar_url || member.avatar}
        alt={member.name}
        fallback={member.name || member.email}
        size="md"
      />
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-foreground">{member.name}</div>
        <div className="truncate text-xs text-muted-foreground">{member.email}</div>
      </div>
    </div>
  );

  if (isLoading && !currentOrganization) {
    return (
      <div className="flex h-full flex-col space-y-5 overflow-hidden bg-bg-canvas/50 p-4 text-foreground lg:p-6">
        <div className="shrink-0 space-y-2">
          <Skeleton className="h-8 w-40 rounded-lg" />
          <Skeleton className="h-4 w-80 rounded" />
        </div>
        <Skeleton className="h-64 rounded-xl" />
        <Skeleton className="min-h-0 flex-1 rounded-xl" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col space-y-5 overflow-auto bg-bg-canvas/50 p-4 text-foreground lg:p-6">
      <header className="shrink-0">
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary">
          {t('organization.settings.title')}
        </h1>
        <p className="mt-1 max-w-2xl text-sm text-text-secondary">
          {t('organization.settings.subtitle')}
        </p>
      </header>

      <section className="shrink-0 overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm">
        <div className="border-b border-border/60 px-5 py-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-text-primary">
                {t('organization.settings.profileTitle')}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('organization.settings.profileDescription')}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {t('organization.settings.permissionHint')}
              </p>
            </div>
            <Badge variant={canEdit ? 'info' : 'secondary'} className="w-fit font-medium">
              {t('organization.settings.currentRole')}: {currentRoleLabel}
            </Badge>
          </div>
        </div>

        <form className="space-y-5 p-5" onSubmit={handleSubmit}>
          <div className="max-w-2xl space-y-2.5">
            <Label htmlFor="organization-name">{t('organization.settings.name')}</Label>
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
              className="h-10 rounded-md bg-bg-canvas/50 shadow-none transition-all focus:border-primary/40 focus:ring-0"
            />
          </div>

          {!canEdit ? (
            <div className="max-w-2xl rounded-lg border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-warning-foreground">
              {t('organization.settings.noPermission')}
            </div>
          ) : null}

          <div className="flex justify-end border-t border-border/60 pt-4">
            <Button
              type="submit"
              disabled={!canEdit || !isDirty || isUpdatingOrganization}
              className="h-10 gap-2 rounded-md px-4 font-medium"
            >
              <Save className="size-4" />
              {isUpdatingOrganization
                ? t('organization.settings.saving')
                : saveFeedbackVisible
                  ? t('organization.settings.saved')
                  : t('organization.settings.save')}
            </Button>
          </div>
        </form>
      </section>

      {isOwner ? (
        <section className="min-h-[420px] overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm">
          <div className="flex flex-col gap-3 border-b border-border/60 px-5 py-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <h2 className="flex items-center gap-2 text-lg font-semibold tracking-tight text-text-primary">
                <ShieldCheck className="size-5 shrink-0 text-primary" />
                {t('organization.settings.adminManagement.title')}
              </h2>
              <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
                {t('organization.settings.adminManagement.description')}
              </p>
            </div>
            <div className="flex items-center gap-3">
              {isFetchingMembers ? (
                <span className="text-xs text-muted-foreground">
                  {t('organization.settings.adminManagement.refreshing')}
                </span>
              ) : null}
              <Badge variant="info" className="font-medium">
                {t('organization.settings.adminManagement.adminCount', {
                  count: admins.length,
                })}
              </Badge>
            </div>
          </div>

          <div className="grid min-h-0 gap-0 lg:grid-cols-[minmax(0,1fr)_minmax(340px,420px)]">
            <div className="space-y-6 p-5">
              <div className="space-y-3">
                <div className="text-sm font-semibold text-foreground">
                  {t('organization.settings.adminManagement.ownerTitle')}
                </div>
                <div className="overflow-hidden rounded-lg border border-border/80">
                  {owners.length > 0 ? (
                    owners.map(owner => (
                      <div
                        key={owner.id}
                        className="flex items-center justify-between gap-3 bg-background px-4 py-3"
                      >
                        {renderMemberIdentity(owner)}
                        <Badge variant="warning" className="shrink-0 font-medium">
                          {t('organization.settings.adminManagement.ownerBadge')}
                        </Badge>
                      </div>
                    ))
                  ) : (
                    <div className="px-4 py-6 text-sm text-muted-foreground">
                      {t('organization.settings.adminManagement.noOwner')}
                    </div>
                  )}
                </div>
              </div>

              <div className="space-y-3">
                <div className="text-sm font-semibold text-foreground">
                  {t('organization.settings.adminManagement.adminTitle')}
                </div>
                <div className="overflow-hidden rounded-lg border border-border/80">
                  {isLoadingRoleMembers ? (
                    <>
                      <div className="border-b border-border/60 px-4 py-3">
                        <Skeleton className="h-10 rounded-lg" />
                      </div>
                      <div className="px-4 py-3">
                        <Skeleton className="h-10 rounded-lg" />
                      </div>
                    </>
                  ) : admins.length > 0 ? (
                    admins.map(admin => (
                      <div
                        key={admin.id}
                        className="flex flex-col gap-3 border-b border-border/60 bg-background px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between"
                      >
                        {renderMemberIdentity(admin)}
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="h-8 gap-2 self-start rounded-md sm:self-auto"
                          disabled={isUpdatingCurrentOrganizationMemberRole}
                          onClick={() => setAdminToDemote(admin)}
                        >
                          <X className="size-4" />
                          {t('organization.settings.adminManagement.demote')}
                        </Button>
                      </div>
                    ))
                  ) : (
                    <div className="px-4 py-6 text-sm text-muted-foreground">
                      {t('organization.settings.adminManagement.emptyAdmins')}
                    </div>
                  )}
                </div>
              </div>
            </div>

            <aside className="border-t border-border/60 bg-muted/20 p-5 lg:border-l lg:border-t-0">
              <div className="space-y-4">
                <Label htmlFor="organization-admin-search">
                  {t('organization.settings.adminManagement.addTitle')}
                </Label>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    id="organization-admin-search"
                    value={memberKeyword}
                    onChange={event => setMemberKeyword(event.target.value)}
                    placeholder={t('organization.settings.adminManagement.searchPlaceholder')}
                    className="h-10 rounded-md bg-background pl-9 shadow-none transition-all focus:border-primary/40 focus:ring-0"
                  />
                </div>

                <div className="overflow-hidden rounded-lg border border-border/80 bg-background">
                  {isLoadingCandidateMembers ? (
                    <>
                      <div className="border-b border-border/60 px-4 py-3">
                        <Skeleton className="h-10 rounded-lg" />
                      </div>
                      <div className="px-4 py-3">
                        <Skeleton className="h-10 rounded-lg" />
                      </div>
                    </>
                  ) : candidateMembers.length > 0 ? (
                    candidateMembers.map(member => (
                      <div
                        key={member.id}
                        className="flex flex-col gap-3 border-b border-border/60 px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between lg:flex-col lg:items-stretch xl:flex-row xl:items-center"
                      >
                        {renderMemberIdentity(member)}
                        <Button
                          type="button"
                          size="sm"
                          className="h-8 gap-2 self-start rounded-md sm:self-auto lg:self-start xl:self-auto"
                          disabled={isUpdatingCurrentOrganizationMemberRole}
                          onClick={() => handlePromoteAdmin(member)}
                        >
                          <UserPlus className="size-4" />
                          {t('organization.settings.adminManagement.promote')}
                        </Button>
                      </div>
                    ))
                  ) : (
                    <div className="px-4 py-8 text-sm text-muted-foreground">
                      {t('organization.settings.adminManagement.emptyCandidates')}
                    </div>
                  )}
                </div>
              </div>
            </aside>
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
