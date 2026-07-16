import React, { useState } from 'react';
import { Download, ImageOff, Loader2, X, ZoomIn } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle, DialogClose } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n/translations';

interface MarkdownImageProps extends React.ImgHTMLAttributes<HTMLImageElement> {
  className?: string;
  frameClassName?: string;
  imageClassName?: string;
}

const loadedMarkdownImageSrcs = new Set<string>();
const LOADED_MARKDOWN_IMAGE_SRC_LIMIT = 300;

function isMarkdownImageLoaded(src: unknown): boolean {
  return typeof src === 'string' && loadedMarkdownImageSrcs.has(src);
}

function markMarkdownImageLoaded(src: unknown): void {
  if (typeof src !== 'string' || !src) return;

  loadedMarkdownImageSrcs.add(src);
  if (loadedMarkdownImageSrcs.size <= LOADED_MARKDOWN_IMAGE_SRC_LIMIT) return;

  const oldestSrc = loadedMarkdownImageSrcs.values().next().value;
  if (oldestSrc) {
    loadedMarkdownImageSrcs.delete(oldestSrc);
  }
}

export function MarkdownImage({
  src,
  alt,
  className,
  frameClassName,
  imageClassName,
  onLoad,
  onError,
  ...props
}: MarkdownImageProps) {
  const t = useT('webapp');
  const [isLoading, setIsLoading] = useState(() => Boolean(src) && !isMarkdownImageLoaded(src));
  const [hasError, setHasError] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const lastOpenTimeRef = React.useRef<number>(0);
  // Cache the blob URL to avoid reloading image when opening dialog
  const [blobUrl, setBlobUrl] = useState<string | null>(null);

  React.useEffect(() => {
    return () => {
      if (blobUrl) URL.revokeObjectURL(blobUrl);
    };
  }, [blobUrl]);

  React.useEffect(() => {
    setIsLoading(Boolean(src) && !isMarkdownImageLoaded(src));
    setHasError(false);
    setBlobUrl(currentBlobUrl => {
      if (currentBlobUrl) URL.revokeObjectURL(currentBlobUrl);
      return null;
    });
  }, [src]);

  const loadBlob = async () => {
    if (blobUrl || !src) return;
    try {
      const response = await fetch(src);
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      setBlobUrl(url);
    } catch (e) {
      console.error('Failed to load image blob', e);
    }
  };

  const handleOpenChange = (open: boolean) => {
    if (open) {
      lastOpenTimeRef.current = Date.now();
      setIsOpen(true);
      loadBlob();
    } else {
      // Prevent closing if opened less than 500ms ago
      if (Date.now() - lastOpenTimeRef.current < 500) {
        return;
      }
      setIsOpen(false);
    }
  };

  const handleDownload = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!src) return;
    try {
      let url = blobUrl;
      if (!url) {
        const response = await fetch(src);
        const blob = await response.blob();
        url = window.URL.createObjectURL(blob);
      }

      const link = document.createElement('a');
      link.href = url;
      link.download = alt || 'image.png';
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);

      // Only revoke if we created it just now
      if (!blobUrl) {
        window.URL.revokeObjectURL(url);
      }
    } catch (error) {
      console.error('Download failed:', error);
      // Fallback: open in new tab
      window.open(src, '_blank');
    }
  };

  if (hasError) {
    return (
      <div
        role="img"
        aria-label={alt ? `${t('chat.markdownImage.loadFailedTitle')}: ${alt}` : t('chat.markdownImage.loadFailedTitle')}
        className={cn(
          'relative inline-flex min-h-36 w-full max-w-[320px] min-w-0 items-center justify-center overflow-hidden rounded-lg border border-dashed border-border bg-muted/25 align-top text-muted-foreground',
          className
        )}
      >
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_1px_1px,hsl(var(--muted-foreground)/0.16)_1px,transparent_0)] [background-size:16px_16px]" />
        <div className="relative flex w-full flex-col items-center justify-center gap-3 px-5 py-5 text-center">
          <div className="flex size-10 items-center justify-center rounded-full border border-border bg-background/85 text-muted-foreground shadow-sm">
            <ImageOff className="size-5" />
          </div>
          <div className="space-y-1">
            <div className="text-sm font-medium text-foreground">
              {t('chat.markdownImage.loadFailedTitle')}
            </div>
            <div className="mx-auto max-w-56 text-xs leading-5 text-muted-foreground">
              {t('chat.markdownImage.loadFailedDescription')}
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className={cn(
        'relative group inline-flex max-w-full rounded-lg overflow-hidden align-top',
        className
      )}
    >
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center bg-muted rounded-lg min-h-[200px] w-full">
          <Skeleton className="w-full h-full absolute inset-0" />
          <Loader2 className="w-6 h-6 animate-spin text-muted-foreground z-10" />
        </div>
      )}

      <Dialog open={isOpen} onOpenChange={handleOpenChange}>
        <div
          className={cn(
            'relative cursor-zoom-in overflow-hidden rounded-lg border bg-background',
            isLoading ? 'min-h-[200px]' : '',
            frameClassName
          )}
          onClick={() => handleOpenChange(true)}
        >
          <img
            src={src}
            alt={alt}
            className={cn(
              'max-h-[400px] w-auto h-auto object-contain transition-opacity duration-300 block',
              imageClassName,
              isLoading ? 'opacity-0' : 'opacity-100'
            )}
            onLoad={event => {
              markMarkdownImageLoaded(src);
              setIsLoading(false);
              onLoad?.(event);
            }}
            onError={event => {
              if (typeof src === 'string') {
                loadedMarkdownImageSrcs.delete(src);
              }
              setIsLoading(false);
              setHasError(true);
              onError?.(event);
            }}
            {...props}
          />
          {!isLoading && (
            <div className="absolute inset-0 bg-black/0 group-hover:bg-black/10 transition-colors duration-200" />
          )}
        </div>

        {!isLoading && (
          <div className="absolute top-2 right-2 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-200 z-10">
            <Button
              variant="secondary"
              isIcon
              className="h-8 w-8 rounded-full bg-background/80 backdrop-blur-sm shadow-sm hover:bg-background"
              onClick={e => {
                e.stopPropagation();
                handleOpenChange(true);
              }}
              title={t('chat.markdownImage.zoomIn')}
            >
              <ZoomIn className="h-4 w-4" />
            </Button>
            <Button
              variant="secondary"
              isIcon
              className="h-8 w-8 rounded-full bg-background/80 backdrop-blur-sm shadow-sm hover:bg-background"
              onClick={handleDownload}
              title={t('chat.markdownImage.download')}
            >
              <Download className="h-4 w-4" />
            </Button>
          </div>
        )}

        <DialogContent
          showCloseButton={false}
          className="w-fit min-w-64 max-w-[90vw] overflow-visible border-none bg-transparent p-0 shadow-none"
        >
          <DialogTitle className="sr-only">{t('chat.markdownImage.previewTitle')}</DialogTitle>
          <div className="relative flex min-w-64 items-center justify-center">
            <img
              src={blobUrl || src}
              alt={alt}
              className="max-h-[85vh] min-w-64 max-w-[90vw] rounded-lg object-contain shadow-2xl"
            />
            <div className="absolute -top-12 right-0 flex gap-2">
              <Button
                variant="secondary"
                isIcon
                className="h-10 w-10 rounded-full bg-background/80 backdrop-blur-sm hover:bg-background"
                onClick={handleDownload}
              >
                <Download className="h-5 w-5" />
              </Button>
              <DialogClose asChild>
                <Button
                  variant="secondary"
                  isIcon
                  className="h-10 w-10 rounded-full bg-background/80 backdrop-blur-sm hover:bg-background"
                >
                  <X className="h-5 w-5" />
                </Button>
              </DialogClose>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
