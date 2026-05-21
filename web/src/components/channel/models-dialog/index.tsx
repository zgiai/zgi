'use client';

import React, { useMemo } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { ModelIcon } from 'modelicons';
import type { ChannelDetail, ChannelItem } from '@/services/types/channel';

interface ModelsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: ChannelDetail | ChannelItem | null;
  loading?: boolean;
}

export default function ModelsDialog(props: ModelsDialogProps): JSX.Element | null {
  const { open, onOpenChange, channel, loading } = props;
  const t = useT('channels');

  const models = useMemo(() => {
    const source = channel?.models ?? [];
    return Array.isArray(source) ? source : [];
  }, [channel?.models]);

  if (!open) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-7xl p-0 overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('modelsDialog.title')}
          </DialogTitle>
          <DialogDescription className="text-neutral-500">
            {t('modelsDialog.description')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="py-6 space-y-4">
          {loading ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
              {Array.from({ length: 10 }).map((_, i) => (
                <div key={i} className="h-10 rounded-xl bg-neutral-100 animate-pulse" />
              ))}
            </div>
          ) : models.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-neutral-400 gap-3 border-2 border-dashed border-neutral-100 rounded-2xl">
              <div className="p-3 bg-neutral-50 rounded-full">
                <ModelIcon model="unknown" size={24} className="opacity-20" />
              </div>
              <span className="text-sm font-medium">{t('modelsDialog.empty')}</span>
            </div>
          ) : (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
              {models.map(m => (
                <div
                  key={m}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-xl border border-neutral-100 bg-white hover:border-blue-200 hover:shadow-sm hover:bg-blue-50/30 transition-all duration-200 group"
                >
                  <div className="flex-shrink-0 p-1 rounded-lg bg-neutral-50 group-hover:bg-white border border-transparent group-hover:border-neutral-200 transition-all shadow-sm">
                    <ModelIcon model={m} size={18} />
                  </div>
                  <span className="text-[13px] font-medium text-foreground w-0 grow truncate tracking-tight">
                    {m}
                  </span>
                </div>
              ))}
            </div>
          )}
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="font-bold rounded-xl h-11 px-8 hover:bg-neutral-100"
          >
            {t('modelsDialog.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
