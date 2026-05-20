import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import { cn } from '@/lib/utils';

interface ModelFieldSectionProps {
  icon: LucideIcon;
  title: string;
  children: ReactNode;
  required?: boolean;
  description?: string;
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
        </div>
        {description ? <p className="text-xs text-muted-foreground">{description}</p> : null}
      </div>

      {children}

      {errorMessage ? <p className="text-xs text-destructive">{errorMessage}</p> : null}
    </div>
  );
}
