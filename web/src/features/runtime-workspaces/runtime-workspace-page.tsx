'use client';

import * as React from 'react';
import { useRouter } from 'next/navigation';
import {
  ArrowRight,
  AlertTriangle,
  Check,
  CheckCircle2,
  Copy,
  Bot,
  Brain,
  Eye,
  FileText,
  KeyRound,
  Link2,
  Loader2,
  MoreHorizontal,
  Plus,
  Power,
  RefreshCw,
  Search,
  ShieldCheck,
  Tags,
  Trash2,
  Unplug,
} from 'lucide-react';
import { toast } from 'sonner';
import { API_URL, ENABLE_RUNTIME_WORKSPACE, ICON_BG, ICON_TEXT } from '@/lib/config';
import { useT } from '@/i18n';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useRuntimeWorkspace } from '@/hooks/runtime-workspace/use-runtime-workspace';
import { getErrorMessage } from '@/utils/error-notifications';
import { cn } from '@/lib/utils';
import type {
  RuntimeAgentCandidate,
  RuntimeCatalogTag,
  RuntimeCredential,
  RuntimeApp,
  RuntimeSourceWorkspace,
} from '@/services/types/runtime-workspace';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

function formatDate(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
    date
  );
}

function formatShortDate(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date);
}

function statusBadge(status: string, activeLabel: string, inactiveLabel: string) {
  return (
    <Badge variant={status === 'active' ? 'success' : 'subtle'}>
      {status === 'active' ? activeLabel : inactiveLabel}
    </Badge>
  );
}

function SectionHeader({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 @3xl/console:flex-row @3xl/console:items-start @3xl/console:justify-between">
      <div>
        <CardTitle className="text-lg">{title}</CardTitle>
        <CardDescription className="mt-1 max-w-2xl leading-6">{description}</CardDescription>
      </div>
      {action}
    </div>
  );
}

function CopyButton({
  value,
  label,
  copiedLabel,
  errorLabel,
}: {
  value: string;
  label: string;
  copiedLabel: string;
  errorLabel: string;
}) {
  const [copied, setCopied] = React.useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      toast.error(errorLabel);
    }
  };

  return (
    <Button type="button" variant="outline" size="sm" onClick={() => void handleCopy()}>
      {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
      {copied ? copiedLabel : label}
    </Button>
  );
}

function EmptyState({
  title,
  description,
  icon: Icon,
}: {
  title: string;
  description: string;
  icon: React.ElementType;
}) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center rounded-xl border border-dashed bg-muted/20 px-6 text-center">
      <div className="mb-3 flex size-10 items-center justify-center rounded-xl border bg-background shadow-sm">
        <Icon className="size-5 text-muted-foreground" />
      </div>
      <p className="font-medium">{title}</p>
      <p className="mt-1 max-w-md text-sm leading-6 text-muted-foreground">{description}</p>
    </div>
  );
}

function OverviewTab({
  totalAppCount,
  callableAppCount,
  usableCredentialCount,
  runtimeEndpoint,
  isActive,
  isUpdating,
  onEnable,
  onOpenSection,
}: {
  totalAppCount: number;
  callableAppCount: number;
  usableCredentialCount: number;
  runtimeEndpoint: string;
  isActive: boolean;
  isUpdating: boolean;
  onEnable: () => void;
  onOpenSection: (section: 'agents' | 'keys' | 'docs') => void;
}) {
  const t = useT('workspace');
  const ready = isActive && callableAppCount > 0 && usableCredentialCount > 0;
  const readiness = !isActive
    ? {
        title: t('runtime.overview.readiness.pausedTitle'),
        description: t('runtime.overview.readiness.pausedDescription'),
        action: t('runtime.status.enableAction'),
        tab: null,
      }
    : callableAppCount === 0
      ? {
          title: t('runtime.overview.readiness.agentTitle'),
          description: t('runtime.overview.readiness.agentDescription'),
          action: t('runtime.apps.bind'),
          tab: 'agents' as const,
        }
      : usableCredentialCount === 0
        ? {
            title: t('runtime.overview.readiness.keyTitle'),
            description: t('runtime.overview.readiness.keyDescription'),
            action: t('runtime.credentials.create'),
            tab: 'keys' as const,
          }
        : {
            title: t('runtime.overview.readiness.readyTitle'),
            description: t('runtime.overview.readiness.readyDescription', {
              agents: callableAppCount,
              keys: usableCredentialCount,
            }),
            action: t('runtime.overview.readiness.readyAction'),
            tab: 'docs' as const,
          };
  const steps = [
    { label: t('runtime.overview.steps.enabled'), complete: isActive },
    { label: t('runtime.overview.steps.agent'), complete: callableAppCount > 0 },
    { label: t('runtime.overview.steps.key'), complete: usableCredentialCount > 0 },
  ];

  return (
    <div className="space-y-5">
      <Card
        className={cn(
          'overflow-hidden border-border/80 shadow-sm',
          ready
            ? 'border-primary/25 bg-primary/[0.04]'
            : !isActive
              ? 'border-warning/40 bg-warning/5'
              : 'border-primary/30 bg-primary/5'
        )}
      >
        <CardContent className="flex flex-col gap-4 p-5 @3xl/console:flex-row @3xl/console:items-center @3xl/console:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
              {ready ? <CheckCircle2 className="size-5" /> : <ShieldCheck className="size-5" />}
            </div>
            <div className="min-w-0">
              <h2 className="text-base font-semibold">{readiness.title}</h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                {readiness.description}
              </p>
            </div>
          </div>
          {!ready ? (
            <Button
              type="button"
              className="shrink-0 self-start @3xl/console:self-center"
              onClick={() => (readiness.tab ? onOpenSection(readiness.tab) : onEnable())}
              disabled={isUpdating}
            >
              {isUpdating && !isActive ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <ArrowRight className="size-4" />
              )}
              {readiness.action}
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <div className="grid gap-4 @3xl/console:grid-cols-2">
        <button type="button" className="text-left" onClick={() => onOpenSection('agents')}>
          <Card className="h-full border-border/80 transition-colors hover:border-primary/40 hover:bg-muted/20">
            <CardContent className="p-5">
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">{t('runtime.overview.agents')}</span>
                <Link2 className="size-4 text-muted-foreground" />
              </div>
              <div className="mt-4 text-2xl font-semibold">{callableAppCount}</div>
              <div className="mt-1 flex items-center justify-between gap-3 text-xs text-muted-foreground">
                <span>{t('runtime.overview.agentsDescription', { total: totalAppCount })}</span>
                <ArrowRight className="size-3.5 shrink-0" />
              </div>
            </CardContent>
          </Card>
        </button>
        <Card className="border-border/80">
          <CardContent className="p-5">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{t('runtime.overview.endpoint')}</span>
              <ShieldCheck className="size-4 text-muted-foreground" />
            </div>
            <code className="mt-4 block truncate text-sm font-semibold" title={runtimeEndpoint}>
              {runtimeEndpoint}
            </code>
            <div className="mt-3">
              <CopyButton
                value={runtimeEndpoint}
                label={t('runtime.overview.copyEndpoint')}
                copiedLabel={t('runtime.integration.copied')}
                errorLabel={t('runtime.integration.copyFailed')}
              />
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="border-border/80 shadow-sm">
        <CardHeader>
          <SectionHeader
            title={t('runtime.overview.steps.title')}
            description={t('runtime.overview.steps.description')}
          />
        </CardHeader>
        <CardContent className="grid gap-3 @3xl/console:grid-cols-3">
          {steps.map((step, index) => (
            <div
              key={step.label}
              className="flex items-center gap-3 rounded-lg border border-border/70 px-3 py-2.5"
            >
              <div
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold',
                  step.complete ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'
                )}
              >
                {step.complete ? <Check className="size-3.5" /> : index + 1}
              </div>
              <span
                className={cn(
                  'text-sm',
                  step.complete ? 'text-foreground' : 'text-muted-foreground'
                )}
              >
                {step.label}
              </span>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

type RuntimeAgentPresentation = Pick<
  RuntimeAgentCandidate,
  'name' | 'icon' | 'icon_type' | 'icon_url'
>;

function RuntimeAgentIcon({ agent }: { agent: RuntimeAgentPresentation }) {
  let icon = agent.name.slice(0, 2).toUpperCase() || ICON_TEXT;
  let background = ICON_BG;
  try {
    const parsed = JSON.parse(agent.icon || '{}');
    icon = parsed?.icon || icon;
    background = parsed?.icon_background || background;
  } catch {
    background = ICON_BG;
  }
  return (
    <IconPreview
      icon={icon}
      iconType={agent.icon_type}
      src={agent.icon_type === 'image' ? agent.icon_url || '' : ''}
      iconBackground={agent.icon_type === 'image' ? '' : background}
      editable={false}
      size="sm"
    />
  );
}

function CapabilityBadges({ capabilities }: { capabilities: string[] }) {
  const t = useT('workspace');
  const icons: Record<string, React.ElementType> = {
    files: FileText,
    memory: Brain,
    vision: Eye,
    chat: Bot,
  };
  return (
    <div className="flex flex-wrap gap-1.5">
      {capabilities
        .filter(capability => capability !== 'conversations')
        .map(capability => {
          const Icon = icons[capability] ?? CheckCircle2;
          return (
            <Badge key={capability} variant="subtle" className="gap-1 text-[11px] font-normal">
              <Icon className="size-3" />
              {t(`runtime.apps.capabilities.${capability}` as never)}
            </Badge>
          );
        })}
    </div>
  );
}

function CatalogTagBadges({ tags = [] }: { tags?: RuntimeCatalogTag[] }) {
  const t = useT('workspace');
  if (tags.length === 0) return null;
  const visibleTags = tags.slice(0, 3);
  const hiddenCount = tags.length - visibleTags.length;
  return (
    <div
      className="flex min-h-6 items-center gap-1 overflow-hidden"
      aria-label={t('runtime.apps.categories')}
      title={tags.map(tag => tag.name).join(', ')}
    >
      <Tags className="size-3.5 shrink-0 text-muted-foreground" />
      {visibleTags.map(tag => (
        <Badge
          key={tag.id}
          variant="outline"
          className="max-w-28 truncate rounded-full border-primary/20 bg-primary/5 px-2 py-0 text-[10px] font-normal text-primary"
        >
          {tag.name}
        </Badge>
      ))}
      {hiddenCount > 0 ? (
        <Badge
          variant="outline"
          className="rounded-full px-1.5 py-0 text-[10px] font-normal text-muted-foreground"
        >
          +{hiddenCount}
        </Badge>
      ) : null}
    </div>
  );
}

function AppsTab({
  revision,
  apps,
  candidates,
  crossWorkspaceCandidates,
  crossWorkspaceEnabled,
  sourceWorkspaces,
  hostWorkspaceId,
  onSync,
  isSyncing,
}: {
  revision: number;
  apps: RuntimeApp[];
  candidates: RuntimeAgentCandidate[];
  crossWorkspaceCandidates: RuntimeAgentCandidate[];
  crossWorkspaceEnabled: boolean;
  sourceWorkspaces: RuntimeSourceWorkspace[];
  hostWorkspaceId: string;
  onSync: (input: { expected_revision: number; agent_ids: string[] }) => Promise<void>;
  isSyncing: boolean;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState('');
  const [boundQuery, setBoundQuery] = React.useState('');
  const [candidateScope, setCandidateScope] = React.useState<'current' | 'authorized'>('current');
  const [sourceWorkspaceFilter, setSourceWorkspaceFilter] = React.useState('all');
  const [selectedCandidateIds, setSelectedCandidateIds] = React.useState<Set<string>>(new Set());
  const [batchMode, setBatchMode] = React.useState(false);
  const [selectedBoundIds, setSelectedBoundIds] = React.useState<Set<string>>(new Set());
  const [pendingRemoveIds, setPendingRemoveIds] = React.useState<Set<string> | null>(null);
  const activeApps = React.useMemo(() => apps.filter(app => app.status === 'active'), [apps]);
  const filteredApps = React.useMemo(() => {
    const normalizedQuery = boundQuery.trim().toLowerCase();
    if (!normalizedQuery) return apps;
    return apps.filter(app =>
      [
        app.name,
        app.description,
        app.workspace_name,
        app.app_key,
        ...(app.tags ?? []).map(tag => tag.name),
      ].some(value => value?.toLowerCase().includes(normalizedQuery))
    );
  }, [apps, boundQuery]);
  const activeAgentIds = React.useMemo(
    () => new Set(activeApps.map(app => app.agent_id)),
    [activeApps]
  );
  const allCandidates = React.useMemo(
    () => [...candidates, ...crossWorkspaceCandidates],
    [candidates, crossWorkspaceCandidates]
  );
  const availableCandidates = React.useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    const scoped =
      candidateScope === 'current'
        ? candidates
        : crossWorkspaceCandidates.filter(
            candidate =>
              sourceWorkspaceFilter === 'all' || candidate.workspace_id === sourceWorkspaceFilter
          );
    return scoped.filter(candidate => {
      if (activeAgentIds.has(candidate.agent_id)) return false;
      if (!normalizedQuery) return true;
      return [
        candidate.name,
        candidate.description,
        candidate.workspace_name,
        candidate.version,
        ...(candidate.tags ?? []).map(tag => tag.name),
      ].some(value => value?.toLowerCase().includes(normalizedQuery));
    });
  }, [
    activeAgentIds,
    candidateScope,
    candidates,
    crossWorkspaceCandidates,
    query,
    sourceWorkspaceFilter,
  ]);
  const toggleID = (
    source: Set<string>,
    id: string,
    update: React.Dispatch<React.SetStateAction<Set<string>>>
  ) => {
    const next = new Set(source);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    update(next);
  };
  const resetAdd = () => {
    setSelectedCandidateIds(new Set());
    setQuery('');
    setCandidateScope('current');
    setSourceWorkspaceFilter('all');
  };
  const submitAdd = async () => {
    if (selectedCandidateIds.size === 0) return;
    await onSync({
      expected_revision: revision,
      agent_ids: [...activeAgentIds, ...selectedCandidateIds],
    });
    toast.success(t('runtime.messages.agentsAdded', { count: selectedCandidateIds.size }));
    resetAdd();
    setOpen(false);
  };
  const confirmRemove = async () => {
    if (!pendingRemoveIds?.size) return;
    const desired = [...activeAgentIds].filter(agentID => !pendingRemoveIds.has(agentID));
    await onSync({ expected_revision: revision, agent_ids: desired });
    toast.success(t('runtime.messages.agentsRemoved', { count: pendingRemoveIds.size }));
    setPendingRemoveIds(null);
    setSelectedBoundIds(new Set());
    setBatchMode(false);
  };
  const removeApps = pendingRemoveIds
    ? activeApps.filter(app => pendingRemoveIds.has(app.agent_id))
    : [];
  const removeGroups = removeApps.reduce<Record<string, { name: string; items: RuntimeApp[] }>>(
    (groups, app) => {
      const group = groups[app.workspace_id] ?? { name: app.workspace_name, items: [] };
      group.items.push(app);
      groups[app.workspace_id] = group;
      return groups;
    },
    {}
  );
  const selectedCandidates = allCandidates.filter(candidate =>
    selectedCandidateIds.has(candidate.agent_id)
  );
  const selectedGroups = selectedCandidates.reduce<
    Record<string, { name: string; items: RuntimeAgentCandidate[] }>
  >((groups, candidate) => {
    const group = groups[candidate.workspace_id] ?? { name: candidate.workspace_name, items: [] };
    group.items.push(candidate);
    groups[candidate.workspace_id] = group;
    return groups;
  }, {});

  return (
    <section className="space-y-4">
      <SectionHeader
        title={t('runtime.apps.titleWithCount', { count: activeApps.length })}
        description={t('runtime.apps.description')}
        action={
          <div className="flex flex-wrap items-center gap-2">
            {activeApps.length > 0 ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setBatchMode(value => !value);
                  setSelectedBoundIds(new Set());
                }}
                disabled={isSyncing}
              >
                {batchMode ? tCommon('cancel') : t('runtime.apps.batchManage')}
              </Button>
            ) : null}
            <Dialog
              open={open}
              onOpenChange={value => {
                setOpen(value);
                if (!value) resetAdd();
              }}
            >
              <Button
                type="button"
                onClick={() => setOpen(true)}
                disabled={allCandidates.length === 0 || isSyncing}
              >
                <Plus className="size-4" />
                {t('runtime.apps.bind')}
              </Button>
              <DialogContent className="max-w-5xl">
                <DialogHeader>
                  <DialogTitle>{t('runtime.apps.bindTitle')}</DialogTitle>
                  <DialogDescription>{t('runtime.apps.bindDescription')}</DialogDescription>
                </DialogHeader>
                <DialogBody className="space-y-4">
                  <Tabs
                    value={candidateScope}
                    onValueChange={value => setCandidateScope(value as 'current' | 'authorized')}
                  >
                    <TabsList>
                      <TabsTrigger value="current">
                        {t('runtime.apps.currentWorkspace')}
                      </TabsTrigger>
                      {crossWorkspaceEnabled ? (
                        <TabsTrigger value="authorized">
                          {t('runtime.apps.otherWorkspaces')}
                        </TabsTrigger>
                      ) : null}
                    </TabsList>
                  </Tabs>
                  {candidateScope === 'authorized' ? (
                    <div className="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-3">
                      <p className="text-xs leading-5 text-muted-foreground">
                        {t('runtime.apps.crossWorkspaceNote')}
                      </p>
                      <Select
                        value={sourceWorkspaceFilter}
                        onValueChange={setSourceWorkspaceFilter}
                      >
                        <SelectTrigger className="w-full sm:w-72">
                          <SelectValue placeholder={t('runtime.apps.sourceWorkspace')} />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">
                            {t('runtime.apps.allSourceWorkspaces')}
                          </SelectItem>
                          {sourceWorkspaces.map(workspace => (
                            <SelectItem key={workspace.id} value={workspace.id}>
                              {workspace.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      {t('runtime.apps.currentWorkspaceDescription')}
                    </p>
                  )}
                  <div className="flex items-center gap-3">
                    <div className="relative min-w-0 flex-1">
                      <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                      <Input
                        value={query}
                        onChange={event => setQuery(event.target.value)}
                        placeholder={t('runtime.apps.searchPlaceholder')}
                        className="pl-9"
                        disabled={isSyncing}
                      />
                    </div>
                    <Badge variant="subtle">
                      {t('runtime.apps.selectedCount', { count: selectedCandidateIds.size })}
                    </Badge>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 text-xs">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        setSelectedCandidateIds(
                          current =>
                            new Set([
                              ...current,
                              ...availableCandidates.map(candidate => candidate.agent_id),
                            ])
                        )
                      }
                      disabled={availableCandidates.length === 0 || isSyncing}
                    >
                      {t('runtime.apps.selectAllResults')}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setSelectedCandidateIds(new Set())}
                      disabled={selectedCandidateIds.size === 0 || isSyncing}
                    >
                      {t('runtime.apps.clearSelection')}
                    </Button>
                  </div>
                  <div className="-m-1 grid max-h-[55vh] grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2.5 overflow-y-auto p-1">
                    {availableCandidates.map(candidate => {
                      const selected = selectedCandidateIds.has(candidate.agent_id);
                      return (
                        <div
                          key={candidate.agent_id}
                          role="checkbox"
                          tabIndex={isSyncing ? -1 : 0}
                          aria-checked={selected}
                          aria-disabled={isSyncing}
                          onClick={() => {
                            if (!isSyncing)
                              toggleID(
                                selectedCandidateIds,
                                candidate.agent_id,
                                setSelectedCandidateIds
                              );
                          }}
                          onKeyDown={event => {
                            if (!isSyncing && (event.key === 'Enter' || event.key === ' ')) {
                              event.preventDefault();
                              toggleID(
                                selectedCandidateIds,
                                candidate.agent_id,
                                setSelectedCandidateIds
                              );
                            }
                          }}
                          className="h-full w-full cursor-pointer text-left focus-visible:outline-none"
                        >
                          <Card
                            className={cn(
                              'h-full min-h-[184px] border transition-colors hover:border-primary/50 hover:bg-muted/20',
                              selected &&
                                'border-primary bg-primary/[0.04] ring-2 ring-inset ring-primary/70'
                            )}
                          >
                            <CardContent className="flex h-full flex-col gap-2.5 p-3.5">
                              <div className="flex items-start justify-between gap-3">
                                <RuntimeAgentIcon agent={candidate} />
                                <Checkbox checked={selected} tabIndex={-1} aria-hidden />
                              </div>
                              <div>
                                <div className="line-clamp-1 font-semibold" title={candidate.name}>
                                  {candidate.name}
                                </div>
                                <div className="mt-1 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
                                  {candidate.description || t('runtime.apps.noDescription')}
                                </div>
                              </div>
                              <div className="space-y-1 text-xs text-muted-foreground">
                                {candidateScope === 'authorized' ? (
                                  <div
                                    className="truncate font-medium text-foreground/80"
                                    title={candidate.workspace_name}
                                  >
                                    {t('runtime.apps.sourceWorkspaceLabel', {
                                      name: candidate.workspace_name,
                                    })}
                                  </div>
                                ) : null}
                                <div className="flex min-w-0 items-center gap-1">
                                  <span className="truncate" title={candidate.version}>
                                    {candidate.version}
                                  </span>
                                  <span className="shrink-0">
                                    · {formatShortDate(candidate.published_at)}
                                  </span>
                                </div>
                              </div>
                              <CatalogTagBadges tags={candidate.tags} />
                              <div className="mt-auto">
                                <CapabilityBadges capabilities={candidate.capabilities} />
                              </div>
                            </CardContent>
                          </Card>
                        </div>
                      );
                    })}
                  </div>
                  {availableCandidates.length === 0 ? (
                    <div className="py-8 text-center text-sm text-muted-foreground">
                      {t('runtime.apps.noSearchResults')}
                    </div>
                  ) : null}
                  {selectedCandidates.length > 0 ? (
                    <div className="max-h-32 space-y-2 overflow-y-auto rounded-lg border bg-muted/20 p-3">
                      {Object.entries(selectedGroups).map(([workspaceID, group]) => (
                        <div key={workspaceID} className="text-xs">
                          <span className="font-medium">
                            {workspaceID === hostWorkspaceId
                              ? t('runtime.apps.currentWorkspace')
                              : group.name}
                          </span>
                          <span className="ml-2 text-muted-foreground">
                            {group.items.map(item => item.name).join('、')}
                          </span>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </DialogBody>
                <DialogFooter>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => setOpen(false)}
                    disabled={isSyncing}
                  >
                    {tCommon('cancel')}
                  </Button>
                  <Button
                    type="button"
                    onClick={() => void submitAdd()}
                    disabled={selectedCandidateIds.size === 0 || isSyncing}
                  >
                    {isSyncing ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <Link2 className="size-4" />
                    )}
                    {t('runtime.apps.addSelected', { count: selectedCandidateIds.size })}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </div>
        }
      />
      {apps.length > 0 ? (
        <div className="relative max-w-sm">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={boundQuery}
            onChange={event => setBoundQuery(event.target.value)}
            placeholder={t('runtime.apps.searchBoundPlaceholder')}
            className="pl-9"
          />
        </div>
      ) : null}
      <div className="space-y-4">
        {batchMode ? (
          <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/20 bg-background/95 p-3 shadow-sm backdrop-blur">
            <span className="text-sm font-medium">
              {t('runtime.apps.selectedCount', { count: selectedBoundIds.size })}
            </span>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() =>
                  setSelectedBoundIds(
                    new Set(
                      filteredApps.filter(app => app.status === 'active').map(app => app.agent_id)
                    )
                  )
                }
                disabled={isSyncing}
              >
                {t('runtime.apps.selectAllResults')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setSelectedBoundIds(new Set())}
                disabled={selectedBoundIds.size === 0 || isSyncing}
              >
                {t('runtime.apps.clearSelection')}
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                onClick={() => setPendingRemoveIds(new Set(selectedBoundIds))}
                disabled={selectedBoundIds.size === 0 || isSyncing}
              >
                <Trash2 className="size-3.5" />
                {t('runtime.apps.removeSelected', { count: selectedBoundIds.size })}
              </Button>
            </div>
          </div>
        ) : null}
        {apps.length === 0 ? (
          <EmptyState
            icon={Link2}
            title={t('runtime.apps.empty')}
            description={
              allCandidates.length === 0
                ? t('runtime.apps.noCandidates')
                : t('runtime.apps.emptyDescription')
            }
          />
        ) : filteredApps.length === 0 ? (
          <div className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
            {t('runtime.apps.noSearchResults')}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 @3xl/console:grid-cols-2 @7xl/console:grid-cols-3">
            {filteredApps.map(app => {
              const selected = selectedBoundIds.has(app.agent_id);
              const selectable = batchMode && app.status === 'active';
              const crossWorkspace = app.workspace_id !== hostWorkspaceId;
              return (
                <Card
                  key={`${app.agent_id}:${app.app_key}`}
                  role={batchMode ? 'checkbox' : undefined}
                  tabIndex={selectable && !isSyncing ? 0 : undefined}
                  aria-checked={batchMode ? selected : undefined}
                  onClick={() => {
                    if (selectable && !isSyncing)
                      toggleID(selectedBoundIds, app.agent_id, setSelectedBoundIds);
                  }}
                  onKeyDown={event => {
                    if (selectable && !isSyncing && (event.key === 'Enter' || event.key === ' ')) {
                      event.preventDefault();
                      toggleID(selectedBoundIds, app.agent_id, setSelectedBoundIds);
                    }
                  }}
                  className={cn(
                    'flex min-h-56 flex-col border-border/80 transition-colors focus-visible:outline-none',
                    selectable && 'cursor-pointer hover:border-primary/50',
                    selected &&
                      'border-primary bg-primary/[0.04] ring-2 ring-inset ring-primary/70',
                    isSyncing && selected && 'opacity-60'
                  )}
                >
                  <CardContent className="flex h-full flex-col gap-3 p-4">
                    <div className="flex items-start gap-3">
                      <RuntimeAgentIcon agent={app} />
                      <div className="min-w-0 flex-1">
                        <div className="line-clamp-1 font-semibold" title={app.name}>
                          {app.name}
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                          {crossWorkspace ? (
                            <Badge variant="subtle" className="max-w-full truncate font-normal">
                              {t('runtime.apps.sourceWorkspaceLabel', { name: app.workspace_name })}
                            </Badge>
                          ) : null}
                          <span title={app.version}>{formatShortDate(app.published_at)}</span>
                          {!app.callable ? (
                            <Badge variant="warning">{t('runtime.apps.unavailable')}</Badge>
                          ) : null}
                        </div>
                      </div>
                      {batchMode ? (
                        <Checkbox
                          checked={selected}
                          disabled={!selectable || isSyncing}
                          tabIndex={-1}
                          aria-hidden
                        />
                      ) : (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              className="size-8 shrink-0 px-0"
                              aria-label={t('runtime.apps.actions')}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              className="text-destructive focus:text-destructive"
                              onSelect={() => setPendingRemoveIds(new Set([app.agent_id]))}
                            >
                              {t('runtime.apps.unbind')}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      )}
                    </div>
                    <p className="line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
                      {app.description || t('runtime.apps.noDescription')}
                    </p>
                    <CatalogTagBadges tags={app.tags} />
                    <CapabilityBadges capabilities={app.capabilities} />
                    <div className="mt-auto flex min-w-0 items-center gap-3 border-t border-border/70 pt-3">
                      <div className="min-w-0 flex-1">
                        <div className="text-[11px] font-medium text-muted-foreground">
                          {t('runtime.apps.appId')}
                        </div>
                        <code className="mt-1 block truncate text-xs" title={app.app_key}>
                          {app.app_key}
                        </code>
                      </div>
                      {!batchMode ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="size-8 shrink-0 px-0"
                          aria-label={t('runtime.apps.copyAppId')}
                          title={t('runtime.apps.copyAppId')}
                          onClick={event => {
                            event.stopPropagation();
                            void navigator.clipboard
                              .writeText(app.app_key)
                              .then(() => toast.success(t('runtime.credentials.copied')))
                              .catch(() => toast.error(t('runtime.credentials.copyFailed')));
                          }}
                        >
                          <Copy className="size-3.5" />
                        </Button>
                      ) : null}
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        )}
      </div>
      <ConfirmDialog
        open={Boolean(pendingRemoveIds?.size)}
        onOpenChange={value => {
          if (!value) setPendingRemoveIds(null);
        }}
        title={t('runtime.apps.batchRemoveTitle', { count: pendingRemoveIds?.size ?? 0 })}
        description={
          <div className="space-y-3">
            <p>
              {t('runtime.apps.batchRemoveDescription', { count: pendingRemoveIds?.size ?? 0 })}
            </p>
            <div className="max-h-36 space-y-2 overflow-y-auto rounded-md border bg-muted/20 p-3">
              {Object.entries(removeGroups).map(([workspaceID, group]) => (
                <div key={workspaceID} className="text-xs">
                  <div className="font-medium">
                    {workspaceID === hostWorkspaceId
                      ? t('runtime.apps.currentWorkspace')
                      : group.name}
                  </div>
                  <div className="mt-1 text-muted-foreground">
                    {group.items.map(item => item.name).join('、')}
                  </div>
                </div>
              ))}
            </div>
          </div>
        }
        confirmText={t('runtime.apps.removeSelected', { count: pendingRemoveIds?.size ?? 0 })}
        cancelText={tCommon('cancel')}
        variant="warning"
        loading={isSyncing}
        closeOnConfirm={false}
        onConfirm={() => {
          void confirmRemove();
        }}
      />
    </section>
  );
}

function CredentialsTab({
  credentials,
  onCreate,
  onRevoke,
  isCreating,
  isRevoking,
}: {
  credentials: RuntimeCredential[];
  onCreate: (input: {
    name?: string;
    expires_at?: string;
    idempotency_key: string;
  }) => Promise<string>;
  onRevoke: (credentialId: string) => Promise<void>;
  isCreating: boolean;
  isRevoking: boolean;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [open, setOpen] = React.useState(false);
  const [name, setName] = React.useState('');
  const [expiresAt, setExpiresAt] = React.useState('');
  const [secret, setSecret] = React.useState<string | null>(null);
  const [revokeId, setRevokeId] = React.useState<string | null>(null);
  const [idempotencyKey, setIdempotencyKey] = React.useState<string | null>(null);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const operationKey = idempotencyKey ?? crypto.randomUUID();
    setIdempotencyKey(operationKey);
    const generatedSecret = await onCreate({
      ...(name.trim() ? { name: name.trim() } : {}),
      ...(expiresAt ? { expires_at: new Date(expiresAt).toISOString() } : {}),
      idempotency_key: operationKey,
    });
    setName('');
    setExpiresAt('');
    setIdempotencyKey(null);
    setOpen(false);
    setSecret(generatedSecret);
  };

  return (
    <Card className="border-border/80 shadow-sm">
      <CardHeader>
        <SectionHeader
          title={t('runtime.credentials.title')}
          description={t('runtime.credentials.description')}
          action={
            <Dialog
              open={open}
              onOpenChange={value => {
                setOpen(value);
                if (!value) setIdempotencyKey(null);
              }}
            >
              <Button type="button" onClick={() => setOpen(true)}>
                <Plus className="size-4" />
                {t('runtime.credentials.create')}
              </Button>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>{t('runtime.credentials.createTitle')}</DialogTitle>
                  <DialogDescription>
                    {t('runtime.credentials.createDescription')}
                  </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit}>
                  <DialogBody className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="runtime-key-name">{t('runtime.credentials.name')}</Label>
                      <Input
                        id="runtime-key-name"
                        value={name}
                        onChange={event => {
                          setName(event.target.value);
                          setIdempotencyKey(null);
                        }}
                        placeholder={t('runtime.credentials.namePlaceholder')}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-key-expires">
                        {t('runtime.credentials.expiresAt')}
                      </Label>
                      <Input
                        id="runtime-key-expires"
                        type="datetime-local"
                        value={expiresAt}
                        onChange={event => {
                          setExpiresAt(event.target.value);
                          setIdempotencyKey(null);
                        }}
                        aria-label={t('runtime.credentials.expiresAtPlaceholder')}
                      />
                    </div>
                  </DialogBody>
                  <DialogFooter>
                    <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
                      {tCommon('cancel')}
                    </Button>
                    <Button type="submit" disabled={isCreating}>
                      {isCreating ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <KeyRound className="size-4" />
                      )}
                      {t('runtime.credentials.createConfirm')}
                    </Button>
                  </DialogFooter>
                </form>
              </DialogContent>
            </Dialog>
          }
        />
      </CardHeader>
      <CardContent>
        {credentials.length === 0 ? (
          <EmptyState
            icon={KeyRound}
            title={t('runtime.credentials.empty')}
            description={t('runtime.credentials.emptyDescription')}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('runtime.credentials.name')}</TableHead>
                <TableHead>{t('runtime.credentials.prefix')}</TableHead>
                <TableHead>{t('runtime.credentials.status')}</TableHead>
                <TableHead>{t('runtime.credentials.createdAt')}</TableHead>
                <TableHead>{t('runtime.credentials.lastUsed')}</TableHead>
                <TableHead>{t('runtime.credentials.expiresAt')}</TableHead>
                <TableHead className="text-right">{t('runtime.credentials.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {credentials.map(credential => (
                <TableRow key={credential.id}>
                  <TableCell className="font-medium">{credential.name || 'Runtime Key'}</TableCell>
                  <TableCell>
                    <code className="rounded bg-muted px-2 py-1 text-xs">
                      {credential.key_prefix}
                    </code>
                  </TableCell>
                  <TableCell>
                    {statusBadge(
                      credential.status,
                      t('runtime.credentials.active'),
                      t('runtime.credentials.revoked')
                    )}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {formatDate(credential.created_at)}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {formatDate(credential.last_used_at) === '—'
                      ? t('runtime.credentials.never')
                      : formatDate(credential.last_used_at)}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {credential.expires_at
                      ? formatDate(credential.expires_at)
                      : t('runtime.credentials.noExpiry')}
                  </TableCell>
                  <TableCell className="text-right">
                    {credential.status === 'active' ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setRevokeId(credential.id)}
                        disabled={isRevoking}
                      >
                        {t('runtime.credentials.revoke')}
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
      <Dialog
        open={Boolean(secret)}
        onOpenChange={value => {
          if (!value) setSecret(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('runtime.credentials.secretTitle')}</DialogTitle>
            <DialogDescription>{t('runtime.credentials.secretDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <div className="rounded-lg border bg-muted/20 p-3">
              <code className="block break-all text-sm leading-6">{secret}</code>
            </div>
          </DialogBody>
          <DialogFooter>
            <CopyButton
              value={secret ?? ''}
              label={t('runtime.credentials.copy')}
              copiedLabel={t('runtime.credentials.copied')}
              errorLabel={t('runtime.credentials.copyFailed')}
            />
            <Button type="button" onClick={() => setSecret(null)}>
              {t('runtime.credentials.saved')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={Boolean(revokeId)}
        onOpenChange={value => {
          if (!value) setRevokeId(null);
        }}
        title={t('runtime.credentials.revokeTitle')}
        description={t('runtime.credentials.revokeDescription')}
        confirmText={t('runtime.credentials.confirmRevoke')}
        cancelText={tCommon('cancel')}
        variant="danger"
        loading={isRevoking}
        closeOnConfirm={false}
        onConfirm={() => {
          if (!revokeId) return;
          void onRevoke(revokeId).then(() => setRevokeId(null));
        }}
      />
    </Card>
  );
}

export type RuntimeWorkspaceSection = 'overview' | 'agents';
type RuntimeWorkspaceTarget = RuntimeWorkspaceSection | 'keys' | 'docs';

export default function RuntimeWorkspacePage({
  section = 'overview',
}: {
  section?: RuntimeWorkspaceSection;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const currentWorkspace = useCurrentWorkspace();
  const { isWorkspaceManager, isLoading: permissionsLoading } = useAccountPermissions();
  const workspaceId = currentWorkspace?.id ?? null;
  const canManage = ENABLE_RUNTIME_WORKSPACE && isWorkspaceManager();
  const runtime = useRuntimeWorkspace(canManage ? workspaceId : null);
  const router = useRouter();
  const [disableDialogOpen, setDisableDialogOpen] = React.useState(false);
  const openSection = React.useCallback(
    (target: RuntimeWorkspaceTarget) => {
      const suffix =
        target === 'overview'
          ? ''
          : target === 'agents'
            ? '/agents'
            : target === 'keys'
              ? '#runtime-keys'
              : '/docs';
      router.push(`/console/workspace/runtime-workspaces${suffix}`);
    },
    [router]
  );
  const apiBaseUrl = (API_URL || '').replace(/\/$/, '');
  const runtimeEndpoint = `${apiBaseUrl}/api/connect/v1`;

  const currentCandidateSet = runtime.candidates.data ?? {
    scope: 'current' as const,
    cross_workspace_enabled: false,
    source_workspaces: [],
    items: [],
  };
  const crossCandidateSet = runtime.crossWorkspaceCandidates.data ?? {
    scope: 'authorized' as const,
    cross_workspace_enabled: currentCandidateSet.cross_workspace_enabled,
    source_workspaces: currentCandidateSet.source_workspaces,
    items: [],
  };

  const appCollection = runtime.apps.data ?? { revision: 0, items: [] };
  const apps = appCollection.items;
  const totalAppCount = apps.length;
  const callableAppCount = apps.filter(
    app => app.status === 'active' && app.callable !== false
  ).length;
  const usableCredentialCount = (runtime.credentials.data ?? []).filter(item => {
    if (item.status !== 'active') return false;
    return !item.expires_at || new Date(item.expires_at).getTime() > Date.now();
  }).length;

  if (permissionsLoading || runtime.current.isLoading) {
    return (
      <div className="mx-auto max-w-7xl space-y-5 px-4 py-6 @md/console:px-6">
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }

  if (!canManage) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <Alert className="max-w-lg" variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{tCommon('accessDenied')}</AlertTitle>
          <AlertDescription>{t('runtime.errors.permission')}</AlertDescription>
        </Alert>
      </div>
    );
  }

  if (runtime.current.error) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-8 @md/console:px-6">
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t('runtime.errors.load')}</AlertTitle>
          <AlertDescription>{getErrorMessage(runtime.current.error)}</AlertDescription>
        </Alert>
      </div>
    );
  }

  if (!runtime.current.data) {
    return (
      <div className="mx-auto flex h-full max-w-3xl items-center px-4 py-8 @md/console:px-6">
        <Card className="w-full border-border/80 shadow-sm">
          <CardHeader>
            <div className="mb-2 flex size-11 items-center justify-center rounded-xl border bg-muted/30">
              <KeyRound className="size-5 text-muted-foreground" />
            </div>
            <CardTitle>{t('runtime.enable.title')}</CardTitle>
            <CardDescription className="max-w-2xl leading-6">
              {t('runtime.enable.description')}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <p className="rounded-lg border bg-muted/20 p-4 text-sm leading-6 text-muted-foreground">
              {t('runtime.enable.note')}
            </p>
            <Button
              type="button"
              onClick={() => runtime.create.mutate()}
              disabled={runtime.create.isPending}
            >
              {runtime.create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              {t('runtime.enable.button')}
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const isActive = runtime.current.data.status === 'active';
  const primarySection: RuntimeWorkspaceTarget =
    callableAppCount === 0 ? 'agents' : usableCredentialCount === 0 ? 'keys' : 'docs';
  const primaryAction =
    callableAppCount === 0
      ? t('runtime.apps.bind')
      : usableCredentialCount === 0
        ? t('runtime.credentials.create')
        : t('runtime.overview.readiness.readyAction');
  const refreshAll = () => {
    void runtime.current.refetch();
    void runtime.apps.refetch();
    void runtime.candidates.refetch();
    void runtime.credentials.refetch();
  };
  const isRefreshing =
    runtime.current.isFetching ||
    runtime.apps.isFetching ||
    runtime.candidates.isFetching ||
    runtime.credentials.isFetching;

  return (
    <div className="mx-auto flex h-full max-w-7xl flex-col px-4 py-5 @md/console:px-6 @md/console:py-6">
      {section === 'overview' ? (
        <>
          <header className="mb-5 flex flex-col gap-4 border-b border-border/70 pb-5 @4xl/console:flex-row @4xl/console:items-start @4xl/console:justify-between">
            <div className="min-w-0">
              <div className="mb-2 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                {t('runtime.eyebrow')}
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight">{t('runtime.pageTitle')}</h1>
                {statusBadge(
                  runtime.current.data.status,
                  t('runtime.status.active'),
                  t('runtime.status.disabled')
                )}
              </div>
              <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
                {t('runtime.pageDescription')}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {isActive && section === 'overview' ? (
                <Button type="button" onClick={() => openSection(primarySection)}>
                  <ArrowRight className="size-4" />
                  {primaryAction}
                </Button>
              ) : !isActive ? (
                <Button
                  type="button"
                  onClick={() => runtime.update.mutate({ status: 'active' })}
                  disabled={runtime.update.isPending}
                >
                  {runtime.update.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Power className="size-4" />
                  )}
                  {t('runtime.status.enableAction')}
                </Button>
              ) : null}
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="size-9 px-0"
                onClick={refreshAll}
                disabled={isRefreshing}
                aria-label={tCommon('refresh')}
                title={tCommon('refresh')}
              >
                <RefreshCw className={cn('size-4', isRefreshing && 'animate-spin')} />
              </Button>
              {isActive ? (
                <Button type="button" variant="outline" onClick={() => setDisableDialogOpen(true)}>
                  <Unplug className="size-4" />
                  {t('runtime.status.disableAction')}
                </Button>
              ) : null}
            </div>
          </header>

          <ConfirmDialog
            open={disableDialogOpen}
            onOpenChange={setDisableDialogOpen}
            title={t('runtime.status.disableTitle')}
            description={t('runtime.status.disableDescription')}
            confirmText={t('runtime.status.disableConfirm')}
            cancelText={tCommon('cancel')}
            variant="warning"
            loading={runtime.update.isPending}
            closeOnConfirm={false}
            onConfirm={() => {
              void runtime.update
                .mutateAsync({ status: 'disabled' })
                .then(() => setDisableDialogOpen(false));
            }}
          />

          {!isActive ? (
            <Alert className="mb-5 border-warning/40 bg-warning/5">
              <AlertTriangle className="size-4 text-warning" />
              <AlertTitle>{t('runtime.status.pausedTitle')}</AlertTitle>
              <AlertDescription className="flex flex-col gap-3 @3xl/console:flex-row @3xl/console:items-center @3xl/console:justify-between">
                <span>{t('runtime.status.pausedDescription')}</span>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => runtime.update.mutate({ status: 'active' })}
                  disabled={runtime.update.isPending}
                >
                  {runtime.update.isPending ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Power className="size-3.5" />
                  )}
                  {t('runtime.status.enableAction')}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
        </>
      ) : null}

      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden pb-8">
        {section === 'overview' ? (
          <div className="space-y-5">
            <OverviewTab
              totalAppCount={totalAppCount}
              callableAppCount={callableAppCount}
              usableCredentialCount={usableCredentialCount}
              runtimeEndpoint={runtimeEndpoint}
              isActive={isActive}
              isUpdating={runtime.update.isPending}
              onEnable={() => runtime.update.mutate({ status: 'active' })}
              onOpenSection={openSection}
            />
            <div id="runtime-keys" className="scroll-mt-4">
              <CredentialsTab
                credentials={runtime.credentials.data ?? []}
                onCreate={async input => {
                  const result = await runtime.createCredential.mutateAsync(input);
                  toast.success(t('runtime.messages.keyCreated'));
                  return result.secret;
                }}
                onRevoke={async credentialId => {
                  await runtime.revokeCredential.mutateAsync(credentialId);
                }}
                isCreating={runtime.createCredential.isPending}
                isRevoking={runtime.revokeCredential.isPending}
              />
            </div>
          </div>
        ) : null}
        {section === 'agents' ? (
          <AppsTab
            revision={appCollection.revision}
            apps={apps}
            candidates={currentCandidateSet.items}
            crossWorkspaceCandidates={crossCandidateSet.items}
            crossWorkspaceEnabled={currentCandidateSet.cross_workspace_enabled}
            sourceWorkspaces={currentCandidateSet.source_workspaces}
            hostWorkspaceId={workspaceId ?? ''}
            onSync={async input => {
              await runtime.syncAgents.mutateAsync(input);
            }}
            isSyncing={runtime.syncAgents.isPending}
          />
        ) : null}
      </main>
    </div>
  );
}
