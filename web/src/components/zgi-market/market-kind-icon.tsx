import { Database, Link2, Package, Server, Sparkles, Webhook, type LucideIcon } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { MarketListingKind } from '@/services/types/zgi-market';

const kindIcons: Record<MarketListingKind, LucideIcon> = {
  api: Webhook,
  mcp: Server,
  skill: Sparkles,
  plugin: Package,
  dataset: Database,
  connector: Link2,
};

interface MarketKindIconProps {
  kind: MarketListingKind;
  size?: 'sm' | 'lg';
  className?: string;
}

/** The tile that stands for a listing kind; decoration beside the kind's name. */
export function MarketKindIcon({ kind, size = 'sm', className }: MarketKindIconProps) {
  const Icon = kindIcons[kind];
  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex shrink-0 items-center justify-center border bg-primary/5 text-primary',
        size === 'lg' ? 'size-14 rounded-xl' : 'size-9 rounded-lg',
        className
      )}
    >
      <Icon className={size === 'lg' ? 'size-7' : 'size-[18px]'} />
    </span>
  );
}
