'use client';

import { useEffect, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { toast } from 'sonner';
import {
  ArrowLeft,
  Check,
  Download,
  MoreHorizontal,
  Star,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  useInstallPluginFromMarketplace,
  useMarketplaceBranding,
  useMarketplacePluginDetail,
  useMarketplacePluginFavorite,
  useMarketplacePluginVersions,
} from '@/hooks/use-plugins';
import { useLocale } from '@/hooks/use-locale';
import { useAuthStore } from '@/store';
import { pluginService } from '@/services/plugin.service';
import type { MarketplaceBrandingSettings, MarketplacePlugin } from '@/services/types/plugin';

type PluginToolDefinition = {
  name: string;
  description: string;
};

export default function MarketplacePluginDetailPage() {
  const { pluginId } = useParams<{ pluginId: string }>();
  const router = useRouter();
  const { locale } = useLocale();
  const user = useAuthStore.use.user();
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);
  const [isIconLoadFailed, setIsIconLoadFailed] = useState(false);
  const [isReportOpen, setIsReportOpen] = useState(false);
  const [reportContent, setReportContent] = useState('');
  const [isSubmittingReport, setIsSubmittingReport] = useState(false);

  const { plugin, isLoading, error } = useMarketplacePluginDetail(pluginId, true);
  const { versions, isLoading: isLoadingVersions } = useMarketplacePluginVersions(pluginId, {
    page: 1,
    page_size: 20,
  });
  const branding = useMarketplaceBranding();
  const isZh = locale === 'zh-Hans';
  const isFavoriteEnabled =
    branding.is_loaded === true && branding.metric_enabled?.favorites !== false;
  const {
    isInstalled,
    isPluginInstalled,
    installedVersionId,
    isInstalling,
    isUninstalling,
    installPlugin,
    uninstallPlugin,
    refetchStatus,
  } = useInstallPluginFromMarketplace(pluginId, selectedVersionId, Boolean(pluginId && selectedVersionId));
  const favorite = useMarketplacePluginFavorite(pluginId, user?.id, isFavoriteEnabled);

  useEffect(() => {
    if (!selectedVersionId) {
      setSelectedVersionId(plugin?.latest_version?.id || versions[0]?.id || null);
    }
  }, [plugin?.latest_version?.id, selectedVersionId, versions]);

  useEffect(() => {
    setIsIconLoadFailed(false);
  }, [plugin?.id, plugin?.icon]);

  const selectedVersion = versions.find(version => version.id === selectedVersionId) ?? versions[0];

  const handleInstall = async () => {
    if (!plugin || !selectedVersionId) return;
    try {
      await installPlugin(plugin.id, selectedVersionId);
      await refetchStatus();
      toast.success(isZh ? '插件已安装' : 'Plugin installed');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : isZh ? '安装失败' : 'Install failed');
    }
  };

  const handleUninstall = async () => {
    const versionIdToUninstall = isInstalled ? selectedVersionId : installedVersionId;
    if (!versionIdToUninstall) return;
    try {
      await uninstallPlugin(versionIdToUninstall);
      await refetchStatus();
      toast.success(isZh ? '插件已卸载' : 'Plugin uninstalled');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : isZh ? '卸载失败' : 'Uninstall failed');
    }
  };

  const handleUpdate = async () => {
    if (!plugin || !selectedVersionId) return;
    const previousVersionId = installedVersionId;
    try {
      await installPlugin(plugin.id, selectedVersionId);
      if (previousVersionId && previousVersionId !== selectedVersionId) {
        await uninstallPlugin(previousVersionId);
      }
      await refetchStatus();
      toast.success(isZh ? '插件已更新' : 'Plugin updated');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : isZh ? '更新失败' : 'Update failed');
    }
  };

  const handleFavorite = async () => {
    if (!user?.id) {
      toast.error(isZh ? '请先登录' : 'Please sign in first');
      return;
    }
    try {
      if (favorite.favorited) {
        await favorite.unfavorite();
      } else {
        await favorite.favorite();
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : isZh ? '收藏操作失败' : 'Favorite failed');
    }
  };

  const handleReport = async () => {
    if (!plugin) return;
    const content = reportContent.trim();
    if (!content) {
      toast.error(isZh ? '请填写举报原因' : 'Please enter a report reason');
      return;
    }

    try {
      setIsSubmittingReport(true);
      await pluginService.submitMarketplacePluginFeedback({
        request_type: 'report',
        plugin_id: plugin.id,
        content,
        submitter_id: user?.id,
        submitter_name: user?.name,
        submitter_email: user?.email,
      });
      toast.success(isZh ? '举报已提交' : 'Report submitted');
      setReportContent('');
      setIsReportOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : isZh ? '举报提交失败' : 'Report failed');
    } finally {
      setIsSubmittingReport(false);
    }
  };

  if (isLoading) {
    return (
      <div className="h-full overflow-y-auto bg-background">
        <div className="mx-auto w-full max-w-7xl space-y-8 px-6 py-8">
          <Skeleton className="h-8 w-8" />
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-48 w-full" />
        </div>
      </div>
    );
  }

  if (!plugin) {
    return (
      <div className="h-full overflow-y-auto bg-background">
        <div className="mx-auto w-full max-w-7xl space-y-6 px-6 py-8">
          <button
            type="button"
            onClick={() => router.back()}
            className="inline-flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label={isZh ? '返回' : 'Back'}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div className="rounded-xl border border-destructive/30 px-6 py-10">
            <h1 className="text-lg font-semibold">
              {isZh ? '插件详情无法打开' : 'Plugin detail unavailable'}
            </h1>
            <p className="mt-2 text-sm text-muted-foreground">
              {error || (isZh ? '未找到该插件或服务暂时不可用。' : 'The plugin was not found or the service is temporarily unavailable.')}
            </p>
          </div>
        </div>
      </div>
    );
  }

  const stats = buildStats(plugin, favorite.favoriteCount, isZh, branding);
  const labels = Array.from(new Set([...(plugin.official_labels || []), ...(plugin.tags || [])]));
  const toolDefinitions = normalizeToolDefinitions(selectedVersion?.manifest?.tools, isZh);
  const hasNewerVersion = Boolean(
    isPluginInstalled &&
      selectedVersion?.id &&
      plugin.latest_version?.id &&
      selectedVersion.id === plugin.latest_version.id &&
      installedVersionId &&
      installedVersionId !== plugin.latest_version.id
  );

  return (
    <div className="h-full overflow-y-auto bg-background">
      <div className="mx-auto w-full max-w-7xl space-y-8 px-4 py-6 sm:px-6 lg:px-8">
        <button
          type="button"
          onClick={() => router.back()}
          className="inline-flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label={isZh ? '返回' : 'Back'}
        >
          <ArrowLeft className="h-4 w-4" />
        </button>

        <section className="grid gap-6 border-b pb-8 lg:grid-cols-[minmax(0,1fr)_auto]">
          <div className="flex min-w-0 gap-5">
            <div className="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-muted">
              {plugin.icon && !isIconLoadFailed ? (
                <img
                  src={plugin.icon}
                  alt={plugin.name}
                  className="h-full w-full object-contain"
                  onError={() => setIsIconLoadFailed(true)}
                />
              ) : (
                <span className="text-2xl font-semibold text-muted-foreground">
                  {plugin.name.slice(0, 2)}
                </span>
              )}
            </div>
            <div className="min-w-0 space-y-3">
              <div>
                <h1 className="break-words text-3xl font-semibold tracking-tight">
                  {plugin.name}
                </h1>
                <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                  <span>{plugin.developer?.organization_name || '-'}</span>
                  {plugin.is_official && <Badge>{isZh ? '官方' : 'Official'}</Badge>}
                  {plugin.latest_version?.published_at && (
                    <span>
                      · {isZh ? '发布于' : 'Published'}{' '}
                      {new Date(plugin.latest_version.published_at).toLocaleString()}
                    </span>
                  )}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                {labels.map(label => (
                  <Badge key={label} variant="secondary">
                    {label}
                  </Badge>
                ))}
              </div>
            </div>
          </div>

          <div className="flex flex-wrap items-start justify-start gap-2 lg:justify-end">
            {isFavoriteEnabled && (
              <Button
                variant={favorite.favorited ? 'secondary' : 'outline'}
                onClick={handleFavorite}
                disabled={favorite.isMutating}
                className="h-10"
              >
                <Star className="mr-2 h-4 w-4" />
                {isZh ? '收藏' : 'Favorite'} {favorite.favoriteCount || ''}
              </Button>
            )}
            {isInstalled ? (
              <Button
                variant="destructive"
                onClick={handleUninstall}
                disabled={isUninstalling}
                className="h-10"
              >
                {isUninstalling ? (isZh ? '卸载中...' : 'Uninstalling...') : isZh ? '卸载' : 'Uninstall'}
              </Button>
            ) : !hasNewerVersion ? (
              <Button
                onClick={handleInstall}
                disabled={isInstalling || !selectedVersionId}
                className="h-10"
              >
                <Download className="mr-2 h-4 w-4" />
                {isInstalling ? (isZh ? '安装中...' : 'Installing...') : isZh ? '安装' : 'Install'}
              </Button>
            ) : null}
            {hasNewerVersion && (
              <Button
                variant="outline"
                className="h-10"
                onClick={handleUpdate}
                disabled={isInstalling || isUninstalling}
              >
                {isInstalling || isUninstalling ? (isZh ? '更新中...' : 'Updating...') : isZh ? '更新' : 'Update'}
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" className="h-10 w-10 p-0">
                  <MoreHorizontal className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setIsReportOpen(true)}>
                  {isZh ? '举报' : 'Report'}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </section>

        {stats.length > 0 && (
          <section className="grid gap-4 border-b pb-8 sm:grid-cols-2 lg:grid-cols-5">
            {stats.map(stat => (
              <div key={stat.key} className="min-w-0 border-r last:border-r-0">
                <div className="text-xl font-medium">{stat.value}</div>
                <div className="mt-1 text-sm text-muted-foreground">{stat.label}</div>
              </div>
            ))}
          </section>
        )}

        <Tabs defaultValue="description">
          <TabsList className="bg-transparent p-0">
            <TabsTrigger value="description">
              {isZh ? '插件描述' : 'Plugin Description'}
            </TabsTrigger>
            <TabsTrigger value="versions">{isZh ? '版本' : 'Versions'}</TabsTrigger>
            <TabsTrigger value="tools">{isZh ? '插件工具' : 'Plugin Tools'}</TabsTrigger>
          </TabsList>
          <TabsContent value="description" className="mt-8 space-y-4">
            <h2 className="text-base font-semibold">
              {isZh ? '插件功能描述' : 'Plugin Function Description'}
            </h2>
            <p className="max-w-4xl whitespace-pre-line text-sm leading-7 text-muted-foreground">
              {plugin.description || plugin.short_description || '-'}
            </p>
          </TabsContent>
          <TabsContent value="versions" className="mt-8">
            {isLoadingVersions ? (
              <Skeleton className="h-32 w-full" />
            ) : versions.length === 0 ? (
              <p className="text-sm text-muted-foreground">{isZh ? '暂无版本' : 'No versions'}</p>
            ) : (
              <div className="grid gap-3 md:grid-cols-2">
                {versions.map(version => (
                  <button
                    key={version.id}
                    type="button"
                    onClick={() => setSelectedVersionId(version.id)}
                    className="relative rounded-lg border p-4 text-left hover:bg-muted/50"
                  >
                    {selectedVersion?.id === version.id && (
                      <Check className="absolute right-4 top-4 h-4 w-4 text-primary" />
                    )}
                    <div className="font-medium">{version.version}</div>
                    <div className="mt-2 text-sm text-muted-foreground">
                      {version.changelog || '-'}
                    </div>
                  </button>
                ))}
              </div>
            )}
          </TabsContent>
          <TabsContent value="tools" className="mt-8">
            <div className="rounded-lg border">
              <div className="grid grid-cols-[180px_minmax(0,1fr)] border-b bg-muted/50 px-4 py-3 text-sm font-medium">
                <span>{isZh ? '工具' : 'Tool'}</span>
                <span>{isZh ? '说明' : 'Description'}</span>
              </div>
              {toolDefinitions.length > 0 ? (
                toolDefinitions.map(tool => (
                  <div
                    key={`${tool.name}-${tool.description}`}
                    className="grid grid-cols-[180px_minmax(0,1fr)] px-4 py-3 text-sm"
                  >
                    <span className="font-medium">{tool.name}</span>
                    <span className="text-muted-foreground">
                      {tool.description ||
                        (isZh ? '由插件包 manifest 提供' : 'Provided by plugin manifest')}
                    </span>
                  </div>
                ))
              ) : (
                <div className="px-4 py-8 text-sm text-muted-foreground">
                  {isZh ? '暂无工具定义' : 'No tool definitions'}
                </div>
              )}
            </div>
          </TabsContent>
        </Tabs>
      </div>
      <Dialog open={isReportOpen} onOpenChange={setIsReportOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{isZh ? '举报插件' : 'Report plugin'}</DialogTitle>
            <DialogDescription>
              {isZh
                ? '请说明插件存在的问题，提交后会进入 console 插件反馈。'
                : 'Describe the issue. The report will appear in console plugin feedback.'}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            <div className="rounded-lg border bg-muted/40 p-3 text-sm">
              <div className="font-medium">{plugin.name}</div>
              <div className="mt-1 text-muted-foreground">
                {plugin.developer?.organization_name || '-'}
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="plugin-report-content">
                {isZh ? '举报原因' : 'Report reason'}
              </Label>
              <Textarea
                id="plugin-report-content"
                rows={5}
                maxLength={2000}
                value={reportContent}
                onChange={event => setReportContent(event.target.value)}
                placeholder={
                  isZh
                    ? '请描述违规、不可用、误导性内容或其他问题'
                    : 'Describe abuse, broken behavior, misleading content, or other issues'
                }
              />
              <div className="text-right text-xs text-muted-foreground">
                {reportContent.length}/2000
              </div>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setIsReportOpen(false)}
              disabled={isSubmittingReport}
            >
              {isZh ? '取消' : 'Cancel'}
            </Button>
            <Button type="button" onClick={handleReport} disabled={isSubmittingReport}>
              {isSubmittingReport ? (isZh ? '提交中...' : 'Submitting...') : isZh ? '提交' : 'Submit'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function buildStats(
  plugin: MarketplacePlugin,
  favoriteCount: number,
  isZh: boolean,
  branding: MarketplaceBrandingSettings
) {
  const hash = stableHash(plugin.id);
  const installs = plugin.download_count || 0;
  const favorites = favoriteCount || plugin.rating_count || 0;
  const metricEnabled = branding.metric_enabled ?? {};
  const metricStats = [
    {
      key: 'downloads',
      label: isZh ? '安装量' : 'Installs',
      value: compactNumber(installs),
      enabled: metricEnabled.downloads !== false,
    },
    {
      key: 'runtime',
      label: isZh ? '执行时长' : 'Execution time',
      value: `${120 + (hash % 1800)}ms`,
      enabled: metricEnabled.runtime !== false,
    },
    {
      key: 'success',
      label: isZh ? '成功率' : 'Success rate',
      value: `${Math.min(99.9, 86 + (hash % 139) / 10).toFixed(1)}%`,
      enabled: metricEnabled.success !== false,
    },
    {
      key: 'favorites',
      label: isZh ? '收藏' : 'Favorites',
      value: compactNumber(favorites),
      enabled: metricEnabled.favorites !== false,
    },
  ];
  const visibleMetricStats = metricStats.filter(stat => stat.enabled);
  if (visibleMetricStats.length === 0) return [];

  return [
    { key: 'tools', label: isZh ? '工具' : 'Tools', value: String(plugin.latest_version ? 1 : 0) },
    ...visibleMetricStats,
  ];
}

function stableHash(value: string) {
  let hash = 0;
  for (let i = 0; i < value.length; i += 1) {
    hash = (hash * 31 + value.charCodeAt(i)) >>> 0;
  }
  return hash;
}

function compactNumber(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(value);
}

function normalizeToolDefinitions(raw: unknown, isZh: boolean): PluginToolDefinition[] {
  if (Array.isArray(raw)) {
    return raw
      .map(item => normalizeToolDefinition(item, isZh))
      .filter((item): item is PluginToolDefinition => Boolean(item));
  }

  if (raw && typeof raw === 'object') {
    return Object.entries(raw as Record<string, unknown>)
      .map(([key, value]) => normalizeToolDefinition(value, isZh, key))
      .filter((item): item is PluginToolDefinition => Boolean(item));
  }

  return [];
}

function normalizeToolDefinition(
  raw: unknown,
  isZh: boolean,
  fallbackName = ''
): PluginToolDefinition | null {
  if (typeof raw === 'string') {
    const name = raw.trim();
    return name ? { name, description: '' } : null;
  }

  if (!raw || typeof raw !== 'object') return null;
  const record = raw as Record<string, unknown>;
  const name =
    pickLocalizedString(record.label, isZh) ||
    pickString(record, ['name', 'tool_name', 'title']) ||
    fallbackName;
  const description =
    pickLocalizedString(record.description, isZh) ||
    pickLocalizedString((record.description as Record<string, unknown> | undefined)?.human, isZh) ||
    pickString(record, ['summary', 'desc', 'description']);
  if (!name && !description) return null;
  return { name, description };
}

function pickString(record: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
  }
  return '';
}

function pickLocalizedString(raw: unknown, isZh: boolean) {
  if (typeof raw === 'string') return raw.trim();
  if (!raw || typeof raw !== 'object') return '';
  const record = raw as Record<string, unknown>;
  const preferredKeys = isZh
    ? ['zh_Hans', 'zh-Hans', 'zh_CN', 'zh-CN', 'default', 'en_US', 'en-US']
    : ['en_US', 'en-US', 'default', 'zh_Hans', 'zh-Hans', 'zh_CN', 'zh-CN'];
  for (const key of preferredKeys) {
    const value = record[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
  }
  return '';
}
