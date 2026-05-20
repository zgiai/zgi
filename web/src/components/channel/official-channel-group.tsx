'use client';

import { useState, useCallback, useEffect } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Pencil, Eye, Info } from 'lucide-react';
import OfficialChannelDialog from '@/components/channel/official-channel-dialog';
import ModelsDialog from '@/components/channel/models-dialog';
import {
  usePlatformChannels,
  usePlatformChannelModels,
  useUpdateOfficialChannelSettings,
} from '@/hooks';
import type { ChannelDetail, ChannelItem } from '@/services/types/channel';

export default function OfficialChannelGroup() {
  const t = useT('channels');

  const { data: platformResp, isLoading: isPlatformLoading } = usePlatformChannels();
  const platformItem = platformResp?.data;

  const [fetchOfficialModels, setFetchOfficialModels] = useState(false);
  const { data: platformModelsResp, isLoading: isPlatformModelsLoading } = usePlatformChannelModels(
    {
      enabled: fetchOfficialModels,
    }
  );
  const platformModels = platformModelsResp?.data?.models || [];

  const { updateOfficialSettings } = useUpdateOfficialChannelSettings();

  const [officialDialogOpen, setOfficialDialogOpen] = useState<boolean>(false);
  const [modelsOpen, setModelsOpen] = useState<boolean>(false);
  const [modelsChannel, setModelsChannel] = useState<ChannelDetail | ChannelItem | null>(null);
  const [togglingChannel, setTogglingChannel] = useState<string | null>(null);

  // Sync official models when they are loaded and the dialog is open
  useEffect(() => {
    if (modelsOpen && modelsChannel?.is_official && platformModels.length > 0) {
      setModelsChannel(prev => (prev ? { ...prev, models: platformModels } : null));
    }
  }, [platformModels, modelsOpen, modelsChannel?.is_official]);

  const onToggle = useCallback(
    async (next: boolean) => {
      setTogglingChannel('official');
      try {
        await updateOfficialSettings({ is_enabled: next });
      } finally {
        setTogglingChannel(null);
      }
    },
    [updateOfficialSettings]
  );

  return (
    <div>
      <div className="text-base font-semibold mb-2 flex items-center gap-2">
        <span>{t('groups.official')}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex items-center">
              <Info className="h-4 w-4" />
            </span>
          </TooltipTrigger>
          <TooltipContent
            side="top"
            className="max-w-[360px] whitespace-normal break-words leading-5 text-left p-2"
          >
            {t('groupsTips.official')}
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="border rounded-lg overflow-hidden p-1 md:p-2 bg-muted/60">
        {isPlatformLoading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4 p-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Card key={i} className="border-none shadow-sm">
                <CardContent className="p-4 flex flex-col gap-3">
                  <div className="flex items-center gap-3">
                    <Skeleton className="h-10 w-10 rounded-lg" />
                    <div className="space-y-2">
                      <Skeleton className="h-4 w-24" />
                      <Skeleton className="h-3 w-16" />
                    </div>
                  </div>
                  <div className="flex justify-between items-center mt-2">
                    <Skeleton className="h-4 w-20" />
                    <Skeleton className="h-5 w-9 rounded-full" />
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4 p-2">
            {platformItem && (
              <Card className="group border-none shadow-sm hover:shadow-md transition-all duration-200 bg-background">
                <CardContent className="p-4 flex flex-col justify-between h-full gap-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3 overflow-hidden">
                      <div className="flex flex-col overflow-hidden">
                        <div className="font-semibold text-sm truncate group-hover:text-primary transition-colors">
                          {platformItem.name}
                        </div>
                        <div className="flex items-center gap-1.5 mt-1.5">
                          <Badge
                            variant="outline"
                            className="text-[10px] py-0 px-1.5 h-4 border-blue-500/30 bg-blue-500/5 text-blue-600 font-medium"
                          >
                            {t('labels.official')}
                          </Badge>
                          <Badge
                            variant="secondary"
                            className="text-[10px] py-0 px-1.5 h-4 font-normal bg-muted/50"
                          >
                            {platformItem.model_count} {t('table.models')}
                          </Badge>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-4 w-4 p-0 ml-1 text-muted-foreground hover:text-primary"
                            onClick={() => {
                              setModelsChannel({
                                ...platformItem,
                                models: platformModels,
                                is_official: true,
                              } as unknown as ChannelItem);
                              setFetchOfficialModels(true);
                              setModelsOpen(true);
                            }}
                          >
                            <Eye className="h-3 w-3" />
                          </Button>
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-1">
                      <Switch
                        checked={Boolean(platformItem.is_enabled)}
                        onCheckedChange={checked => onToggle(checked as boolean)}
                        disabled={togglingChannel === 'official'}
                        className="data-[state=checked]:bg-green-600 scale-75"
                      />
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0"
                        onClick={() => setOfficialDialogOpen(true)}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  <div className="flex items-center justify-between text-[11px] text-muted-foreground pt-1 border-t border-muted/50">
                    <div className="flex items-center gap-4">
                      <span className="flex items-center gap-1.5">
                        <span className="opacity-60">{t('table.priority')}:</span>
                        <span className="font-semibold text-foreground/80">
                          {platformItem.priority ?? 0}
                        </span>
                      </span>
                      <span className="flex items-center gap-1.5">
                        <span className="opacity-60">{t('table.weight')}:</span>
                        <span className="font-semibold text-foreground/80">
                          {platformItem.weight ?? 0}
                        </span>
                      </span>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )}
            {!isPlatformLoading && !platformItem && (
              <div className="col-span-full py-12 flex flex-col items-center justify-center text-center text-muted-foreground bg-background rounded-lg border border-dashed text-sm">
                {t('empty.title')}
              </div>
            )}
          </div>
        )}
      </div>

      <OfficialChannelDialog
        open={officialDialogOpen}
        onOpenChange={setOfficialDialogOpen}
        initial={platformItem}
      />

      <ModelsDialog
        open={modelsOpen}
        onOpenChange={open => {
          setModelsOpen(open);
          if (!open) setModelsChannel(null);
        }}
        channel={modelsChannel}
        loading={modelsChannel?.is_official ? isPlatformModelsLoading : false}
      />
    </div>
  );
}
