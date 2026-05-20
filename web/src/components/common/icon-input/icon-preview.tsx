'use client';

import React, { useState, useRef } from 'react';
import { Plus, Upload, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { IconType } from '@/utils/icon-helpers';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

/** Size variants for the icon preview */
export type IconPreviewSize = 'xs' | 'sidebar' | 'sidebarExpanded' | 'sm' | 'md' | 'lg' | 'xl';

/** Size configuration mapping */
const SIZE_CONFIG = {
  xs: { container: 32, fontSize: 'text-xs' },
  sidebar: { container: 36, fontSize: 'text-xs' },
  sidebarExpanded: { container: 40, fontSize: 'text-sm' },
  sm: { container: 48, fontSize: 'text-sm' },
  md: { container: 64, fontSize: 'text-base' },
  lg: { container: 80, fontSize: 'text-xl' },
  xl: { container: 96, fontSize: 'text-2xl' },
} as const;

interface IconPreviewProps {
  /** Icon type */
  iconType?: IconType;
  /** Text icon content */
  icon?: string;
  /** Background color for text icon */
  iconBackground?: string;
  /** Image source URL or data URL */
  src?: string | null;
  /** Alt text for the image */
  alt?: string;
  /** Size variant of the preview container */
  size?: IconPreviewSize;
  /** Whether to enable editing functionality */
  editable?: boolean;
  /** Whether to show upload functionality */
  showUpload?: boolean;
  /** Whether to show remove functionality */
  showRemove?: boolean;
  /** Custom upload button text */
  uploadText?: string;
  /** Custom placeholder text */
  placeholderText?: string;
  /** Additional CSS classes */
  className?: string;
  /** Callback when image is selected */
  onImageSelect?: (file: File) => void;
  /** Callback when image is removed */
  onImageRemove?: () => void;
  /** Callback when upload button is clicked */
  onUploadClick?: () => void;
  /** Callback when icon is clicked */
  onIconClick?: () => void;
  /** Accepted file types */
  accept?: string;
  /** Whether the component is disabled */
  disabled?: boolean;
}

export function IconPreview({
  iconType = 'image',
  icon = ICON_TEXT,
  iconBackground = ICON_BG,
  src,
  alt = ICON_TEXT,
  size = 'lg',
  editable = true,
  showUpload = true,
  showRemove = true,
  uploadText = 'Upload Image',
  className,
  onImageSelect,
  onImageRemove,
  onUploadClick,
  onIconClick,
  accept = 'image/*',
  disabled = false,
}: IconPreviewProps) {
  const [isHovered, setIsHovered] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const [imageError, setImageError] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const t = useT();

  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file && onImageSelect) {
      onImageSelect(file);
    }
    // Reset input value to allow selecting the same file again
    event.target.value = '';
  };

  const handleUploadClick = () => {
    if (onUploadClick) {
      onUploadClick();
    } else if (fileInputRef.current) {
      fileInputRef.current.click();
    }
  };

  const handleIconClick = () => {
    if (!disabled && editable && onIconClick) {
      onIconClick();
    }
  };

  const handleRemove = (event: React.MouseEvent) => {
    event.stopPropagation();
    if (onImageRemove) {
      onImageRemove();
    }
  };

  // Handle drag events
  const handleDragEnter = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    if (!disabled) {
      setIsDragging(true);
    }
  };

  const handleDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  };

  const handleDragOver = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    if (!disabled && !isDragging) {
      setIsDragging(true);
    }
  };

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    if (disabled) return;

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      const file = e.dataTransfer.files[0];
      if (file.type.startsWith('image/') && onImageSelect) {
        onImageSelect(file);
      }
    }
  };

  const sizeConfig = SIZE_CONFIG[size];

  // Extract icon data
  const iconText = icon || ICON_TEXT;
  const iconBg = iconBackground;
  const containerStyle = {
    width: `${sizeConfig.container}px`,
    height: `${sizeConfig.container}px`,
  };

  // Reset error state when src changes
  React.useEffect(() => {
    setImageError(false);
  }, [src]);

  return (
    <div className={cn('relative', className)}>
      {/* Hidden file input */}
      {editable && showUpload && onImageSelect && (
        <input
          ref={fileInputRef}
          type="file"
          accept={accept}
          onChange={handleFileSelect}
          className="hidden"
          disabled={disabled}
        />
      )}

      {/* Preview container */}
      <div
        className={cn(
          'relative rounded-lg overflow-hidden transition-all duration-200',
          'flex items-center justify-center',
          editable && !disabled ? 'cursor-pointer hover:border-primary/60 hover:bg-muted/20' : '',
          disabled ? 'opacity-50 cursor-not-allowed' : '',
          isDragging ? 'border-primary bg-primary/5' : '',
          iconType === 'text' || src
            ? 'border-none'
            : 'border-muted-foreground/40 border-2 border-dashed',
          className
        )}
        style={containerStyle}
        onMouseEnter={() => !disabled && editable && setIsHovered(true)}
        onMouseLeave={() => !disabled && editable && setIsHovered(false)}
        onClick={
          !disabled && editable
            ? iconType === 'text'
              ? handleIconClick
              : handleUploadClick
            : undefined
        }
        onDragEnter={editable ? handleDragEnter : undefined}
        onDragOver={editable ? handleDragOver : undefined}
        onDragLeave={editable ? handleDragLeave : undefined}
        onDrop={editable ? handleDrop : undefined}
      >
        {iconType === 'text' ? (
          // Text icon preview
          <>
            <div
              className="w-full h-full flex items-center justify-center"
              style={{ backgroundColor: iconBg }}
            >
              <span className={cn(sizeConfig.fontSize, 'font-bold text-white')}>{iconText}</span>
            </div>
            {/* Overlay for text icon when editable and hovered */}
            {editable && isHovered && (
              <div className="absolute inset-0 bg-black/30 flex items-center justify-center">
                <div className="text-white text-xs">Click to edit</div>
              </div>
            )}
          </>
        ) : src && !imageError ? (
          // Image preview
          <>
            <img
              src={src}
              alt={alt}
              className="object-cover w-full h-full"
              onError={() => {
                setImageError(true);
              }}
              onLoad={() => {
                // Reset error state on successful load
                setImageError(false);
              }}
            />

            {/* Overlay with remove button */}
            {editable && showRemove && isHovered && (
              <div className="absolute inset-0 bg-black/50 flex items-center justify-center">
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={handleRemove}
                  className="h-8 w-8 p-0 rounded-full"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            )}
          </>
        ) : imageError && src ? (
          // Error state - show alt text with primary background
          <div
            className={cn(
              'flex items-center justify-center bg-primary w-full h-full text-primary-foreground p-2 rounded-lg',
              sizeConfig.fontSize
            )}
          >
            <span className="text-center break-words leading-tight">{alt}</span>
          </div>
        ) : (
          // Placeholder with upload icon and text
          <div className="flex flex-col items-center justify-center text-center p-4">
            <div
              className={`w-12 h-12 rounded-full flex items-center justify-center ${isDragging ? 'bg-primary/20' : ''}`}
            >
              {isDragging ? (
                <Upload size={24} className="text-primary" />
              ) : (
                <Plus size={24} className="text-muted-foreground" />
              )}
            </div>
            {isDragging && (
              <p className="text-sm text-primary mt-2">{t('common.iconInput.dropToUpload')}</p>
            )}
          </div>
        )}
      </div>

      {/* Upload button (optional, shown below preview) */}
      {editable && showUpload && iconType === 'image' && !src && (
        <Button
          variant="outline"
          size="sm"
          className="w-full mt-2"
          onClick={handleUploadClick}
          disabled={disabled}
        >
          <Upload className="h-4 w-4 mr-2" />
          {uploadText}
        </Button>
      )}
    </div>
  );
}
