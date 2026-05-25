'use client';

import React from 'react';
import { useParams } from 'next/navigation';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useMaybeMigrateUser } from '@/hooks/webapp/use-maybe-migrate-user';
import { Logo } from '@/components/logo';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { AgentType } from '@/services/types/agent';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { Providers } from '@/providers';

export default function WebappVersionLayout({ children }: { children: React.ReactNode }) {
  return (
    <Providers>
      <WebappVersionLayoutContent>{children}</WebappVersionLayoutContent>
    </Providers>
  );
}

function WebappVersionLayoutContent({ children }: { children: React.ReactNode }) {
  const { version_uuid } = useParams<{ version_uuid: string }>();
  useMaybeMigrateUser();
  const { data, isLoading } = useWebAppConfig(version_uuid);

  const meta = data?.data?.config;
  const iconType = meta?.icon_type;

  // Derive icon props consistent with AgentSidebar
  let textIcon = (meta?.title || ICON_TEXT).slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  let imgSrc: string | undefined = undefined;
  if (iconType === 'image') {
    imgSrc = meta?.icon_url || meta?.icon || '';
  } else if (iconType === 'text') {
    try {
      const parsed = JSON.parse(meta?.icon || '{}');
      textIcon = parsed?.icon || textIcon;
      iconBackground = parsed?.icon_background || iconBackground;
    } catch {
      // ignore parse error
    }
  } else if (meta?.icon) {
    try {
      const parsed = JSON.parse(meta.icon);
      if (parsed?.icon) textIcon = parsed.icon;
      if (parsed?.icon_background) iconBackground = parsed.icon_background;
    } catch {
      // ignore parse error
    }
  }

  return (
    <div className="flex h-[100dvh] min-h-[100dvh] max-h-[100dvh] w-full flex-col overflow-hidden">
      {/* Webapp global header for this version */}
      <div
        className={cn(
          'w-full shrink-0 bg-background',
          meta?.type === AgentType.CONVERSATIONAL_AGENT && 'hidden md:block'
        )}
      >
        <div className="px-4 py-1 flex items-center justify-between">
          <div className="hidden md:block max-w-52">
            <Logo routerToHome={false} showName={false} />
          </div>
          <div className="flex items-center gap-2">
            {isLoading ? (
              <>
                <Skeleton className="h-6 w-6 rounded-md" />
                <Skeleton className="h-4 w-36" />
              </>
            ) : (
              <>
                <IconPreview
                  iconType={iconType === 'image' ? 'image' : 'text'}
                  src={iconType === 'image' ? imgSrc : ''}
                  icon={textIcon}
                  iconBackground={iconBackground}
                  editable={false}
                  size="sm"
                />
                <div className="text-lg font-medium" title={meta?.title}>
                  {meta?.title}
                </div>
              </>
            )}
          </div>
          <div />
        </div>
      </div>

      {/* Children pages */}
      <div className="grow min-h-0 w-full overflow-hidden">{children}</div>
    </div>
  );
}
