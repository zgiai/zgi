'use client';

import React from 'react';
import { useT } from '@/i18n';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { cn } from '@/lib/utils';
import { SegmentHeader } from './segment-header';
import { SecondaryChunks } from './secondary-chunks';
import type { SegmentCardProps } from './types';

/**
 * SegmentCard component - displays a single segment with all its details
 */
export const SegmentCard = React.memo(function SegmentCard({
  segment,
  displayIndex,
  isSelected,
  isExpanded,
  onSelect,
  onToggleExpand,
  onToggleEnabled,
  onEdit,
  onDelete,
  onViewChildChunks,
  onViewAllChildSegments,
  onEditChildSegment,
  onDeleteChildSegment,
  readOnly = false,
}: SegmentCardProps) {
  const t = useT('datasets');

  return (
    <Card className={isSelected ? 'ring-2 ring-primary' : ''}>
      <CardContent className="p-4">
        <div className="space-y-3">
          {/* Header */}
          <SegmentHeader
            segment={segment}
            displayIndex={displayIndex}
            isSelected={isSelected}
            onSelect={onSelect}
            onToggleEnabled={onToggleEnabled}
            onEdit={onEdit}
            onDelete={onDelete}
            onViewChildChunks={onViewChildChunks}
            readOnly={readOnly}
          />

          {/* Content */}
          <div className="flex items-start justify-between gap-2">
            <MarkdownViewer
              content={segment.content}
              className={cn(
                'text-sm leading-6 flex-1 min-w-0 break-words',
                isExpanded ? '' : 'line-clamp-3'
              )}
            />
            <Button
              variant="ghost"
              size="sm"
              className="shrink-0"
              onClick={() => onToggleExpand(segment.id)}
            >
              {isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
            </Button>
          </div>

          {/* Keywords */}
          {segment.keywords && segment.keywords.length > 0 && (
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-xs text-muted-foreground">{t('common.keywords')}:</span>
              {segment.keywords.map((keyword, index) => (
                <Badge key={index} variant="outline" className="text-xs">
                  {keyword}
                </Badge>
              ))}
            </div>
          )}

          {/* Error message */}
          {segment.error && (
            <div className="p-2 bg-destructive/10 border border-destructive/20 rounded text-sm text-destructive">
              {segment.error}
            </div>
          )}

          {/* Expanded content: Secondary chunks with inline edit/delete */}
          {isExpanded && (
            <SecondaryChunks
              segment={segment}
              onViewAllChildSegments={onViewAllChildSegments}
              onEditChildSegment={onEditChildSegment}
              onDeleteChildSegment={onDeleteChildSegment}
              readOnly={readOnly}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
});
