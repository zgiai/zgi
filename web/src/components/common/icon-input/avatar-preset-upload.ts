import { withBasePath } from '@/lib/config';
import { uploadService } from '@/services/upload.service';
import { createImageIconValue, type ImageIconValue } from './types';
import type { AppAvatarPreset } from './avatar-presets';

export async function uploadAppAvatarPreset(
  preset: AppAvatarPreset,
  signal?: AbortSignal
): Promise<ImageIconValue> {
  const response = await fetch(withBasePath(preset.src), { signal });
  if (!response.ok) {
    throw new Error(`Unable to load avatar: ${response.status}`);
  }

  const blob = await response.blob();
  const file = new File([blob], `app-avatar-${preset.id}.webp`, {
    type: blob.type || 'image/webp',
  });
  const uploadResult = await uploadService.uploadSingle(file, { is_icon: true, signal });

  return createImageIconValue(preset.src, uploadResult.id);
}
