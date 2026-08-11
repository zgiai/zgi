'use client';

import Image from 'next/image';
import { getDefaultAppAvatar } from '@/components/common/icon-input/avatar-presets';
import { IconPreview, type IconPreviewSize } from '@/components/common/icon-input/icon-preview';
import { cn } from '@/lib/utils';

const APP_AVATAR_SIZE: Record<IconPreviewSize, number> = {
  xs: 32,
  sidebar: 36,
  sidebarExpanded: 40,
  sm: 48,
  md: 64,
  lg: 80,
  xl: 96,
};

interface AppAvatarProps {
  appId: string;
  title: string;
  iconType: 'image' | 'text';
  src?: string | null;
  size: IconPreviewSize;
  className?: string;
}

export function AppAvatar({ appId, title, iconType, src, size, className }: AppAvatarProps) {
  const customImageSrc = iconType === 'image' ? src?.trim() : '';
  if (customImageSrc) {
    return (
      <IconPreview
        iconType="image"
        src={customImageSrc}
        alt={title}
        editable={false}
        size={size}
        className={className}
      />
    );
  }

  const dimension = APP_AVATAR_SIZE[size];
  const avatar = getDefaultAppAvatar(appId);

  return (
    <span
      role="img"
      aria-label={title}
      className={cn(
        'inline-flex shrink-0 items-center justify-center overflow-hidden rounded-lg bg-gradient-to-br p-[3px] ring-1 ring-black/5 dark:ring-white/10',
        avatar.background,
        className
      )}
      style={{ width: dimension, height: dimension }}
    >
      <Image
        src={avatar.src}
        alt=""
        aria-hidden="true"
        width={dimension}
        height={dimension}
        loading="lazy"
        draggable={false}
        unoptimized
        className="h-full w-full select-none object-contain"
      />
    </span>
  );
}
