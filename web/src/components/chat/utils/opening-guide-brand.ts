import { ICON_BG, ICON_TEXT } from '@/lib/config';

export interface OpeningGuideBrand {
  title?: string;
  iconType?: 'image' | 'text' | string;
  icon?: string;
  iconBackground?: string;
  iconSrc?: string;
}

interface BuildOpeningGuideBrandOptions {
  title?: string;
  iconType?: string;
  icon?: string;
  iconUrl?: string;
}

export function buildOpeningGuideBrand({
  title,
  iconType,
  icon,
  iconUrl,
}: BuildOpeningGuideBrandOptions): OpeningGuideBrand {
  const normalizedTitle = typeof title === 'string' ? title.trim() : '';
  let textIcon = normalizedTitle.slice(0, 2).toUpperCase() || ICON_TEXT;
  let iconBackground = ICON_BG;

  if (iconType === 'text' && icon) {
    try {
      const parsed = JSON.parse(icon);
      textIcon = parsed?.icon || textIcon;
      iconBackground = parsed?.icon_background || iconBackground;
    } catch {
      textIcon = icon || textIcon;
    }
  } else if (iconType !== 'image' && icon) {
    try {
      const parsed = JSON.parse(icon);
      if (parsed?.icon) textIcon = parsed.icon;
      if (parsed?.icon_background) iconBackground = parsed.icon_background;
    } catch {
      textIcon = icon || textIcon;
    }
  }

  return {
    title: normalizedTitle,
    iconType: iconType === 'image' ? 'image' : 'text',
    icon: textIcon,
    iconBackground,
    iconSrc: iconType === 'image' ? iconUrl || icon || undefined : undefined,
  };
}
