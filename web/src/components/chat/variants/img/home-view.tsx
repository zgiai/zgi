'use client';

import * as React from 'react';
import { Palette } from 'lucide-react';
import { useT } from '@/i18n';

export const ImageHomeView = React.forwardRef<HTMLDivElement>(
  (_props, ref) => {
    const t = useT('webapp');

    return (
      <div
        ref={ref}
        className="flex h-full flex-col bg-background animate-in fade-in zoom-in duration-500 overflow-hidden items-center justify-center pb-[260px]"
      >
        <div className="w-full flex flex-col items-center px-6 md:px-8 shrink-0">
          <div className="relative mb-5">
            <div className="absolute inset-0 bg-purple-500/20 blur-2xl rounded-full" />
            <div className="relative flex h-14 w-14 items-center justify-center rounded-2xl bg-purple-500/10 border border-purple-400/30 shadow-md">
              <Palette className="h-7 w-7 text-purple-500" />
            </div>
          </div>
          <h1 className="text-lg md:text-xl font-bold text-foreground tracking-tight mb-1.5">
            {t('chat.imageHomeTitle')}
          </h1>
          <p className="text-sm text-muted-foreground mb-8 text-center max-w-lg">
            {t('chat.imageHomeSubtitle')}
          </p>
        </div>
      </div>
    );
  }
);
ImageHomeView.displayName = 'ImageHomeView';
