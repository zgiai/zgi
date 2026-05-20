import React, { useEffect, useRef, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FileIcon } from '@/components/ui/file-icon';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { AlertCircle, Trash2 } from 'lucide-react';

export interface AttachmentDisplayItem {
  id: string;
  filename: string;
  extension: string;
  progress: number;
  status: 'uploading' | 'uploaded' | 'error';
  error?: string;
}

interface AttachmentsPanelProps {
  items: AttachmentDisplayItem[];
  isUploading: boolean;
  sessionIds: string[];
  onRemove: (id: string) => void;
}

const AttachmentsPanel: React.FC<AttachmentsPanelProps> = ({
  items,
  isUploading,
  sessionIds,
  onRemove,
}) => {
  const t = useT('ui');
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [hasOverflow, setHasOverflow] = useState(false);

  useEffect(() => {
    const element = scrollRef.current;

    if (!element) {
      setHasOverflow(false);
      return;
    }

    const updateOverflow = () => {
      setHasOverflow(element.scrollWidth - element.clientWidth > 4);
    };

    updateOverflow();

    if (typeof ResizeObserver === 'undefined') {
      return;
    }

    const observer = new ResizeObserver(() => {
      updateOverflow();
    });

    observer.observe(element);
    Array.from(element.children).forEach(child => observer.observe(child));

    return () => {
      observer.disconnect();
    };
  }, [items]);

  if (items.length === 0) return null;
  return (
    <div
      ref={scrollRef}
      data-overflow={hasOverflow ? 'true' : 'false'}
      className={`attachments-scrollbar flex flex-nowrap items-start gap-2 overflow-x-auto overflow-y-hidden transition-all duration-100 px-2 ${
        hasOverflow ? 'pb-1' : 'pb-0.5'
      }`}
    >
      {items.map(item => {
        const baseClasses = 'relative overflow-hidden rounded-xl px-2 py-0.5 shadow border';
        const statusClasses =
          item.status === 'error'
            ? 'bg-destructive/15 border-destructive/40'
            : item.status === 'uploaded'
              ? 'bg-emerald-500/10 border-emerald-500/40'
              : 'bg-muted border-muted/60';
        const overlayWidth = `${Math.max(0, Math.min(item.progress, 100))}%`;
        const disabled = isUploading && sessionIds.includes(item.id);
        return (
          <div key={item.id} className={`${baseClasses} ${statusClasses} shrink-0`}>
            {item.status === 'uploading' && (
              <div
                className="absolute inset-y-0 left-0 bg-highlight opacity-20 transition-[width] duration-200"
                style={{ width: overlayWidth }}
              />
            )}
            <div className="flex items-center gap-1 relative z-10" title={item.filename}>
              <FileIcon filename={item.filename} size="sm" />
              <div className="max-w-[120px] text-xs leading-4 text-secondary-foreground">
                <div className="truncate">{item.filename}</div>
              </div>
              {item.status === 'error' && (
                <div className="flex items-center gap-1">
                  <Badge variant="destructive" className="h-5 px-1.5 text-[10px]">
                    {t('fileUpload.error')}
                  </Badge>
                  {item.error && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="inline-flex items-center justify-center rounded-sm text-destructive/80 hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          aria-label={t('fileUpload.errorDetails')}
                        >
                          <AlertCircle className="size-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" className="max-w-xs break-all">
                        {item.error}
                      </TooltipContent>
                    </Tooltip>
                  )}
                </div>
              )}
              <Button
                isIcon
                variant="ghost"
                className="h-6 w-6 hover:bg-destructive/10 hover:text-destructive"
                onClick={() => onRemove(item.id)}
                disabled={disabled}
                aria-label="Remove attachment"
              >
                <Trash2 size={14} />
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
};

export default AttachmentsPanel;
