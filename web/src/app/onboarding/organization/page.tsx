'use client';

import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight, Building2, Loader2, MailPlus, Plus } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Logo } from '@/components/logo';
import { useT } from '@/i18n';
import { accountService } from '@/services/account.service';
import { organizationService } from '@/services/organization.service';
import type { Organization } from '@/services/types/organization';
import { useAuthStore } from '@/store/auth-store';
import { cn } from '@/lib/utils';
import { getErrorMessage } from '@/utils/error-notifications';

const CREATE_ORGANIZATION_ERROR_KEYS = {
  '199001': 'organizationOnboarding.invalidCreateParams',
  '205003': 'organizationOnboarding.duplicateName',
  '399001': 'organizationOnboarding.createSystemError',
  '401001': 'organizationOnboarding.createUnauthorized',
} as const;

type CreateOrganizationErrorKey =
  (typeof CREATE_ORGANIZATION_ERROR_KEYS)[keyof typeof CREATE_ORGANIZATION_ERROR_KEYS];

function getBusinessErrorCode(error: unknown): string | undefined {
  if (!error || typeof error !== 'object') return undefined;

  const businessError = (error as { businessError?: { code?: string } }).businessError;
  if (businessError?.code) return businessError.code;

  const responseData = (error as { response?: { data?: { code?: string; errorCode?: string } } })
    .response?.data;
  return responseData?.code ?? responseData?.errorCode;
}

function getCreateOrganizationErrorKey(error: unknown): CreateOrganizationErrorKey | null {
  const code = getBusinessErrorCode(error);
  const key = code
    ? CREATE_ORGANIZATION_ERROR_KEYS[code as keyof typeof CREATE_ORGANIZATION_ERROR_KEYS]
    : undefined;

  return key ?? null;
}

function extractInviteToken(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return '';

  try {
    const url = new URL(trimmed, window.location.origin);
    const match = url.pathname.match(/\/invite\/([^/?#]+)/);
    if (match?.[1]) return decodeURIComponent(match[1]);
  } catch {
    // Treat non-URL input as a raw invitation token.
  }

  return /^[A-Za-z0-9._~-]+$/.test(trimmed) ? trimmed : '';
}

export default function OrganizationOnboardingPage() {
  const t = useT('auth');
  const user = useAuthStore.use.user();
  const [selectedID, setSelectedID] = useState('');
  const [organizationName, setOrganizationName] = useState('');
  const [inviteValue, setInviteValue] = useState('');
  const [creating, setCreating] = useState(false);
  const [entering, setEntering] = useState(false);

  const organizationsQuery = useQuery({
    queryKey: ['organizations', 'onboarding'],
    queryFn: () =>
      organizationService.getOrganizationList({ page: 1, limit: 100, status: 'active' }),
  });
  const organizations = useMemo(
    () => organizationsQuery.data?.data ?? [],
    [organizationsQuery.data]
  );

  useEffect(() => {
    if (selectedID || organizations.length === 0) return;
    const current = organizations.find(item => item.id === user?.current_organization_id);
    setSelectedID((current ?? organizations[0]).id);
  }, [organizations, selectedID, user?.current_organization_id]);

  const enterOrganization = async (organization: Organization) => {
    setEntering(true);
    try {
      await accountService.updateContext({
        mode: 'organization',
        current_organization_id: organization.id,
        current_workspace_id: null,
      });
      window.location.href = '/console';
    } catch {
      toast.error(t('organizationOnboarding.enterFailed'));
      setEntering(false);
    }
  };

  const handleCreate = async () => {
    const name = organizationName.trim();
    if (!name) return;

    setCreating(true);
    try {
      const organization = await organizationService.createOrganization({ name });
      await enterOrganization(organization);
    } catch (error) {
      const errorKey = getCreateOrganizationErrorKey(error);
      const message =
        (errorKey ? t(errorKey) : null) ||
        getErrorMessage(error) ||
        t('organizationOnboarding.createFailed');
      toast.error(message);
    } finally {
      setCreating(false);
    }
  };

  const handleInvite = () => {
    const token = extractInviteToken(inviteValue);
    if (!token) {
      toast.error(t('organizationOnboarding.invalidInvite'));
      return;
    }
    window.location.href = `/invite/${encodeURIComponent(token)}`;
  };

  const selectedOrganization = organizations.find(item => item.id === selectedID);
  const busy = creating || entering;

  return (
    <main className="min-h-screen bg-muted/30 px-4 py-10 sm:px-6">
      <div className="mx-auto w-full max-w-3xl space-y-8">
        <div className="flex justify-center">
          <Logo />
        </div>

        <div className="space-y-2 text-center">
          <h1 className="text-3xl font-semibold tracking-tight">
            {t('organizationOnboarding.title')}
          </h1>
          <p className="text-muted-foreground">{t('organizationOnboarding.description')}</p>
        </div>

        {organizationsQuery.isLoading ? (
          <div className="flex justify-center py-16">
            <Loader2 className="size-7 animate-spin text-muted-foreground" />
          </div>
        ) : organizationsQuery.isError ? (
          <Card>
            <CardContent className="flex flex-col items-center gap-4 py-10 text-center">
              <p className="text-sm text-muted-foreground">
                {t('organizationOnboarding.loadFailed')}
              </p>
              <Button variant="outline" onClick={() => organizationsQuery.refetch()}>
                {t('organizationOnboarding.retry')}
              </Button>
            </CardContent>
          </Card>
        ) : (
          <div className="grid gap-5 md:grid-cols-2">
            {organizations.length > 0 && (
              <Card className="md:col-span-2">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-lg">
                    <Building2 className="size-5" />
                    {t('organizationOnboarding.chooseTitle')}
                  </CardTitle>
                  <CardDescription>{t('organizationOnboarding.chooseDescription')}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid gap-2 sm:grid-cols-2">
                    {organizations.map(organization => (
                      <button
                        key={organization.id}
                        type="button"
                        disabled={busy}
                        onClick={() => setSelectedID(organization.id)}
                        className={cn(
                          'rounded-xl border p-4 text-left transition-colors hover:bg-muted/60',
                          selectedID === organization.id && 'border-primary bg-primary/5'
                        )}
                      >
                        <span className="block font-medium">{organization.name}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {t(
                            `organizationOnboarding.role.${organization.organization_role ?? 'normal'}`
                          )}
                        </span>
                      </button>
                    ))}
                  </div>
                  <Button
                    className="w-full sm:w-auto"
                    disabled={!selectedOrganization || busy}
                    onClick={() => selectedOrganization && enterOrganization(selectedOrganization)}
                  >
                    {entering && <Loader2 className="mr-2 size-4 animate-spin" />}
                    {t('organizationOnboarding.enter')}
                    {!entering && <ArrowRight className="ml-2 size-4" />}
                  </Button>
                </CardContent>
              </Card>
            )}

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <Plus className="size-5" />
                  {t('organizationOnboarding.createTitle')}
                </CardTitle>
                <CardDescription>{t('organizationOnboarding.createDescription')}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <Label htmlFor="organization-name">
                  {t('organizationOnboarding.organizationName')}
                </Label>
                <Input
                  id="organization-name"
                  value={organizationName}
                  disabled={busy}
                  maxLength={100}
                  placeholder={t('organizationOnboarding.organizationNamePlaceholder')}
                  onChange={event => setOrganizationName(event.target.value)}
                  onKeyDown={event => {
                    if (event.key === 'Enter') void handleCreate();
                  }}
                />
                <Button
                  className="w-full"
                  disabled={!organizationName.trim() || busy}
                  onClick={handleCreate}
                >
                  {creating && <Loader2 className="mr-2 size-4 animate-spin" />}
                  {t('organizationOnboarding.create')}
                </Button>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <MailPlus className="size-5" />
                  {t('organizationOnboarding.joinTitle')}
                </CardTitle>
                <CardDescription>{t('organizationOnboarding.joinDescription')}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <Label htmlFor="invite-link">{t('organizationOnboarding.inviteLabel')}</Label>
                <Input
                  id="invite-link"
                  value={inviteValue}
                  disabled={busy}
                  placeholder={t('organizationOnboarding.invitePlaceholder')}
                  onChange={event => setInviteValue(event.target.value)}
                  onKeyDown={event => {
                    if (event.key === 'Enter') handleInvite();
                  }}
                />
                <Button
                  className="w-full"
                  variant="outline"
                  disabled={!inviteValue.trim() || busy}
                  onClick={handleInvite}
                >
                  {t('organizationOnboarding.join')}
                </Button>
              </CardContent>
            </Card>
          </div>
        )}
      </div>
    </main>
  );
}
