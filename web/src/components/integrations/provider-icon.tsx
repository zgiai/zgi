import { AppWindow, Github, Globe2, Mail, MessageSquareText, PlugZap } from 'lucide-react';
import { cn } from '@/lib/utils';

interface IntegrationProviderIconProps {
  integrationId: string;
  driverId?: string;
  className?: string;
}

export function IntegrationProviderIcon({
  integrationId,
  driverId,
  className,
}: IntegrationProviderIconProps) {
  const identities = [integrationId, driverId]
    .filter((value): value is string => Boolean(value))
    .map(value => value.trim().toLowerCase());
  const identity = identities.join(' ');
  const isX = identities.some(
    value => value === 'x' || value === 'twitter' || value.startsWith('x-api')
  );

  if (isX) {
    return (
      <span
        className={cn(
          'inline-flex size-5 items-center justify-center font-bold leading-none',
          className
        )}
        aria-hidden="true"
      >
        X
      </span>
    );
  }

  const Icon = identity.includes('github')
    ? Github
    : identity.includes('gmail') || identity.includes('google-mail')
      ? Mail
      : identity.includes('feishu') || identity.includes('lark')
        ? MessageSquareText
        : identity.includes('web-search') || identity.includes('exa')
          ? Globe2
          : identity.includes('app')
            ? AppWindow
            : PlugZap;

  return <Icon className={cn('size-5', className)} aria-hidden="true" />;
}
