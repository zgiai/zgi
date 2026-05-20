import React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import ManualResizeHandle from './node-resize-handle';
import ImageSafe from '@/components/common/image-safe';

type IconComponent = React.ComponentType<{
  className?: string;
  size?: number | string;
  strokeWidth?: number | string;
}>;

interface NodeCardProps {
  selected?: boolean;
  title: string;
  desc?: string;
  icon: IconComponent;
  // Optional remote icon url for provider/tool logos
  iconUrl?: string;
  badge?: { text: string; className?: string };
  className?: string; // Wrapper div classes
  cardClassName?: string; // Card classes (bg/border)
  titleClassName?: string; // Title color classes
  iconBgClassName?: string; // Icon circle background classes
  iconClassName?: string; // Icon foreground classes
  descClassName?: string; // Description text classes
  contentClassName?: string; // CardContent classes
  onClick?: () => void;
  children?: React.ReactNode; // Main content inside CardContent
  after?: React.ReactNode; // Content rendered after the Card but inside wrapper
  // Enable bottom-right L-shaped resize handle
  showResizeHandle?: boolean;
}

const NodeCard: React.FC<NodeCardProps> = ({
  selected,
  title,
  desc,
  icon: Icon,
  iconUrl,
  badge,
  className,
  cardClassName,
  titleClassName,
  iconBgClassName,
  iconClassName,
  descClassName,
  contentClassName,
  onClick,
  children,
  after,
  showResizeHandle = false,
}) => {
  return (
    <div className={cn('relative group/node', className)} onClick={onClick}>
      <Card
        className={cn(
          'border-transparent bg-white/80 dark:bg-slate-900/80 backdrop-blur-md transition-all duration-200',
          selected
            ? 'ring-1 ring-blue-500/70 shadow-[0_0_15px_rgba(59,130,246,0.2)] border-blue-400/50'
            : 'shadow-sm hover:shadow-md border-slate-200/60 dark:border-slate-800/60',
          cardClassName
        )}
      >
        <CardContent className="p-0 flex flex-col h-full">
          {/* Header area - always has padding */}
          <div className="flex items-center gap-2.5 h-10 shrink-0 px-3.5">
            {iconUrl ? (
              <ImageSafe
                src={iconUrl}
                alt=""
                className="w-5 h-5 rounded-full shrink-0"
                aria-hidden="true"
                loading="lazy"
                referrerPolicy="no-referrer"
                fallbackComponent={
                  <div className="w-6 h-6 rounded-lg flex items-center justify-center shrink-0 bg-primary">
                    <Icon className="w-3.5 h-3.5 text-primary-foreground" strokeWidth={2.5} />
                  </div>
                }
              />
            ) : (
              <div
                className={cn(
                  'w-6 h-6 rounded-lg flex items-center justify-center shrink-0',
                  iconBgClassName
                )}
              >
                <Icon className={cn('w-3.5 h-3.5 text-white', iconClassName)} strokeWidth={2.5} />
              </div>
            )}
            <h3
              className={cn(
                'font-semibold text-sm grow truncate tracking-tight text-slate-900 dark:text-slate-100',
                titleClassName
              )}
            >
              {title}
            </h3>
            {badge && (
              <Badge
                variant="secondary"
                className={cn(
                  'ml-auto text-[10px] px-1.5 h-4.5 font-medium bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 border-none transition-colors shrink-0',
                  badge.className
                )}
              >
                {badge.text}
              </Badge>
            )}
          </div>

          {/* Description area - optional, has padding */}
          {desc && (
            <p
              className={cn(
                'text-[11px] px-3.5 pb-2.5 text-slate-500 dark:text-slate-400 leading-normal line-clamp-2',
                descClassName
              )}
            >
              {desc}
            </p>
          )}

          {/* Body area - padding/layout provided via contentClassName (from theme) */}
          {children && <div className={cn('grow relative', contentClassName)}>{children}</div>}
        </CardContent>
      </Card>

      {showResizeHandle && <ManualResizeHandle minWidth={300} minHeight={200} />}

      {after}
    </div>
  );
};

export default React.memo(NodeCard);
