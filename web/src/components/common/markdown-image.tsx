import React, { useState } from 'react';
import { Download, ZoomIn, Loader2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle, DialogClose } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

interface MarkdownImageProps extends React.ImgHTMLAttributes<HTMLImageElement> {
  className?: string;
  frameClassName?: string;
  imageClassName?: string;
}

export function MarkdownImage({
  src,
  alt,
  className,
  frameClassName,
  imageClassName,
  ...props
}: MarkdownImageProps) {
  const [isLoading, setIsLoading] = useState(true);
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
    setIsLoading(true);
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
        className={cn(
          'relative inline-flex items-center justify-center bg-muted rounded-lg border border-border p-4',
          className
        )}
      >
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <div className="h-8 w-8 rounded-full bg-background flex items-center justify-center">
            <X className="h-4 w-4" />
          </div>
          <span className="text-xs">Failed to load image</span>
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
            onLoad={() => setIsLoading(false)}
            onError={() => {
              setIsLoading(false);
              setHasError(true);
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
              title="Zoom In"
            >
              <ZoomIn className="h-4 w-4" />
            </Button>
            <Button
              variant="secondary"
              isIcon
              className="h-8 w-8 rounded-full bg-background/80 backdrop-blur-sm shadow-sm hover:bg-background"
              onClick={handleDownload}
              title="Download"
            >
              <Download className="h-4 w-4" />
            </Button>
          </div>
        )}

        <DialogContent
          showCloseButton={false}
          className="max-w-screen-lg w-auto h-auto p-0 border-none bg-transparent shadow-none"
        >
          <DialogTitle className="sr-only">Image Preview</DialogTitle>
          <div className="relative flex items-center justify-center">
            <img
              src={blobUrl || src}
              alt={alt}
              className="max-w-[90vw] max-h-[85vh] object-contain rounded-lg shadow-2xl"
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
