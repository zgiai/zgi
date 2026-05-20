'use client';

import React from 'react';
import { useT } from '@/i18n';
import { Edit, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import type { SegmentHeaderProps } from './types';

/**
 * Segment header component - displays position, status, and action buttons
 */
export const SegmentHeader = React.memo(function SegmentHeader({
  segment,
  displayIndex,
  isSelected,
  onSelect,
  onToggleEnabled,
  onEdit,
  onDelete,
  onViewChildChunks,
}: SegmentHeaderProps) {
  const t = useT('datasets');

  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-3">
        <Checkbox
          checked={isSelected}
          onCheckedChange={() => onSelect(segment.id)}
          aria-label={`${t('segments.selectSegmentLabel')} ${displayIndex}`}
        />
        <span className="text-[var(--tag-primary-text)]">#{displayIndex}</span>
        <Badge variant="default">{t('segments.primaryChunk')}</Badge>
        <Switch
          checked={segment.enabled}
          onCheckedChange={() => onToggleEnabled([segment.id], !segment.enabled)}
        />
      </div>

      <div className="flex items-center gap-1">
        <span className="text-xs text-muted-foreground">
          {segment.word_count} {t('segments.characters')}
        </span>
        <span className="text-xs text-muted-foreground mr-2">
          {segment.hit_count} {t('segments.recalls')}
        </span>
        {segment.child_chunks && segment.child_chunks.length > 0 && onViewChildChunks && (
          <Button variant="ghost" size="sm" onClick={() => onViewChildChunks(segment)}>
            {t('segments.childSegmentsCount', {
              count: segment.child_chunks.length,
            })}
          </Button>
        )}

        <Edit className="h-4 w-4 mr-2 cursor-pointer" onClick={() => onEdit(segment)} />
        <Trash2 className="h-4 w-4 mr-2 cursor-pointer" onClick={() => onDelete([segment.id])} />
      </div>
    </div>
  );
});
