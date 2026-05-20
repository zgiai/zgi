/**
 * Icon input component types and interfaces
 */

import { ICON_BG, ICON_TEXT } from '@/lib/config';

// Text icon configuration
export interface TextIconValue {
  type: 'text';
  icon: string;
  iconBackground: string;
}

// Image icon configuration
export interface ImageIconValue {
  type: 'image';
  iconUrl: string;
  imageId?: string;
}

// Union type for all icon values
export type IconValue = TextIconValue | ImageIconValue;

// Props for the new unified IconInput component
export interface IconInputProps {
  className?: string;
  value?: IconValue;
  defaultValue?: IconValue;
  disabled?: boolean;
  onChange?: (value: IconValue) => void;
}

// Legacy props interface for backward compatibility during migration
export interface LegacyIconInputProps {
  className?: string;
  icon?: string;
  iconType?: 'text' | 'image';
  defaultValue?: string;
  iconUrl?: string;
  iconBackground?: string;
  agentMode?: boolean;
  disabled?: boolean;
  onIconChange?: (icon: string | { icon: string; icon_background: string }) => void;
  onIconUrlChange?: (icon: string) => void;
  onIconTypeChange?: (type: 'text' | 'image') => void;
  onIconBackgroundChange?: (background: string) => void;
  onImageIdChange?: (imageId: string) => void;
}

// Helper functions for type conversion
export const createTextIconValue = (icon: string, iconBackground: string): TextIconValue => ({
  type: 'text',
  icon,
  iconBackground,
});

export const createImageIconValue = (iconUrl: string, imageId?: string): ImageIconValue => ({
  type: 'image',
  iconUrl,
  imageId,
});

// Default values
export const DEFAULT_TEXT_ICON: TextIconValue = {
  type: 'text',
  icon: ICON_TEXT,
  iconBackground: ICON_BG,
};

export const DEFAULT_IMAGE_ICON: ImageIconValue = {
  type: 'image',
  iconUrl: '',
};
