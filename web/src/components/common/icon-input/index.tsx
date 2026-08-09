'use client';

import Image from 'next/image';
import React, { useEffect, useState } from 'react';
import { Loader2, Sparkles, Type, Upload } from 'lucide-react';
import { toast } from 'sonner';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { ImageCropper } from '@/components/common/image-cropper';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { uploadAppAvatarPreset } from './avatar-preset-upload';
import { APP_AVATAR_PRESETS, type AppAvatarPreset } from './avatar-presets';
import {
  type IconInputProps,
  DEFAULT_TEXT_ICON,
  createTextIconValue,
  createImageIconValue,
} from './types';

export function IconInput({
  className,
  value,
  defaultValue = DEFAULT_TEXT_ICON,
  disabled = false,
  showAvatarPresets = false,
  onChange,
}: IconInputProps) {
  const t = useT('common');
  const [isTextDialogOpen, setIsTextDialogOpen] = useState(false);
  const [isImageDialogOpen, setIsImageDialogOpen] = useState(false);
  const [isAvatarDialogOpen, setIsAvatarDialogOpen] = useState(false);
  const [uploadingPresetId, setUploadingPresetId] = useState<string | null>(null);

  // Use controlled value or fallback to defaultValue
  const currentValue = value || defaultValue;

  // Temporary state for dialog editing
  const [tempIconText, setTempIconText] = useState(
    currentValue.type === 'text' ? currentValue.icon : ICON_TEXT
  );
  const [tempIconBackground, setTempIconBackground] = useState(
    currentValue.type === 'text' ? currentValue.iconBackground : ICON_BG
  );

  const handleTextIconClick = () => {
    if (disabled) return;
    setTempIconText(currentValue.type === 'text' ? currentValue.icon : ICON_TEXT);
    setTempIconBackground(currentValue.type === 'text' ? currentValue.iconBackground : ICON_BG);
    setIsTextDialogOpen(true);
  };

  const handleImageIconClick = () => {
    if (disabled) return;
    setIsImageDialogOpen(true);
  };

  const handleAvatarPresetClick = () => {
    if (disabled) return;
    setIsAvatarDialogOpen(true);
  };

  const handleAvatarSelect = async (preset: AppAvatarPreset) => {
    if (disabled || uploadingPresetId) return;

    try {
      setUploadingPresetId(preset.id);
      onChange?.(await uploadAppAvatarPreset(preset));
      setIsAvatarDialogOpen(false);
    } catch (error) {
      console.error('Avatar preset upload failed:', error);
      toast.error(t('iconInput.avatarLibraryDialog.uploadFailed'));
    } finally {
      setUploadingPresetId(null);
    }
  };

  const handleIconPreviewClick = () => {
    if (disabled) return;
    if (currentValue.type === 'text') {
      setTempIconText(currentValue.icon);
      setTempIconBackground(currentValue.iconBackground);
      setIsTextDialogOpen(true);
    } else {
      setIsImageDialogOpen(true);
    }
  };

  // Handle crop complete - now always in upload mode, returns imageId
  const handleCropComplete = (croppedImageUrl: string, imageId?: string) => {
    if (imageId) {
      onChange?.(createImageIconValue(croppedImageUrl, imageId));
    }
  };

  const handleTextDialogSave = () => {
    const newValue = createTextIconValue(
      tempIconText || (defaultValue.type === 'text' ? defaultValue.icon : ICON_TEXT),
      tempIconBackground
    );
    onChange?.(newValue);
    setIsTextDialogOpen(false);
  };

  const handleTextDialogCancel = () => {
    setTempIconText(currentValue.type === 'text' ? currentValue.icon : ICON_TEXT);
    setTempIconBackground(currentValue.type === 'text' ? currentValue.iconBackground : ICON_BG);
    setIsTextDialogOpen(false);
  };

  useEffect(() => {
    setTempIconText(currentValue.type === 'text' ? currentValue.icon : ICON_TEXT);
    setTempIconBackground(currentValue.type === 'text' ? currentValue.iconBackground : ICON_BG);
  }, [currentValue]);

  return (
    <>
      <div className={cn('space-x-4 flex', className)}>
        {/* Icon preview */}
        <IconPreview
          iconType={currentValue.type}
          icon={currentValue.type === 'text' ? currentValue.icon : ''}
          iconBackground={currentValue.type === 'text' ? currentValue.iconBackground : ICON_BG}
          src={currentValue.type === 'image' ? currentValue.iconUrl : ''}
          size="lg"
          showUpload={false}
          showRemove={false}
          disabled={disabled}
          onUploadClick={handleIconPreviewClick}
          onIconClick={handleIconPreviewClick}
        />

        {/* Action buttons */}
        <div className="space-y-2">
          <div className="flex flex-wrap gap-2">
            {showAvatarPresets && (
              <Button
                variant="outline"
                type="button"
                size="sm"
                className="flex items-center gap-2"
                disabled={disabled}
                onClick={handleAvatarPresetClick}
              >
                <Sparkles className="h-4 w-4" />
                {t('iconInput.avatarLibrary')}
              </Button>
            )}
            <Button
              variant="outline"
              type="button"
              size="sm"
              className="flex items-center gap-2"
              disabled={disabled}
              onClick={handleImageIconClick}
            >
              <Upload className="h-4 w-4" />
              {t('iconInput.uploadImage')}
            </Button>
            <Button
              variant="outline"
              type="button"
              size="sm"
              className="flex items-center gap-2"
              disabled={disabled}
              onClick={handleTextIconClick}
            >
              <Type className="h-4 w-4" />
              {t('iconInput.textIcon')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t('iconInput.supportedFormats')}</p>
        </div>
      </div>

      {/* Text Icon Dialog */}
      <Dialog open={isTextDialogOpen} onOpenChange={setIsTextDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('iconInput.textIconDialog.title')}</DialogTitle>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <div className="space-y-2">
              <Input
                id="icon-text"
                value={tempIconText}
                onChange={e => setTempIconText(e.target.value)}
                placeholder={t('iconInput.textIconDialog.iconTextPlaceholder')}
                maxLength={2}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="icon-background">
                {t('iconInput.textIconDialog.backgroundColor')}
              </Label>
              <div className="flex relative">
                <label
                  htmlFor="icon-background"
                  className="w-14 h-14 rounded border cursor-pointer"
                  style={{ backgroundColor: tempIconBackground }}
                />
                <Input
                  id="icon-background"
                  type="color"
                  value={tempIconBackground}
                  onChange={e => setTempIconBackground(e.target.value)}
                  className="invisible w-0 h-0 p-0"
                />
              </div>
            </div>
            <div className="flex justify-end gap-2 px-0.5">
              <Button variant="outline" onClick={handleTextDialogCancel}>
                {t('iconInput.textIconDialog.cancel')}
              </Button>
              <Button onClick={handleTextDialogSave}>{t('iconInput.textIconDialog.save')}</Button>
            </div>
          </DialogBody>
        </DialogContent>
      </Dialog>

      {/* Avatar Preset Dialog */}
      <Dialog open={isAvatarDialogOpen} onOpenChange={setIsAvatarDialogOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t('iconInput.avatarLibraryDialog.title')}</DialogTitle>
          </DialogHeader>
          <DialogBody className="space-y-5">
            <p className="text-sm text-muted-foreground">
              {t('iconInput.avatarLibraryDialog.description')}
            </p>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              {APP_AVATAR_PRESETS.map(preset => {
                const isUploading = uploadingPresetId === preset.id;
                return (
                  <button
                    key={preset.id}
                    type="button"
                    disabled={uploadingPresetId !== null}
                    onClick={() => handleAvatarSelect(preset)}
                    className="group rounded-2xl border bg-card p-2 text-left transition-colors hover:border-primary/40 hover:bg-accent/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-wait disabled:opacity-60"
                  >
                    <span
                      className={cn(
                        'relative flex aspect-square items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br p-3',
                        preset.background
                      )}
                    >
                      <Image
                        src={preset.src}
                        alt=""
                        aria-hidden="true"
                        width={160}
                        height={160}
                        unoptimized
                        className="h-full w-full select-none object-contain transition-transform duration-200 group-hover:scale-105"
                      />
                      {isUploading && (
                        <span className="absolute inset-0 flex items-center justify-center bg-background/65 backdrop-blur-sm">
                          <Loader2 className="h-5 w-5 animate-spin text-primary" />
                        </span>
                      )}
                    </span>
                    <span className="mt-2 block px-1 pb-1 text-sm font-medium">
                      {t(`iconInput.avatarLibraryDialog.avatars.${preset.labelKey}`)}
                    </span>
                  </button>
                );
              })}
            </div>
          </DialogBody>
        </DialogContent>
      </Dialog>

      {/* Image Crop Dialog */}
      <ImageCropper
        isOpen={isImageDialogOpen}
        onOpenChange={setIsImageDialogOpen}
        onCropComplete={handleCropComplete}
        mode="upload"
        title={t('iconInput.imageCropDialog.title')}
        uploadText={t('iconInput.imageCropDialog.uploadText')}
        supportedFormatsText={t('iconInput.supportedFormats')}
        cancelText={t('iconInput.imageCropDialog.cancel')}
        cropText={t('iconInput.imageCropDialog.crop')}
        isIcon
      />
    </>
  );
}
