'use client';

import { memo } from 'react';
import { Badge } from '@/components/ui/badge';
import { ModelFeatureIcon, type ModelFeatureKey } from './model-feature-icon';

export interface ModelFeatureBadgeProps {
  feature: ModelFeatureKey;
  label: string;
  className?: string;
}

export const ModelFeatureBadge = memo(function ModelFeatureBadge({
  feature,
  label,
  className,
}: ModelFeatureBadgeProps) {
  return (
    <Badge variant="secondary" className={`text-xs gap-1 ${className || ''}`}>
      <ModelFeatureIcon feature={feature} />
      <span className="leading-none">{label}</span>
    </Badge>
  );
});
