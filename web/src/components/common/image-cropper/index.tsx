'use client';

import React, { useState, useCallback } from 'react';
import { Upload } from 'lucide-react';
import Cropper from 'react-easy-crop';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Progress } from '@/components/ui/progress';
import { useT } from '@/i18n';
import { uploadService } from '@/services/upload.service';

interface Point {
  x: number;
  y: number;
}

interface Area {
  x: number;
  y: number;
  width: number;
  height: number;
}

interface ImageCropperProps {
  /** Whether the dialog is open */
  isOpen: boolean;
  /** Callback when dialog open state changes */
  onOpenChange: (open: boolean) => void;
  /** Callback when image is cropped successfully */
  onCropComplete: (croppedImageUrl: string, imageId?: string) => void;
  /** Crop output mode: 'upload' (default) or 'base64' */
  mode?: 'upload' | 'base64';
  /** Dialog title */
  title?: string;
  /** Upload text */
  uploadText?: string;
  /** Supported formats text */
  supportedFormatsText?: string;
  /** Cancel button text */
  cancelText?: string;
  /** Crop button text */
  cropText?: string;
  /** Aspect ratio for cropping (default: 1 for square) */
  aspect?: number;
  /** Whether this is uploading an icon (enables icon-specific processing on backend) */
  isIcon?: boolean;
}

// Compression settings for icon images
const ICON_MAX_WIDTH = 200;
const ICON_MAX_HEIGHT = 200;
const ICON_COMPRESSION_QUALITY = 0.8;

// Helper function to create an image from URL
const createImage = (url: string): Promise<HTMLImageElement> => {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.crossOrigin = 'anonymous';
    image.src = url;
    image.onload = () => resolve(image);
    image.onerror = err => reject(err);
  });
};

// Helper function to compress image using canvas
const compressImage = (
  imageSrc: string,
  pixelCrop: Area
): Promise<{ blob: Blob; dataUrl: string }> => {
  return new Promise((resolve, reject) => {
    const image = createImage(imageSrc);
    image
      .then(img => {
        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d');
        if (!ctx) throw new Error('Failed to get canvas context');

        let { width, height } = pixelCrop;

        if (width > height) {
          if (width > ICON_MAX_WIDTH) {
            height = (height * ICON_MAX_WIDTH) / width;
            width = ICON_MAX_WIDTH;
          }
        } else {
          if (height > ICON_MAX_HEIGHT) {
            width = (width * ICON_MAX_HEIGHT) / height;
            height = ICON_MAX_HEIGHT;
          }
        }

        canvas.width = width;
        canvas.height = height;

        ctx.drawImage(
          img,
          pixelCrop.x,
          pixelCrop.y,
          pixelCrop.width,
          pixelCrop.height,
          0,
          0,
          width,
          height
        );

        canvas.toBlob(
          blob => {
            if (blob) {
              const dataUrl = canvas.toDataURL('image/jpeg', ICON_COMPRESSION_QUALITY);
              resolve({ blob, dataUrl });
            } else {
              reject(new Error('Failed to create blob'));
            }
          },
          'image/jpeg',
          ICON_COMPRESSION_QUALITY
        );
      })
      .catch(reject);
  });
};

// Helper function to get cropped image from canvas
const getCroppedImg = async (
  imageSrc: string,
  pixelCrop: Area
): Promise<{ blob: Blob; dataUrl: string }> => {
  return compressImage(imageSrc, pixelCrop);
};

export function ImageCropper({
  isOpen,
  onOpenChange,
  onCropComplete,
  mode = 'upload',
  title,
  uploadText,
  supportedFormatsText,
  cancelText,
  cropText,
  aspect = 1,
  isIcon = false,
}: ImageCropperProps) {
  const t = useT('common');
  const [selectedImage, setSelectedImage] = useState<string | null>(null);
  const [crop, setCrop] = useState<Point>({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [croppedAreaPixels, setCroppedAreaPixels] = useState<Area | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);

  // Handle image upload from input
  const handleImageUpload = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      processFile(file);
    }
  };

  // Process file for both drag-drop and file input
  const processFile = (file: File) => {
    if (file.type.startsWith('image/')) {
      const reader = new FileReader();
      reader.onload = () => {
        setSelectedImage(reader.result as string);
      };
      reader.readAsDataURL(file);
    }
  };

  // Handle drag events
  const handleDragEnter = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  };

  const handleDragOver = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    if (!isDragging) {
      setIsDragging(true);
    }
  };

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      const file = e.dataTransfer.files[0];
      processFile(file);
    }
  };

  // Handle crop complete
  const onCropCompleteCallback = useCallback((_: Area, croppedAreaPixels: Area) => {
    setCroppedAreaPixels(croppedAreaPixels);
  }, []);

  // Handle image crop
  const handleImageCrop = async () => {
    if (!selectedImage || !croppedAreaPixels || isUploading) return;

    try {
      setIsUploading(true);
      setUploadProgress(0);

      const { blob, dataUrl: croppedImageUrl } = await getCroppedImg(
        selectedImage,
        croppedAreaPixels
      );
      if (mode === 'base64') {
        onCropComplete(croppedImageUrl, undefined);
        handleCancel();
      } else {
        const file = new File([blob], 'cropped-image.jpg', { type: 'image/jpeg' });

        try {
          const uploadResult = await uploadService.uploadSingle(file, {
            is_icon: isIcon,
            onProgress: progress => {
              setUploadProgress(progress);
            },
          });

          onCropComplete(croppedImageUrl, uploadResult.id);
          handleCancel();
        } catch (uploadError) {
          console.error('上传失败:', uploadError);
          onCropComplete(croppedImageUrl, undefined);
          handleCancel();
        }
      }
    } catch (e) {
      console.error('裁剪失败:', e);
    } finally {
      setIsUploading(false);
      setUploadProgress(0);
    }
  };

  // Handle cancel
  const handleCancel = () => {
    onOpenChange(false);
    setSelectedImage(null);
    setCrop({ x: 0, y: 0 });
    setZoom(1);
    setCroppedAreaPixels(null);
  };

  return (
    <Dialog open={isOpen} onOpenChange={isUploading ? undefined : onOpenChange}>
      <DialogContent className="max-w-lg" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{title || t('imageCropper.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {!selectedImage ? (
            <div className="space-y-4">
              <div
                className={`border-2 border-dashed rounded-lg p-8 text-center transition-colors ${
                  isUploading
                    ? 'border-gray-200 bg-gray-50 cursor-not-allowed'
                    : isDragging
                      ? 'border-primary bg-primary/5'
                      : 'border-gray-300'
                }`}
                onDragEnter={isUploading ? undefined : handleDragEnter}
                onDragOver={isUploading ? undefined : handleDragOver}
                onDragLeave={isUploading ? undefined : handleDragLeave}
                onDrop={isUploading ? undefined : handleDrop}
              >
                <label
                  htmlFor="image-upload"
                  className={`w-full h-full ${isUploading ? 'cursor-not-allowed' : 'cursor-pointer'}`}
                >
                  <Upload
                    className={`h-12 w-12 mx-auto mb-4 transition-colors ${
                      isUploading ? 'text-gray-300' : isDragging ? 'text-primary' : 'text-gray-400'
                    }`}
                  />
                  <p className="text-sm text-gray-600 mb-2">
                    {isUploading
                      ? '上传中...'
                      : isDragging
                        ? t('imageCropper.dropHere')
                        : uploadText || t('imageCropper.uploadText')}
                  </p>
                  <p className="text-xs text-gray-500">
                    {supportedFormatsText || t('imageCropper.supportedFormats')}
                  </p>
                </label>
                <Input
                  id="image-upload"
                  type="file"
                  accept="image/*"
                  onChange={handleImageUpload}
                  className="hidden"
                  disabled={isUploading}
                />
              </div>
            </div>
          ) : (
            <div className="relative h-80 w-full">
              <Cropper
                image={selectedImage}
                crop={crop}
                zoom={zoom}
                aspect={aspect}
                onCropChange={setCrop}
                onZoomChange={setZoom}
                onCropComplete={onCropCompleteCallback}
                showGrid
                objectFit="horizontal-cover"
              />
            </div>
          )}

          {/* Upload Progress */}
          {isUploading && (
            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>{t('imageCropper.uploadProgress')}</span>
                <span>{uploadProgress}%</span>
              </div>
              <Progress value={uploadProgress} className="h-2" />
            </div>
          )}

          <div className="flex justify-end gap-2 px-0.5">
            <Button variant="outline" onClick={handleCancel} disabled={isUploading}>
              {cancelText || t('imageCropper.cancel')}
            </Button>
            {selectedImage && (
              <Button onClick={handleImageCrop} disabled={isUploading}>
                {isUploading ? '上传中...' : cropText || t('imageCropper.crop')}
              </Button>
            )}
          </div>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
