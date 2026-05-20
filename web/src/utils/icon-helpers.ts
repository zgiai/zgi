/**
 * Shared helpers for icon value conversions and dataset/folder payload mapping.
 *
 * Ensures strict typing and centralizes logic to avoid duplication across components.
 */

import type { Dataset } from '@/services/types/dataset';
import type { DatasetFolder } from '@/services/types/dataset-folder';
import {
  createTextIconValue,
  createImageIconValue,
  type IconValue,
} from '@/components/common/icon-input/types';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

// Shared icon type definition centralized here
export type IconType = 'text' | 'image';

export interface DatasetIconPayload {
  icon_type: IconType;
  icon: string;
  icon_background: string;
}

// Minimal shape required to build icon value from an existing record
export interface IconSourceMinimal {
  name: string;
  icon_type?: IconType | null;
  icon?: string | null;
  icon_url?: string | null;
  icon_background?: string | null;
}

/**
 * Build IconValue from an existing record (dataset or folder).
 * - If uses an image icon, returns ImageIconValue with both URL and image id.
 * - If uses a text icon, returns TextIconValue with sensible fallbacks.
 */
export function buildIconValueFromDataset(dataset: Dataset | IconSourceMinimal): IconValue {
  if (dataset.icon_type === 'image') {
    return createImageIconValue(dataset.icon_url || '', dataset.icon || '');
  }
  const textIcon =
    dataset.icon && dataset.icon.trim() ? dataset.icon : dataset.name.slice(0, 2).toUpperCase();
  return createTextIconValue(textIcon || ICON_TEXT, dataset.icon_background || ICON_BG);
}

/**
 * Alias for clarity when used with folders.
 */
export function buildIconValueFromFolder(folder: DatasetFolder): IconValue {
  return buildIconValueFromDataset(folder);
}

/**
 * Convert IconValue to dataset/folder icon payload for create/update mutations.
 * - For image type, uses imageId or falls back to existing icon id.
 * - For text type, uses icon or derives from name, with background fallback.
 */
export function iconValueToDatasetPayload(
  iconValue: IconValue,
  options?: {
    existing?: Pick<IconSourceMinimal, 'icon' | 'icon_background'>;
    defaultTextFromName?: string;
  }
): DatasetIconPayload {
  const existing = options?.existing;

  if (iconValue.type === 'image') {
    const imageId = iconValue.imageId || existing?.icon || '';
    return {
      icon_type: 'image',
      icon: imageId,
      icon_background: ICON_BG,
    };
  }

  const derivedText = options?.defaultTextFromName
    ? options.defaultTextFromName.slice(0, 2).toUpperCase()
    : ICON_TEXT;
  const textIcon = iconValue.icon || derivedText;
  const bg = iconValue.iconBackground || existing?.icon_background || ICON_BG;

  return {
    icon_type: 'text',
    icon: textIcon,
    icon_background: bg,
  };
}
