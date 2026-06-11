'use client';

import { useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { Building2, Search, Save, ShieldCheck, UserPlus, X } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
const ORGANIZATION_SHORT_NAME_MAX_LENGTH = 100;

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
  const [shortName, setShortName] = useState('');
  const [nameError, setNameError] = useState('');
  const [shortNameError, setShortNameError] = useState('');
  const [memberKeyword, setMemberKeyword] = useState('');
  const [adminToDemote, setAdminToDemote] = useState<Member | null>(null);
  const debouncedMemberKeyword = useDebouncedValue(memberKeyword, 300);

  const canEdit = useMemo(
    () => ['owner', 'admin'].includes(currentOrganization?.organization_role ?? ''),
    [currentOrganization?.organization_role]
  );
  const isOwner = currentOrganization?.organization_role === 'owner';
  const {
    members,
    isLoading: isLoadingMembers,
    isFetching: isFetchingMembers,
  } = useCurrentOrganizationMembers({
    keyword: debouncedMemberKeyword,
    limit: 1000,
    enabled: isOwner,
  });

  const owners = useMemo(
    () => members.filter(member => member.organization_role === 'owner'),
    [members]
  );
  const admins = useMemo(
    () => members.filter(member => member.organization_role === 'admin'),
    [members]
  );
  const candidateMembers = useMemo(
    () =>
      members
        .filter(member => member.organization_role === 'normal' && member.status === 'active')
        .slice(0, 8),
    [members]
  );

  useEffect(() => {
    setName(currentOrganization?.name ?? '');
    setShortName(currentOrganization?.short_name ?? '');
    setNameError('');
    setShortNameError('');
  }, [currentOrganization?.id, currentOrganization?.name, currentOrganization?.short_name]);

  const isDirty =
    name.trim() !== (currentOrganization?.name ?? '') ||
    shortName.trim() !== (currentOrganization?.short_name ?? '');

  const validate = () => {
    const nextName = name.trim();
    const nextShortName = shortName.trim();
    if (!nextName) {
      setNameError(t('organization.validation.nameRequired'));
      return false;
    }
    if (nextName.length > ORGANIZATION_NAME_MAX_LENGTH) {
      setNameError(t('organization.validation.nameTooLong', { max: ORGANIZATION_NAME_MAX_LENGTH }));
      return false;
    }
    if (nextShortName.length > ORGANIZATION_SHORT_NAME_MAX_LENGTH) {
      setShortNameError(
        t('organization.validation.nameTooLong', { max: ORGANIZATION_SHORT_NAME_MAX_LENGTH })
      );
      return false;
    }
    return true;
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setNameError('');
    setShortNameError('');
    if (!canEdit || !validate()) {
      return;
    }
    await updateOrganization({
      name: name.trim(),
      short_name: shortName.trim(),
    });
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
      <div className="min-h-full bg-bg-canvas px-6 py-6 text-foreground md:px-8 lg:px-10">
        <div className="mx-auto max-w-4xl space-y-5">
          <Skeleton className="h-20 rounded-lg" />
          <Skeleton className="h-80 rounded-lg" />
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-full bg-bg-canvas px-6 py-6 text-foreground md:px-8 lg:px-10">
      <div className="mx-auto max-w-4xl space-y-5">
        <header className="border-b border-border/70 pb-5">
          <div className="mb-3 flex size-10 items-center justify-center rounded-lg border border-border/80 bg-background">
            <Building2 className="size-5 text-muted-foreground" />
          </div>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">
            {t('organization.settings.title')}
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
            {t('organization.settings.subtitle')}
          </p>
        </header>

        <Card className="border-border/80 shadow-sm">
          <CardHeader>
            <CardTitle>{t('organization.settings.profileTitle')}</CardTitle>
            <CardDescription>{t('organization.settings.profileDescription')}</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-6" onSubmit={handleSubmit}>
              <div className="grid gap-5 md:grid-cols-2">
                <div className="space-y-2.5">
                  <Label htmlFor="organization-name">{t('organization.settings.name')}</Label>
                  <Input
                    id="organization-name"
                    value={name}
                    onChange={event => {
                      setName(event.target.value);
                      if (nameError) setNameError('');
                    }}
                    placeholder={t('organization.settings.namePlaceholder')}
                    maxLength={ORGANIZATION_NAME_MAX_LENGTH}
                    disabled={!canEdit || isUpdatingOrganization}
                    errorText={nameError}
                    showCharacterCount
                  />
                </div>
                <div className="space-y-2.5">
                  <Label htmlFor="organization-short-name">
                    {t('organization.settings.shortName')}
                  </Label>
                  <Input
                    id="organization-short-name"
                    value={shortName}
                    onChange={event => {
                      setShortName(event.target.value);
                      if (shortNameError) setShortNameError('');
                    }}
                    placeholder={t('organization.settings.shortNamePlaceholder')}
                    maxLength={ORGANIZATION_SHORT_NAME_MAX_LENGTH}
                    disabled={!canEdit || isUpdatingOrganization}
                    errorText={shortNameError}
                    showCharacterCount
                  />
                </div>
              </div>

              <div className="rounded-lg border border-border/80 bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
                <span>{t('organization.settings.currentRole')}: </span>
                <span className="font-medium text-foreground">
                  {currentOrganization?.organization_role ?? '-'}
                </span>
              </div>

              {!canEdit ? (
                <div className="rounded-lg border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-warning-foreground">
                  {t('organization.settings.noPermission')}
                </div>
              ) : null}

              <div className="flex justify-end">
                <Button
                  type="submit"
                  disabled={!canEdit || !isDirty || isUpdatingOrganization}
                  className="gap-2"
                >
                  <Save className="size-4" />
                  {isUpdatingOrganization
                    ? t('organization.settings.saving')
                    : t('organization.settings.save')}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>

        {isOwner ? (
          <Card className="border-border/80 shadow-sm">
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    <ShieldCheck className="size-5 text-primary" />
                    {t('organization.settings.adminManagement.title')}
                  </CardTitle>
                  <CardDescription>
                    {t('organization.settings.adminManagement.description')}
                  </CardDescription>
                </div>
                <Badge variant="info">
                  {t('organization.settings.adminManagement.adminCount', {
                    count: admins.length,
                  })}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-3">
                <div className="text-sm font-medium text-foreground">
                  {t('organization.settings.adminManagement.ownerTitle')}
                </div>
                <div className="space-y-2 rounded-lg border border-border/80 bg-muted/20 p-3">
                  {owners.length > 0 ? (
                    owners.map(owner => (
                      <div
                        key={owner.id}
                        className="flex items-center justify-between gap-3 rounded-md bg-background px-3 py-2"
                      >
                        {renderMemberIdentity(owner)}
                        <Badge variant="warning">
                          {t('organization.settings.adminManagement.ownerBadge')}
                        </Badge>
                      </div>
                    ))
                  ) : (
                    <div className="text-sm text-muted-foreground">
                      {t('organization.settings.adminManagement.noOwner')}
                    </div>
                  )}
                </div>
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <div className="text-sm font-medium text-foreground">
                    {t('organization.settings.adminManagement.adminTitle')}
                  </div>
                  {isFetchingMembers ? (
                    <span className="text-xs text-muted-foreground">
                      {t('organization.settings.adminManagement.refreshing')}
                    </span>
                  ) : null}
                </div>
                <div className="space-y-2">
                  {isLoadingMembers ? (
                    <>
                      <Skeleton className="h-14 rounded-lg" />
                      <Skeleton className="h-14 rounded-lg" />
                    </>
                  ) : admins.length > 0 ? (
                    admins.map(admin => (
                      <div
                        key={admin.id}
                        className="flex flex-col gap-3 rounded-lg border border-border/80 bg-background px-3 py-3 sm:flex-row sm:items-center sm:justify-between"
                      >
                        {renderMemberIdentity(admin)}
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="gap-2 self-start sm:self-auto"
                          disabled={isUpdatingCurrentOrganizationMemberRole}
                          onClick={() => setAdminToDemote(admin)}
                        >
                          <X className="size-4" />
                          {t('organization.settings.adminManagement.demote')}
                        </Button>
                      </div>
                    ))
                  ) : (
                    <div className="rounded-lg border border-dashed border-border/80 px-4 py-6 text-sm text-muted-foreground">
                      {t('organization.settings.adminManagement.emptyAdmins')}
                    </div>
                  )}
                </div>
              </div>

              <div className="space-y-3">
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
                    className="pl-9"
                  />
                </div>
                <div className="space-y-2">
                  {candidateMembers.length > 0 ? (
                    candidateMembers.map(member => (
                      <div
                        key={member.id}
                        className="flex flex-col gap-3 rounded-lg border border-border/80 bg-background px-3 py-3 sm:flex-row sm:items-center sm:justify-between"
                      >
                        {renderMemberIdentity(member)}
                        <Button
                          type="button"
                          size="sm"
                          className="gap-2 self-start sm:self-auto"
                          disabled={isUpdatingCurrentOrganizationMemberRole}
                          onClick={() => handlePromoteAdmin(member)}
                        >
                          <UserPlus className="size-4" />
                          {t('organization.settings.adminManagement.promote')}
                        </Button>
                      </div>
                    ))
                  ) : (
                    <div className="rounded-lg border border-dashed border-border/80 px-4 py-6 text-sm text-muted-foreground">
                      {t('organization.settings.adminManagement.emptyCandidates')}
                    </div>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        ) : null}
      </div>

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
