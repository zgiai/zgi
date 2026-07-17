import type { ReactNode } from 'react';
import { Info, type LucideIcon } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

interface ModelFieldSectionProps {
  icon: LucideIcon;
  title: string;
  children: ReactNode;
  required?: boolean;
  description?: string;
  titleTooltip?: string;
  errorMessage?: string;
  className?: string;
}

/**
 * @component ModelFieldSection
 * @category Feature
 * @status Stable
 * @description Shared presentation shell for dataset model selector labels and validation text
 * @usage Wrap embedding or graph model selectors so their headers stay visually consistent
 * @example
 * <ModelFieldSection icon={Brain} title="Embedding Model">
 *   <ModelSelector ... />
 * </ModelFieldSection>
 */
export function ModelFieldSection({
  icon: Icon,
  title,
  children,
  required = false,
  description,
  titleTooltip,
  errorMessage,
  className,
}: ModelFieldSectionProps) {
  return (
    <div className={cn('max-w-md space-y-2.5', className)}>
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <Icon className="h-4 w-4" />
          <h3 className="text-sm font-semibold">
            {title}
            {required ? <span className="ml-1 text-destructive">*</span> : null}
          </h3>
          {titleTooltip ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label={titleTooltip}
                >
                  <Info className="h-3.5 w-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent side="top" align="start" className="max-w-72 text-sm leading-6">
                {titleTooltip}
              </TooltipContent>
            </Tooltip>
          ) : null}
        </div>
        {description ? <p className="text-xs text-muted-foreground">{description}</p> : null}
      </div>

      {children}

      {errorMessage ? <p className="text-xs text-destructive">{errorMessage}</p> : null}
    </div>
  );
}
