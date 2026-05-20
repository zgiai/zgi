import React from 'react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { AlertCircle } from 'lucide-react';
import { useResolvedVariableReference } from '../hooks';

interface ValueBadgeProps {
  selector?: string[];
  template?: string; // e.g. {{#nodeId.variable#}}
  className?: string;
  currentNodeId?: string;
}

const ValueBadge: React.FC<ValueBadgeProps> = ({
  selector,
  template,
  className,
  currentNodeId,
}) => {
  const t = useT();
  const resolved = useResolvedVariableReference({
    selector,
    template,
    currentNodeId,
  });

  if (!resolved) return null;

  return (
    <Badge
      variant="outline"
      aria-invalid={resolved.invalid || undefined}
      className={cn(
        'max-w-full flex items-center gap-1 bg-background rounded-sm',
        resolved.invalid && 'border-destructive',
        className
      )}
      title={resolved.displayText}
    >
      <span className="truncate break-all min-w-1 overflow-hidden">{resolved.sourceTitle}</span>
      <span className="text-xs text-highlight min-w-1 overflow-hidden break-all">
        ({resolved.displayPath})
      </span>
      {resolved.invalid ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="shrink-0 text-destructive"
              aria-label={t('nodes.validation.invalidVariable')}
            >
              <AlertCircle size={14} />
            </span>
          </TooltipTrigger>
          <TooltipContent side="top" className="px-2 pt-1 pb-2 text-xs">
            {t('nodes.validation.invalidVariable')}
          </TooltipContent>
        </Tooltip>
      ) : null}
    </Badge>
  );
};

export default ValueBadge;
