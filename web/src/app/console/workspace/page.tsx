'use client';

import Link from 'next/link';
import { type ComponentType, type ReactNode } from 'react';
import {
  ArrowRight,
  Bot,
  BookOpen,
  CheckCircle2,
  Circle,
  Coins,
  Database,
  FileText,
  RefreshCw,
  Settings,
  ShieldCheck,
  Users,
  Workflow,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkspaceQuota } from '@/hooks/workspace-quota/use-workspace-quota';
import { useWorkspaceStatistics } from '@/hooks/workspace/use-workspace-statistics';
import { useLocale } from '@/hooks/use-locale';
import { useT, type WorkspaceKey } from '@/i18n';
import { cn } from '@/lib/utils';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { formatAiCreditFiatEstimate, formatChannelCreditPoints } from '@/utils/ai-credits';
import {
  normalizeOrganizationRole,
  normalizeWorkspaceMemberRole,
} from '@/utils/role-labels';
import {
  AGENT_VISIBLE_PERMISSION_CODES,
  DATABASE_VISIBLE_PERMISSION_CODES,
  KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES,
  WORKFLOW_VISIBLE_PERMISSION_CODES,
} from '@/constants/permissions';

interface WorkspaceActionEntry {
  key: string;
  title: string;
  description: string;
  href: string;
  icon: ComponentType<{ className?: string }>;
  enabled: boolean;
}

interface WorkspaceStatCard {
  key: string;
  label: string;
  value: number | undefined;
  icon: ComponentType<{ className?: string }>;
  href: string;
  enabled: boolean;
}

interface PermissionItem {
  key: string;
  label: string;
  enabled: boolean;
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <div className="mb-3 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
      {children}
    </div>
  );
}

function PermissionBadge({ enabled }: { enabled: boolean }) {
  const t = useT();

  return (
    <Badge variant={enabled ? 'success' : 'outline'}>
      {enabled ? (
        <CheckCircle2 className="size-3" />
      ) : (
        <Circle className="size-3 text-muted-foreground" />
      )}
      {enabled
        ? t('workspace.overview.permissions.enabled')
        : t('workspace.overview.permissions.disabled')}
    </Badge>
  );
}

function ActionTile({ entry }: { entry: WorkspaceActionEntry }) {
  const Icon = entry.icon;
  const content = (
    <div
      className={cn(
        'flex h-full items-start gap-3 rounded-lg border border-border/70 p-4 transition-colors',
        entry.enabled ? 'hover:border-primary/40 hover:bg-muted/30' : 'opacity-60'
      )}
    >
      <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border/70 bg-muted/30">
        <Icon className="size-4 text-muted-foreground" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold text-foreground">{entry.title}</div>
        <div className="mt-1 line-clamp-2 text-sm leading-5 text-muted-foreground">
          {entry.description}
        </div>
      </div>
      <ArrowRight
        className={cn(
          'mt-1 size-3.5 shrink-0',
          entry.enabled ? 'text-primary' : 'text-muted-foreground'
        )}
      />
    </div>
  );

  if (!entry.enabled) {
    return <div aria-disabled="true">{content}</div>;
  }

  return (
    <Link key={entry.key} href={entry.href}>
      {content}
    </Link>
  );
}

export default function WorkspacePage() {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const workspaceId = currentWorkspace?.id ?? '';
  const {
    hasAnyPermission,
    hasWorkspaceAccess,
    isWorkspaceManager,
    workspaceRole,
    workspaceRoleName,
    organizationRole,
    isAdmin,
    isLoading: isPermissionsLoading,
    isFetching: isPermissionsFetching,
  } = useAccountPermissions();
  const {
    data: workspaceStats,
    isLoading: isStatsLoading,
    isFetching: isStatsFetching,
    refetch: refetchStats,
  } = useWorkspaceStatistics(workspaceId, Boolean(workspaceId));
  const {
    quota: workspaceQuota,
    isLoading: isQuotaLoading,
    isFetching: isQuotaFetching,
    refetch: refetchQuota,
  } = useWorkspaceQuota(workspaceId);
  const { locale } = useLocale();

  const canViewWorkspace = hasWorkspaceAccess();
  const canManageWorkspace = isWorkspaceManager();
  const canViewAgents = hasAnyPermission(AGENT_VISIBLE_PERMISSION_CODES);
  const canViewWorkflows = hasAnyPermission(WORKFLOW_VISIBLE_PERMISSION_CODES);
  const canViewDatasets = hasAnyPermission(KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES);
  const canViewDatabases = hasAnyPermission(DATABASE_VISIBLE_PERMISSION_CODES);
  const canViewFiles = canViewWorkspace;
  const isOrganizationAdmin = isAdmin();
  const isPermissionsBusy = isPermissionsLoading || isPermissionsFetching;
  const isQuotaBusy = isQuotaLoading || isQuotaFetching;
  const hasQuotaData = Boolean(workspaceQuota);
  const isQuotaUnlimited =
    hasQuotaData &&
    (workspaceQuota?.quota_limit === null || workspaceQuota?.quota_limit === undefined);
  const normalizedWorkspaceRole = normalizeWorkspaceMemberRole(workspaceRole);
  const normalizedOrganizationRole = normalizeOrganizationRole(organizationRole);
  const roleLabel =
    normalizedWorkspaceRole === 'owner' || normalizedWorkspaceRole === 'admin'
      ? t(`workspace.members.roles.${normalizedWorkspaceRole}` as WorkspaceKey)
      : workspaceRoleName ||
        (normalizedWorkspaceRole
          ? t(`workspace.members.roles.${normalizedWorkspaceRole}` as WorkspaceKey)
          : t('workspace.overview.permissions.roleFallback'));
  const organizationRoleLabel = normalizedOrganizationRole
    ? t(`workspace.overview.permissions.organizationRoles.${normalizedOrganizationRole}` as WorkspaceKey)
    : t('workspace.overview.permissions.organizationRoleFallback');
  const workspaceQuotaBalance = workspaceQuota?.remain_quota ?? 0;
  const quotaBalanceValue = !hasQuotaData
    ? '-'
    : isQuotaUnlimited
      ? t('workspace.quota.unlimited')
      : t('workspace.quota.pointsLabel', {
          points: formatChannelCreditPoints(workspaceQuotaBalance, { locale }),
        });
  const quotaBalanceFiat = t('workspace.quota.approxFiat', {
    amount: formatAiCreditFiatEstimate(workspaceQuotaBalance, { locale }),
  });

  const statCards: WorkspaceStatCard[] = [
    {
      key: 'agents',
      label: t('workspace.overview.stats.agents'),
      value: workspaceStats?.agents_count,
      icon: Bot,
      href: '/console/agents',
      enabled: canViewAgents,
    },
    {
      key: 'datasets',
      label: t('workspace.overview.stats.datasets'),
      value: workspaceStats?.datasets_count,
      icon: BookOpen,
      href: '/console/dataset',
      enabled: canViewDatasets,
    },
    {
      key: 'members',
      label: t('workspace.overview.stats.members'),
      value: workspaceStats?.members_count,
      icon: Users,
      href: '/console/workspace/members',
      enabled: canManageWorkspace,
    },
    {
      key: 'admins',
      label: t('workspace.overview.stats.admins'),
      value: workspaceStats?.admins_count,
      icon: ShieldCheck,
      href: '/console/workspace/members',
      enabled: canManageWorkspace,
    },
  ];

  const actionEntries: WorkspaceActionEntry[] = [
    {
      key: 'members',
      title: t('workspace.overview.management.membersTitle'),
      description: t('workspace.overview.management.membersDescription'),
      href: '/console/workspace/members',
      icon: Users,
      enabled: canManageWorkspace,
    },
    {
      key: 'settings',
      title: t('workspace.overview.management.settingsTitle'),
      description: t('workspace.overview.management.settingsDescription'),
      href: '/console/workspace/settings',
      icon: Settings,
      enabled: canManageWorkspace,
    },
    {
      key: 'agents',
      title: t('workspace.overview.management.agentsTitle'),
      description: t('workspace.overview.management.agentsDescription'),
      href: '/console/agents',
      icon: Bot,
      enabled: canViewAgents,
    },
    {
      key: 'workflows',
      title: t('workspace.overview.management.workflowsTitle'),
      description: t('workspace.overview.management.workflowsDescription'),
      href: '/console/workflows',
      icon: Workflow,
      enabled: canViewWorkflows,
    },
    {
      key: 'datasets',
      title: t('workspace.overview.management.datasetsTitle'),
      description: t('workspace.overview.management.datasetsDescription'),
      href: '/console/dataset',
      icon: BookOpen,
      enabled: canViewDatasets,
    },
    {
      key: 'databases',
      title: t('workspace.overview.management.databasesTitle'),
      description: t('workspace.overview.management.databasesDescription'),
      href: '/console/db',
      icon: Database,
      enabled: canViewDatabases,
    },
    {
      key: 'files',
      title: t('workspace.overview.management.filesTitle'),
      description: t('workspace.overview.management.filesDescription'),
      href: '/console/files',
      icon: FileText,
      enabled: canViewFiles,
    },
  ];

  const permissionItems: PermissionItem[] = [
    {
      key: 'membership',
      label: t('workspace.overview.permissions.membership'),
      enabled: canViewWorkspace,
    },
    {
      key: 'governance',
      label: t('workspace.overview.permissions.governanceAccess'),
      enabled: canManageWorkspace,
    },
    {
      key: 'agent-view',
      label: t('workspace.overview.permissions.agentView'),
      enabled: canViewAgents,
    },
    {
      key: 'workflow-view',
      label: t('workspace.overview.permissions.workflowView'),
      enabled: canViewWorkflows,
    },
    {
      key: 'knowledge-view',
      label: t('workspace.overview.permissions.datasetView'),
      enabled: canViewDatasets,
    },
    {
      key: 'database-view',
      label: t('workspace.overview.permissions.databaseView'),
      enabled: canViewDatabases,
    },
    {
      key: 'file-view',
      label: t('workspace.overview.permissions.fileView'),
      enabled: canViewFiles,
    },
  ];

  const handleRefresh = () => {
    void refetchStats();
    if (workspaceId) {
      void refetchQuota();
    }
  };

  if (!currentWorkspace) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <Skeleton className="h-8 w-52" />
      </div>
    );
  }

  return (
    <div className="mx-auto flex h-full max-w-7xl flex-col px-6 py-6">
      <header className="mb-5 flex flex-col gap-4 border-b border-border/70 pb-5 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="mb-2 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {t('workspace.overview.eyebrow')}
          </div>
          <h2 className="text-2xl font-semibold tracking-tight text-foreground">
            {t('workspace.overview.title')}
          </h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
            {t('workspace.overview.description', { name: currentWorkspace.name })}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="default"
          onClick={handleRefresh}
          disabled={isStatsLoading || isStatsFetching || isQuotaBusy}
          className="self-start"
        >
          <RefreshCw className={`size-4 ${isStatsFetching || isQuotaFetching ? 'animate-spin' : ''}`} />
          {t('common.refresh')}
        </Button>
      </header>

      <main className="min-h-0 flex-1 space-y-5 overflow-y-auto">
        <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
          <Card className="h-full border-border/80 shadow-sm">
            <CardHeader className="p-4 pb-3">
              <div className="flex items-center justify-between gap-3">
                <div className="flex size-9 items-center justify-center rounded-lg border border-border/70 bg-muted/30">
                  <Coins className="size-4 text-muted-foreground" />
                </div>
              </div>
            </CardHeader>
            <CardContent className="px-4 pb-4 pt-0">
              <div className="text-2xl font-semibold tracking-tight">
                {isQuotaLoading ? '-' : quotaBalanceValue}
              </div>
              <div className="mt-1 text-sm text-muted-foreground">
                {t('workspace.overview.stats.quotaBalance')}
              </div>
              {!isQuotaLoading && !isQuotaUnlimited ? (
                <div className="mt-1 text-xs text-muted-foreground">{quotaBalanceFiat}</div>
              ) : null}
            </CardContent>
          </Card>

          {statCards.map(card => {
            const Icon = card.icon;
            const content = (
              <Card
                className={cn(
                  'h-full border-border/80 shadow-sm transition-colors',
                  card.enabled ? 'hover:border-primary/40' : 'opacity-60'
                )}
              >
                <CardHeader className="p-4 pb-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex size-9 items-center justify-center rounded-lg border border-border/70 bg-muted/30">
                      <Icon className="size-4 text-muted-foreground" />
                    </div>
                    <ArrowRight
                      className={cn(
                        'size-3.5',
                        card.enabled ? 'text-primary' : 'text-muted-foreground'
                      )}
                    />
                  </div>
                </CardHeader>
                <CardContent className="px-4 pb-4 pt-0">
                  <div className="text-2xl font-semibold tracking-tight">
                    {isStatsLoading || card.value === undefined ? '-' : card.value}
                  </div>
                  <div className="mt-1 text-sm text-muted-foreground">{card.label}</div>
                </CardContent>
              </Card>
            );

            return card.enabled ? (
              <Link key={card.key} href={card.href}>
                {content}
              </Link>
            ) : (
              <div key={card.key} aria-disabled="true">
                {content}
              </div>
            );
          })}
        </section>

        <section className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <Card className="border-border/80 shadow-sm">
            <CardHeader>
              <SectionLabel>{t('workspace.overview.management.eyebrow')}</SectionLabel>
              <CardTitle className="text-lg">
                {t('workspace.overview.management.title')}
              </CardTitle>
              <CardDescription className="mt-2 leading-6">
                {t('workspace.overview.management.description')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid gap-3 md:grid-cols-2">
                {actionEntries.map(entry => (
                  <ActionTile key={entry.key} entry={entry} />
                ))}
              </div>
            </CardContent>
          </Card>

          <Card className="border-border/80 shadow-sm">
            <CardHeader>
              <SectionLabel>{t('workspace.overview.permissions.eyebrow')}</SectionLabel>
              <CardTitle className="text-lg">
                {t('workspace.overview.permissions.title')}
              </CardTitle>
              <CardDescription className="mt-2 leading-6">
                {t('workspace.overview.permissions.description')}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                <div className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                  {t('workspace.overview.permissions.currentRole')}
                </div>
                <div className="mt-2 text-base font-semibold text-foreground">
                  {isPermissionsBusy ? '-' : roleLabel}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {t('workspace.overview.permissions.organizationRole', {
                    role: isPermissionsBusy ? '-' : organizationRoleLabel,
                  })}
                </div>
              </div>

              <div className="divide-y divide-border/70 rounded-lg border border-border/70">
                {permissionItems.map(item => (
                  <div
                    key={item.key}
                    className="flex items-center justify-between gap-3 px-4 py-3"
                  >
                    <span className="min-w-0 text-sm font-medium text-foreground">
                      {item.label}
                    </span>
                    <PermissionBadge enabled={!isPermissionsBusy && item.enabled} />
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card className="border-border/80 shadow-sm xl:col-span-2">
            <CardHeader>
              <SectionLabel>{t('workspace.overview.governance.eyebrow')}</SectionLabel>
              <CardTitle className="text-lg">
                {isOrganizationAdmin || canManageWorkspace
                  ? t('workspace.overview.governance.adminTitle')
                  : t('workspace.overview.governance.memberTitle')}
              </CardTitle>
              <CardDescription className="mt-2 leading-6">
                {isOrganizationAdmin || canManageWorkspace
                  ? t('workspace.overview.governance.adminDescription')
                  : t('workspace.overview.governance.memberDescription')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid gap-3 md:grid-cols-2">
                <div className="rounded-lg border border-border/70 p-4">
                  <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                    <ShieldCheck className="size-4 text-muted-foreground" />
                    {t('workspace.overview.governance.permissionBoundaryTitle')}
                  </div>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {t('workspace.overview.governance.permissionBoundaryDescription')}
                  </p>
                </div>
                <div className="rounded-lg border border-border/70 p-4">
                  <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                    <Users className="size-4 text-muted-foreground" />
                    {t('workspace.overview.governance.memberOperationTitle')}
                  </div>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {t('workspace.overview.governance.memberOperationDescription')}
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </section>
      </main>
    </div>
  );
}
