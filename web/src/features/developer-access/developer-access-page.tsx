'use client';

import * as React from 'react';
import {
  Activity,
  Check,
  Clipboard,
  Clock3,
  Code2,
  KeyRound,
  MoreHorizontal,
  Plus,
  Settings2,
  ShieldCheck,
  UsersRound,
} from 'lucide-react';
import { useT } from '@/i18n';
import { API_URL } from '@/lib/config';
import { useCurrentWorkspace, useWorkspaceContextStatus } from '@/store/workspace-store';
import {
  useDeveloperAccess,
  useDeveloperAccessActions,
} from '@/hooks/developer-access/use-developer-access';
import type {
  CreatedPersonalApiKey,
  DeveloperAccessMode,
  DeveloperAccessRequest,
  DeveloperAccessAuditItem,
  PersonalApiKey,
  ReviewAccessRequestInput,
} from '@/services/developer-access.service';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

function formatDate(value?: string | null): string {
  if (!value) return '—';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
    new Date(value)
  );
}

function statusVariant(status: string): 'success' | 'warning' | 'subtle' | 'destructive' {
  if (status === 'active' || status === 'approved') return 'success';
  if (status === 'pending') return 'warning';
  if (status === 'revoked' || status === 'rejected' || status === 'failed') return 'destructive';
  return 'subtle';
}

function splitModels(value: string): string[] {
  return [
    ...new Set(
      value
        .split(',')
        .map(item => item.trim())
        .filter(Boolean)
    ),
  ];
}

function toDateTimeLocal(value: Date): string {
  const local = new Date(value.getTime() - value.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function EmptyState({ title, description }: { title: string; description?: string }) {
  return (
    <div className="flex min-h-56 w-full flex-col items-center justify-center rounded-xl border border-dashed bg-muted/20 px-6 text-center">
      <div className="mb-3 rounded-xl border bg-background p-3 shadow-sm">
        <KeyRound className="size-5 text-muted-foreground" />
      </div>
      <p className="font-medium">{title}</p>
      {description ? (
        <p className="mt-1 max-w-md text-sm text-muted-foreground">{description}</p>
      ) : null}
    </div>
  );
}

function KeyRow({
  item,
  onStatus,
  showPrincipal = false,
  canRotate = true,
}: {
  item: PersonalApiKey;
  onStatus: (action: 'enable' | 'disable' | 'revoke' | 'rotate') => void;
  showPrincipal?: boolean;
  canRotate?: boolean;
}) {
  const t = useT('apikeys.developerAccess');
  return (
    <div className="grid gap-4 border-b px-1 py-4 last:border-b-0 md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate font-medium">{item.name}</p>
          <Badge variant={statusVariant(item.status)}>{t(`statuses.${item.status}`)}</Badge>
          <Badge variant="outline">
            {item.environment === 'production' ? t('labels.production') : t('labels.development')}
          </Badge>
        </div>
        <code className="mt-1 block truncate text-xs text-muted-foreground">{item.key_masked}</code>
        {showPrincipal ? (
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {item.principal_name || item.principal_id.slice(0, 8)}
            {item.principal_email ? ` · ${item.principal_email}` : ''}
          </p>
        ) : null}
      </div>
      <div className="text-xs text-muted-foreground">
        <p>
          {t('labels.created')}: {formatDate(item.created_at)}
        </p>
        <p>
          {t('labels.lastUsed')}: {formatDate(item.accessed_at)}
        </p>
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" isIcon aria-label={t('actions.keyActions')}>
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {canRotate && item.status !== 'revoked' ? (
            <DropdownMenuItem onClick={() => onStatus('rotate')}>
              {t('actions.rotate')}
            </DropdownMenuItem>
          ) : null}
          {item.status === 'active' ? (
            <DropdownMenuItem onClick={() => onStatus('disable')}>
              {t('actions.disable')}
            </DropdownMenuItem>
          ) : item.status === 'inactive' ? (
            <DropdownMenuItem onClick={() => onStatus('enable')}>
              {t('actions.enable')}
            </DropdownMenuItem>
          ) : null}
          {item.status !== 'revoked' ? (
            <DropdownMenuItem className="text-destructive" onClick={() => onStatus('revoke')}>
              {t('actions.revoke')}
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function AuditList({
  items,
  loading,
  error,
  showPrincipal,
  total,
  page,
  pageSize,
  onPageChange,
}: {
  items: DeveloperAccessAuditItem[];
  loading: boolean;
  error: boolean;
  showPrincipal: boolean;
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
}) {
  const t = useT('apikeys.developerAccess');
  if (error) {
    return <EmptyState title={t('loadError')} description={t('loadErrorDescription')} />;
  }
  if (loading) {
    return <Skeleton className="h-56 w-full" />;
  }
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  if (!items.length && page <= 1) {
    return <EmptyState title={t('audit.empty')} description={t('audit.emptyDescription')} />;
  }
  return (
    <>
      {items.length ? (
        <Table>
          <TableHeader>
            <TableRow>
              {showPrincipal ? <TableHead>{t('audit.member')}</TableHead> : null}
              <TableHead>{t('audit.key')}</TableHead>
              <TableHead>{t('audit.model')}</TableHead>
              <TableHead>{t('audit.status')}</TableHead>
              <TableHead>{t('audit.tokens')}</TableHead>
              <TableHead>{t('audit.points')}</TableHead>
              <TableHead>{t('audit.time')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map(item => (
              <TableRow key={item.attempt_id}>
                {showPrincipal ? (
                  <TableCell>
                    <div className="max-w-48">
                      <p className="truncate font-medium">
                        {item.principal_name || item.principal_id.slice(0, 8)}
                      </p>
                      {item.principal_email ? (
                        <p className="truncate text-muted-foreground">{item.principal_email}</p>
                      ) : null}
                    </div>
                  </TableCell>
                ) : null}
                <TableCell>
                  <div className="max-w-44">
                    <p className="truncate font-medium">
                      {item.api_key_name || item.api_key_id.slice(0, 8)}
                    </p>
                    <code className="text-[11px] text-muted-foreground">
                      {item.api_key_masked || '—'}
                    </code>
                  </div>
                </TableCell>
                <TableCell>
                  <div className="max-w-40">
                    <p className="truncate font-medium">{item.model_name}</p>
                    <p className="truncate text-muted-foreground">{item.provider_name}</p>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant={statusVariant(item.status)}>{t(`statuses.${item.status}`)}</Badge>
                  {item.error_code ? (
                    <p className="mt-1 max-w-32 truncate text-destructive">{item.error_code}</p>
                  ) : null}
                </TableCell>
                <TableCell>{item.total_tokens.toLocaleString()}</TableCell>
                <TableCell>
                  <p>{item.total_points.toLocaleString()}</p>
                  {item.quota_overage_points > 0 ? (
                    <p className="text-[11px] text-amber-600 dark:text-amber-400">
                      {t('audit.quotaBreakdown', {
                        charged: item.quota_charged_points.toLocaleString(),
                        overage: item.quota_overage_points.toLocaleString(),
                      })}
                    </p>
                  ) : null}
                </TableCell>
                <TableCell>{formatDate(item.created_at)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <EmptyState title={t('audit.empty')} description={t('audit.emptyDescription')} />
      )}
      {totalPages > 1 || page > 1 ? (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
          <p className="text-sm text-muted-foreground">
            {t('audit.pagination', { page, totalPages, total })}
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => onPageChange(page - 1)}
            >
              {t('audit.previous')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => onPageChange(page + 1)}
            >
              {t('audit.next')}
            </Button>
          </div>
        </div>
      ) : null}
    </>
  );
}

function AccessRequestRow({
  item,
  canReview,
  onReview,
}: {
  item: DeveloperAccessRequest;
  canReview: boolean;
  onReview: (decision: 'approve' | 'reject') => void;
}) {
  const t = useT('apikeys.developerAccess');
  return (
    <div className="flex flex-col gap-3 border-b py-4 last:border-b-0 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={statusVariant(item.status)}>{t(`statuses.${item.status}`)}</Badge>
          <Badge variant="outline">
            {item.environment === 'production' ? t('labels.production') : t('labels.development')}
          </Badge>
          <span className="text-xs text-muted-foreground">{formatDate(item.created_at)}</span>
        </div>
        {canReview ? (
          <div className="mt-2">
            <p className="text-sm font-medium">
              {item.requester_name ||
                t('requestFrom', { id: item.requester_account_id.slice(0, 8) })}
            </p>
            {item.requester_email ? (
              <p className="text-xs text-muted-foreground">{item.requester_email}</p>
            ) : null}
          </div>
        ) : null}
        <p className="mt-1 line-clamp-2 text-sm">{item.purpose}</p>
      </div>
      {canReview && item.status === 'pending' ? (
        <div className="flex shrink-0 gap-2">
          <Button variant="outline" size="sm" onClick={() => onReview('reject')}>
            {t('actions.reject')}
          </Button>
          <Button size="sm" onClick={() => onReview('approve')}>
            {t('actions.approve')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function DeveloperAccessPage() {
  const t = useT('apikeys.developerAccess');
  const workspace = useCurrentWorkspace();
  const workspaceStatus = useWorkspaceContextStatus();
  const workspaceId = workspace?.id;
  const [auditPage, setAuditPage] = React.useState(1);
  const auditPageSize = 50;
  const { me, keys, requests, audit } = useDeveloperAccess(workspaceId, auditPage, auditPageSize);
  const actions = useDeveloperAccessActions(workspaceId);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [requestOpen, setRequestOpen] = React.useState(false);
  const [policyOpen, setPolicyOpen] = React.useState(false);
  const [createdKey, setCreatedKey] = React.useState<CreatedPersonalApiKey | null>(null);
  const [keyToRevoke, setKeyToRevoke] = React.useState<PersonalApiKey | null>(null);
  const [keyToRotate, setKeyToRotate] = React.useState<PersonalApiKey | null>(null);
  const [reviewTarget, setReviewTarget] = React.useState<{
    request: DeveloperAccessRequest;
    decision: 'approve' | 'reject';
  } | null>(null);
  const [copied, setCopied] = React.useState(false);

  React.useEffect(() => setAuditPage(1), [workspaceId]);

  if (workspaceStatus === 'loading' || (workspaceId && me.isLoading)) {
    return (
      <div className="mx-auto max-w-6xl space-y-6 p-6 lg:p-8">
        <Skeleton className="h-9 w-56" />
        <Skeleton className="h-36 w-full" />
        <Skeleton className="h-80 w-full" />
      </div>
    );
  }

  if (workspaceStatus !== 'ready' || !workspaceId) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center text-muted-foreground">
        {t('workspaceRequired')}
      </div>
    );
  }

  if (me.isError) {
    return (
      <div className="mx-auto flex min-h-[60vh] max-w-xl flex-col justify-center gap-4 p-6">
        <EmptyState title={t('loadError')} description={t('loadErrorDescription')} />
        <Button variant="outline" onClick={() => void me.refetch()}>
          {t('actions.retry')}
        </Button>
      </div>
    );
  }

  const access = me.data;
  const grant = access?.grant;
  const hasGrant = Boolean(grant);
  const quotaUnlimited = hasGrant && grant?.quota_limit == null;
  const quotaPercent =
    grant?.quota_limit && grant.quota_limit > 0
      ? Math.min(100, Math.round((grant.used_quota / grant.quota_limit) * 100))
      : 0;
  const pendingApprovals = (requests.data ?? []).filter(item => item.status === 'pending');
  const ownRequests = (requests.data ?? []).filter(
    item => item.requester_account_id === access?.principal_id
  );
  const personalKeys = (keys.data ?? []).filter(item => item.principal_id === access?.principal_id);
  const memberKeys = (keys.data ?? []).filter(item => item.principal_id !== access?.principal_id);
  const canCreate = Boolean(access?.can_create_key);
  const canRequest = Boolean(access?.can_request_access);
  const primaryAction = canCreate
    ? () => setCreateOpen(true)
    : canRequest
      ? () => setRequestOpen(true)
      : null;
  const primaryLabel = canCreate ? t('createKey') : t('requestAccess');

  const copySecret = async () => {
    if (!createdKey) return;
    await navigator.clipboard.writeText(createdKey.secret);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-5 sm:p-6 lg:p-8">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">
            {t('eyebrow')}
          </p>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight">{t('title')}</h1>
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{t('description')}</p>
        </div>
        <div className="flex gap-2">
          {access?.can_manage ? (
            <Button variant="outline" onClick={() => setPolicyOpen(true)}>
              <Settings2 className="mr-2 size-4" />
              {t('configure')}
            </Button>
          ) : null}
          {primaryAction ? (
            <Button onClick={primaryAction}>
              <Plus className="mr-2 size-4" />
              {primaryLabel}
            </Button>
          ) : null}
        </div>
      </header>

      {access?.pending_request ? (
        <div className="flex items-start gap-3 rounded-xl border border-warning/25 bg-warning/5 p-4">
          <Clock3 className="mt-0.5 size-5 text-warning" />
          <div>
            <p className="font-medium">{t('pendingTitle')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('pendingDescription')}</p>
          </div>
        </div>
      ) : null}

      <section className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t('quota')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">
              {!hasGrant ? '—' : quotaUnlimited ? t('unlimited') : (grant?.remain_quota ?? 0)}
            </p>
            {grant?.quota_limit != null ? (
              <>
                <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-primary"
                    style={{ width: `${quotaPercent}%` }}
                  />
                </div>
                <p className="mt-2 text-xs text-muted-foreground">
                  {t('quotaSummary', { used: grant.used_quota, remaining: grant.remain_quota })}
                </p>
              </>
            ) : null}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t('activeKeys')}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex items-end justify-between">
            <p className="text-2xl font-semibold">
              {access?.active_key_count ?? 0}
              <span className="text-sm font-normal text-muted-foreground">
                {' '}
                / {grant?.max_keys ?? access?.policy.max_keys ?? 0}
              </span>
            </p>
            <KeyRound className="size-5 text-muted-foreground" />
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t('allowedModels')}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex items-end justify-between">
            <p className="text-lg font-semibold">
              {grant?.allowed_models?.length ? grant.allowed_models.length : t('allModels')}
            </p>
            <ShieldCheck className="size-5 text-muted-foreground" />
          </CardContent>
        </Card>
      </section>

      <Card>
        <CardContent className="pt-6">
          <Tabs defaultValue="keys">
            <TabsList className="h-auto flex-wrap justify-start">
              <TabsTrigger value="keys">{t('tabs.keys')}</TabsTrigger>
              {access?.can_manage ? (
                <TabsTrigger value="memberKeys">
                  <UsersRound className="mr-2 size-4" />
                  {t('tabs.memberKeys')}
                  {memberKeys.length ? (
                    <Badge className="ml-2" variant="subtle">
                      {memberKeys.length}
                    </Badge>
                  ) : null}
                </TabsTrigger>
              ) : null}
              <TabsTrigger value="requests">{t('tabs.requests')}</TabsTrigger>
              {access?.can_manage ? (
                <TabsTrigger value="approvals">
                  {t('tabs.approvals')}
                  {pendingApprovals.length ? (
                    <Badge className="ml-2" variant="warning">
                      {pendingApprovals.length}
                    </Badge>
                  ) : null}
                </TabsTrigger>
              ) : null}
              <TabsTrigger value="audit">
                <Activity className="mr-2 size-4" />
                {t('tabs.audit')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="keys" className="mt-5">
              {keys.isError ? (
                <EmptyState title={t('loadError')} description={t('loadErrorDescription')} />
              ) : keys.isLoading ? (
                <Skeleton className="h-48 w-full" />
              ) : personalKeys.length === 0 ? (
                <EmptyState title={t('emptyKeys')} description={t('emptyKeysDescription')} />
              ) : (
                <div>
                  {personalKeys.map(item => (
                    <KeyRow
                      key={item.id}
                      item={item}
                      onStatus={action => {
                        if (action === 'rotate') {
                          setKeyToRotate(item);
                          return;
                        }
                        if (action === 'revoke') {
                          setKeyToRevoke(item);
                          return;
                        }
                        actions.setKeyStatus.mutate({ keyId: item.id, action });
                      }}
                    />
                  ))}
                </div>
              )}
            </TabsContent>
            {access?.can_manage ? (
              <TabsContent value="memberKeys" className="mt-5">
                {keys.isError ? (
                  <EmptyState title={t('loadError')} description={t('loadErrorDescription')} />
                ) : keys.isLoading ? (
                  <Skeleton className="h-48 w-full" />
                ) : memberKeys.length === 0 ? (
                  <EmptyState
                    title={t('emptyMemberKeys')}
                    description={t('emptyMemberKeysDescription')}
                  />
                ) : (
                  <div>
                    {memberKeys.map(item => (
                      <KeyRow
                        key={item.id}
                        item={item}
                        showPrincipal
                        canRotate={false}
                        onStatus={action => {
                          if (action === 'rotate') return;
                          if (action === 'revoke') {
                            setKeyToRevoke(item);
                            return;
                          }
                          actions.setKeyStatus.mutate({ keyId: item.id, action });
                        }}
                      />
                    ))}
                  </div>
                )}
              </TabsContent>
            ) : null}
            <TabsContent value="requests" className="mt-5">
              {requests.isError ? (
                <EmptyState title={t('loadError')} description={t('loadErrorDescription')} />
              ) : ownRequests.length === 0 ? (
                <EmptyState title={t('emptyRequests')} />
              ) : (
                <div>
                  {ownRequests.map(item => (
                    <AccessRequestRow
                      key={item.id}
                      item={item}
                      canReview={false}
                      onReview={() => undefined}
                    />
                  ))}
                </div>
              )}
            </TabsContent>
            {access?.can_manage ? (
              <TabsContent value="approvals" className="mt-5">
                {requests.isError ? (
                  <EmptyState title={t('loadError')} description={t('loadErrorDescription')} />
                ) : (requests.data ?? []).length === 0 ? (
                  <EmptyState title={t('emptyApprovals')} />
                ) : (
                  <div>
                    {(requests.data ?? []).map(item => (
                      <AccessRequestRow
                        key={item.id}
                        item={item}
                        canReview={item.requester_account_id !== access?.principal_id}
                        onReview={decision => setReviewTarget({ request: item, decision })}
                      />
                    ))}
                  </div>
                )}
              </TabsContent>
            ) : null}
            <TabsContent value="audit" className="mt-5">
              <AuditList
                items={audit.data?.items ?? []}
                loading={audit.isLoading}
                error={audit.isError}
                showPrincipal={Boolean(access?.can_manage)}
                total={audit.data?.total ?? 0}
                page={audit.data?.page ?? auditPage}
                pageSize={audit.data?.page_size ?? auditPageSize}
                onPageChange={setAuditPage}
              />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <CreateKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        pending={actions.createKey.isPending}
        onSubmit={async input => {
          const result = await actions.createKey.mutateAsync(input);
          setCreateOpen(false);
          setCreatedKey(result);
        }}
      />
      <RequestAccessDialog
        open={requestOpen}
        onOpenChange={setRequestOpen}
        pending={actions.createRequest.isPending}
        onSubmit={async input => {
          await actions.createRequest.mutateAsync(input);
          setRequestOpen(false);
        }}
      />
      {access ? (
        <PolicyDialog
          open={policyOpen}
          onOpenChange={setPolicyOpen}
          policy={access.policy}
          pending={actions.updatePolicy.isPending}
          onSubmit={async input => {
            await actions.updatePolicy.mutateAsync(input);
            setPolicyOpen(false);
          }}
        />
      ) : null}
      <ReviewAccessDialog
        target={reviewTarget}
        policy={access?.policy}
        pending={actions.reviewRequest.isPending}
        onOpenChange={open => {
          if (!open) setReviewTarget(null);
        }}
        onSubmit={async review => {
          if (!reviewTarget) return;
          await actions.reviewRequest.mutateAsync({
            requestId: reviewTarget.request.id,
            decision: reviewTarget.decision,
            review,
          });
          setReviewTarget(null);
        }}
      />

      <Dialog
        open={Boolean(createdKey)}
        onOpenChange={open => {
          if (!open) setCreatedKey(null);
        }}
      >
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('secretTitle')}</DialogTitle>
            <DialogDescription>{t('secretDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-5">
            <div className="flex items-center gap-2 rounded-lg border bg-muted/30 p-3">
              <code className="min-w-0 flex-1 break-all text-sm">{createdKey?.secret}</code>
              <Button size="sm" variant="outline" onClick={copySecret}>
                {copied ? <Check className="mr-2 size-4" /> : <Clipboard className="mr-2 size-4" />}
                {copied ? t('actions.copied') : t('actions.copy')}
              </Button>
            </div>
            <div>
              <div className="mb-2 flex items-center gap-2 text-sm font-medium">
                <Code2 className="size-4" />
                {t('quickstart')}
              </div>
              <pre className="overflow-x-auto rounded-lg bg-neutral-950 p-4 text-xs text-neutral-100">
                <code>{`curl ${API_URL}/v1/chat/completions \\\n  -H "Authorization: Bearer ${createdKey?.secret ?? 'YOUR_API_KEY'}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"your-model","messages":[{"role":"user","content":"Hello"}]}'`}</code>
              </pre>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setCreatedKey(null)}>{t('actions.close')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(keyToRevoke)}
        onOpenChange={open => {
          if (!open) setKeyToRevoke(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('revokeTitle')}</DialogTitle>
            <DialogDescription>
              {t('revokeDescription', { name: keyToRevoke?.name ?? '' })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setKeyToRevoke(null)}>
              {t('actions.cancel')}
            </Button>
            <Button
              variant="destructive"
              disabled={actions.setKeyStatus.isPending}
              onClick={async () => {
                if (!keyToRevoke) return;
                await actions.setKeyStatus.mutateAsync({ keyId: keyToRevoke.id, action: 'revoke' });
                setKeyToRevoke(null);
              }}
            >
              {t('actions.confirmRevoke')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(keyToRotate)}
        onOpenChange={open => {
          if (!open) setKeyToRotate(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('rotateTitle')}</DialogTitle>
            <DialogDescription>
              {t('rotateDescription', { name: keyToRotate?.name ?? '' })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setKeyToRotate(null)}>
              {t('actions.cancel')}
            </Button>
            <Button
              disabled={actions.rotateKey.isPending}
              onClick={async () => {
                if (!keyToRotate) return;
                const result = await actions.rotateKey.mutateAsync({
                  keyId: keyToRotate.id,
                  name: keyToRotate.name,
                });
                setKeyToRotate(null);
                setCreatedKey(result);
              }}
            >
              {t('actions.confirmRotate')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function CreateKeyDialog({
  open,
  onOpenChange,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onSubmit: (input: { name: string; environment: 'development' | 'production' }) => Promise<void>;
}) {
  const t = useT('apikeys.developerAccess');
  const [name, setName] = React.useState('');
  const [environment, setEnvironment] = React.useState<'development' | 'production'>('development');
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('createKey')}</DialogTitle>
          <DialogDescription>{t('secretDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="personal-key-name">{t('labels.name')}</Label>
            <Input
              id="personal-key-name"
              value={name}
              onChange={event => setName(event.target.value)}
              placeholder={t('placeholders.keyName')}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="personal-key-env">{t('labels.environment')}</Label>
            <select
              id="personal-key-env"
              className="h-10 w-full rounded-md border bg-background px-3 text-sm"
              value={environment}
              onChange={event => setEnvironment(event.target.value as 'development' | 'production')}
            >
              <option value="development">{t('labels.development')}</option>
              <option value="production">{t('labels.production')}</option>
            </select>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
          <Button
            disabled={!name.trim() || pending}
            onClick={() => onSubmit({ name: name.trim(), environment })}
          >
            {t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RequestAccessDialog({
  open,
  onOpenChange,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onSubmit: (input: {
    purpose: string;
    environment: 'development' | 'production';
    requested_quota?: number;
    requested_models?: string[];
    requested_ttl_seconds?: number;
  }) => Promise<void>;
}) {
  const t = useT('apikeys.developerAccess');
  const [purpose, setPurpose] = React.useState('');
  const [quota, setQuota] = React.useState('');
  const [models, setModels] = React.useState('');
  const [ttlHours, setTTLHours] = React.useState('');
  const [environment, setEnvironment] = React.useState<'development' | 'production'>('development');
  const quotaValue = quota === '' ? undefined : Number(quota);
  const ttlHoursValue = ttlHours === '' ? undefined : Number(ttlHours);
  const requestInvalid =
    (quotaValue !== undefined && (!Number.isFinite(quotaValue) || quotaValue < 0)) ||
    (ttlHoursValue !== undefined && (!Number.isFinite(ttlHoursValue) || ttlHoursValue <= 0));
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('requestAccess')}</DialogTitle>
          <DialogDescription>{t('pendingDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="access-purpose">{t('labels.purpose')}</Label>
            <Textarea
              id="access-purpose"
              value={purpose}
              onChange={event => setPurpose(event.target.value)}
              placeholder={t('placeholders.purpose')}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="access-models">{t('labels.requestedModels')}</Label>
              <Input
                id="access-models"
                value={models}
                onChange={event => setModels(event.target.value)}
                placeholder={t('placeholders.models')}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="access-ttl">{t('labels.requestedTTL')}</Label>
              <Input
                id="access-ttl"
                type="number"
                min={1}
                value={ttlHours}
                onChange={event => setTTLHours(event.target.value)}
                placeholder={t('placeholders.ttlHours')}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="access-env">{t('labels.environment')}</Label>
            <select
              id="access-env"
              className="h-10 w-full rounded-md border bg-background px-3 text-sm"
              value={environment}
              onChange={event => setEnvironment(event.target.value as 'development' | 'production')}
            >
              <option value="development">{t('labels.development')}</option>
              <option value="production">{t('labels.production')}</option>
            </select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="access-quota">{t('labels.requestedQuota')}</Label>
            <Input
              id="access-quota"
              type="number"
              min={0}
              value={quota}
              onChange={event => setQuota(event.target.value)}
              placeholder={t('placeholders.quota')}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
          <Button
            disabled={!purpose.trim() || pending || requestInvalid}
            onClick={() =>
              onSubmit({
                purpose: purpose.trim(),
                environment,
                requested_quota: quotaValue,
                requested_models: splitModels(models),
                requested_ttl_seconds:
                  ttlHoursValue === undefined ? undefined : ttlHoursValue * 3600,
              })
            }
          >
            {t('actions.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ReviewAccessDialog({
  target,
  policy,
  pending,
  onOpenChange,
  onSubmit,
}: {
  target: { request: DeveloperAccessRequest; decision: 'approve' | 'reject' } | null;
  policy?: {
    default_quota?: number | null;
    max_quota?: number | null;
    max_keys: number;
    default_ttl_seconds?: number | null;
    max_ttl_seconds?: number | null;
    allowed_models: string[];
  };
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: ReviewAccessRequestInput) => Promise<void>;
}) {
  const t = useT('apikeys.developerAccess');
  const [quota, setQuota] = React.useState('');
  const [maxKeys, setMaxKeys] = React.useState('1');
  const [models, setModels] = React.useState('');
  const [expiresAt, setExpiresAt] = React.useState('');
  const [reason, setReason] = React.useState('');
  React.useEffect(() => {
    if (!target || !policy) return;
    const initialQuota = target.request.requested_quota ?? policy.default_quota;
    setQuota(initialQuota == null ? '' : initialQuota.toString());
    setMaxKeys(policy.max_keys.toString());
    setModels(
      (target.request.requested_models.length
        ? target.request.requested_models
        : policy.allowed_models
      ).join(', ')
    );
    const ttl = target.request.requested_ttl_seconds ?? policy.default_ttl_seconds;
    setExpiresAt(ttl ? toDateTimeLocal(new Date(Date.now() + ttl * 1000)) : '');
    setReason('');
  }, [policy, target]);
  const approving = target?.decision === 'approve';
  const quotaValue = quota === '' ? undefined : Number(quota);
  const quotaInvalid =
    approving &&
    quotaValue !== undefined &&
    (!Number.isFinite(quotaValue) ||
      quotaValue < 0 ||
      (policy?.max_quota != null && quotaValue > policy.max_quota));
  const maxKeysValue = Number(maxKeys);
  const expiryValue = expiresAt ? new Date(expiresAt) : undefined;
  const approvalInvalid =
    approving &&
    (quotaInvalid ||
      !Number.isInteger(maxKeysValue) ||
      maxKeysValue < 1 ||
      maxKeysValue > (policy?.max_keys ?? 1) ||
      Boolean(expiryValue && !Number.isFinite(expiryValue.getTime())) ||
      Boolean(expiryValue && expiryValue.getTime() <= Date.now()) ||
      Boolean(
        expiryValue &&
          policy?.max_ttl_seconds &&
          expiryValue.getTime() > Date.now() + policy.max_ttl_seconds * 1000
      ));
  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>
            {approving ? t('review.approveTitle') : t('review.rejectTitle')}
          </DialogTitle>
          <DialogDescription>
            {target?.request.requester_name ||
              t('requestFrom', { id: target?.request.requester_account_id.slice(0, 8) ?? '' })}
            {target?.request.requester_email ? ` · ${target.request.requester_email}` : ''}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="rounded-lg border bg-muted/20 p-3">
            <p className="text-xs font-medium text-muted-foreground">{t('requestPurpose')}</p>
            <p className="mt-1 text-sm">{target?.request.purpose}</p>
          </div>
          {approving ? (
            <div className="space-y-2">
              <Label htmlFor="review-quota">{t('approvalQuota')}</Label>
              <Input
                id="review-quota"
                type="number"
                min={0}
                max={policy?.max_quota ?? undefined}
                value={quota}
                onChange={event => setQuota(event.target.value)}
                placeholder={t('placeholders.quota')}
              />
              {policy?.max_quota != null ? (
                <p className="text-xs text-muted-foreground">
                  {t('review.maxQuota', { max: policy.max_quota })}
                </p>
              ) : null}
            </div>
          ) : null}
          {approving ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="review-max-keys">{t('labels.maxKeys')}</Label>
                <Input
                  id="review-max-keys"
                  type="number"
                  min={1}
                  max={policy?.max_keys}
                  value={maxKeys}
                  onChange={event => setMaxKeys(event.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  {t('review.maxKeys', { max: policy?.max_keys ?? 1 })}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="review-expires">{t('labels.expires')}</Label>
                <Input
                  id="review-expires"
                  type="datetime-local"
                  value={expiresAt}
                  onChange={event => setExpiresAt(event.target.value)}
                />
                <p className="text-xs text-muted-foreground">{t('review.expiryHint')}</p>
              </div>
            </div>
          ) : null}
          {approving ? (
            <div className="space-y-2">
              <Label htmlFor="review-models">{t('labels.allowedModelNames')}</Label>
              <Input
                id="review-models"
                value={models}
                onChange={event => setModels(event.target.value)}
                placeholder={t('placeholders.models')}
              />
              <p className="text-xs text-muted-foreground">{t('review.modelsHint')}</p>
            </div>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="review-reason">{t('review.reason')}</Label>
            <Textarea
              id="review-reason"
              value={reason}
              onChange={event => setReason(event.target.value)}
              placeholder={t('review.reasonPlaceholder')}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            variant={approving ? 'default' : 'destructive'}
            disabled={pending || approvalInvalid}
            onClick={() =>
              onSubmit({
                quota_limit: approving ? quotaValue : undefined,
                max_keys: approving ? maxKeysValue : undefined,
                allowed_models: approving ? splitModels(models) : undefined,
                expires_at: approving && expiryValue ? expiryValue.toISOString() : undefined,
                reason: reason.trim() || undefined,
              })
            }
          >
            {approving ? t('actions.approve') : t('actions.reject')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PolicyDialog({
  open,
  onOpenChange,
  policy,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  policy: {
    mode: DeveloperAccessMode;
    default_quota?: number | null;
    max_quota?: number | null;
    max_keys: number;
    default_ttl_seconds?: number | null;
    max_ttl_seconds?: number | null;
    allowed_models: string[];
  };
  pending: boolean;
  onSubmit: (input: {
    mode: DeveloperAccessMode;
    default_quota?: number | null;
    max_quota?: number | null;
    max_keys: number;
    default_ttl_seconds?: number | null;
    max_ttl_seconds?: number | null;
    allowed_models: string[];
  }) => Promise<void>;
}) {
  const t = useT('apikeys.developerAccess');
  const [mode, setMode] = React.useState<DeveloperAccessMode>(policy.mode);
  const [defaultQuota, setDefaultQuota] = React.useState(policy.default_quota?.toString() ?? '');
  const [maxQuota, setMaxQuota] = React.useState(policy.max_quota?.toString() ?? '');
  const [maxKeys, setMaxKeys] = React.useState(policy.max_keys.toString());
  const [defaultTTLHours, setDefaultTTLHours] = React.useState(
    policy.default_ttl_seconds ? (policy.default_ttl_seconds / 3600).toString() : ''
  );
  const [maxTTLHours, setMaxTTLHours] = React.useState(
    policy.max_ttl_seconds ? (policy.max_ttl_seconds / 3600).toString() : ''
  );
  const [models, setModels] = React.useState(policy.allowed_models.join(', '));
  React.useEffect(() => {
    if (open) {
      setMode(policy.mode);
      setDefaultQuota(policy.default_quota?.toString() ?? '');
      setMaxQuota(policy.max_quota?.toString() ?? '');
      setMaxKeys(policy.max_keys.toString());
      setDefaultTTLHours(
        policy.default_ttl_seconds ? (policy.default_ttl_seconds / 3600).toString() : ''
      );
      setMaxTTLHours(policy.max_ttl_seconds ? (policy.max_ttl_seconds / 3600).toString() : '');
      setModels(policy.allowed_models.join(', '));
    }
  }, [open, policy]);
  const defaultQuotaValue = defaultQuota === '' ? undefined : Number(defaultQuota);
  const maxQuotaValue = maxQuota === '' ? undefined : Number(maxQuota);
  const maxKeysValue = Number(maxKeys);
  const defaultTTLValue = defaultTTLHours === '' ? undefined : Number(defaultTTLHours);
  const maxTTLValue = maxTTLHours === '' ? undefined : Number(maxTTLHours);
  const policyInvalid =
    !Number.isInteger(maxKeysValue) ||
    maxKeysValue < 1 ||
    maxKeysValue > 100 ||
    (defaultQuotaValue !== undefined &&
      (!Number.isFinite(defaultQuotaValue) || defaultQuotaValue < 0)) ||
    (maxQuotaValue !== undefined && (!Number.isFinite(maxQuotaValue) || maxQuotaValue < 0)) ||
    (defaultQuotaValue !== undefined &&
      maxQuotaValue !== undefined &&
      defaultQuotaValue > maxQuotaValue) ||
    (defaultTTLValue !== undefined &&
      (!Number.isFinite(defaultTTLValue) || defaultTTLValue <= 0)) ||
    (maxTTLValue !== undefined && (!Number.isFinite(maxTTLValue) || maxTTLValue <= 0)) ||
    (defaultTTLValue !== undefined && maxTTLValue !== undefined && defaultTTLValue > maxTTLValue);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('policy')}</DialogTitle>
          <DialogDescription>{t('policyDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="policy-mode">{t('mode')}</Label>
            <select
              id="policy-mode"
              className="h-10 w-full rounded-md border bg-background px-3 text-sm"
              value={mode}
              onChange={event => setMode(event.target.value as DeveloperAccessMode)}
            >
              <option value="self_service">{t('modes.self_service')}</option>
              <option value="approval_required">{t('modes.approval_required')}</option>
              <option value="disabled">{t('modes.disabled')}</option>
            </select>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="policy-quota">{t('labels.defaultQuota')}</Label>
              <Input
                id="policy-quota"
                type="number"
                min={0}
                value={defaultQuota}
                onChange={event => setDefaultQuota(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="policy-max-quota">{t('labels.maxQuota')}</Label>
              <Input
                id="policy-max-quota"
                type="number"
                min={0}
                value={maxQuota}
                onChange={event => setMaxQuota(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="policy-max-keys">{t('labels.maxKeys')}</Label>
              <Input
                id="policy-max-keys"
                type="number"
                min={1}
                max={100}
                value={maxKeys}
                onChange={event => setMaxKeys(event.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="policy-default-ttl">{t('labels.defaultTTL')}</Label>
              <Input
                id="policy-default-ttl"
                type="number"
                min={1}
                value={defaultTTLHours}
                onChange={event => setDefaultTTLHours(event.target.value)}
                placeholder={t('placeholders.ttlHours')}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="policy-max-ttl">{t('labels.maxTTL')}</Label>
              <Input
                id="policy-max-ttl"
                type="number"
                min={1}
                value={maxTTLHours}
                onChange={event => setMaxTTLHours(event.target.value)}
                placeholder={t('placeholders.ttlHours')}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="policy-models">{t('labels.allowedModelNames')}</Label>
            <Input
              id="policy-models"
              value={models}
              onChange={event => setModels(event.target.value)}
              placeholder={t('placeholders.models')}
            />
            <p className="text-xs text-muted-foreground">{t('review.modelsHint')}</p>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
          <Button
            disabled={pending || policyInvalid}
            onClick={() =>
              onSubmit({
                ...policy,
                mode,
                default_quota: defaultQuotaValue ?? null,
                max_quota: maxQuotaValue ?? null,
                max_keys: maxKeysValue,
                default_ttl_seconds: defaultTTLValue ? defaultTTLValue * 3600 : null,
                max_ttl_seconds: maxTTLValue ? maxTTLValue * 3600 : null,
                allowed_models: splitModels(models),
              })
            }
          >
            {t('actions.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
