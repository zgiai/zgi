import { AppWindow, AtSign, Github, Globe2, Mail, MessageSquareText, PlugZap } from 'lucide-react';
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
  const identity = `${integrationId} ${driverId ?? ''}`.toLowerCase();
  const Icon = identity.includes('github')
    ? Github
    : identity.includes('gmail') || identity.includes('google-mail')
      ? Mail
      : identity === 'x' || identity.includes(' x-rest') || identity.includes('twitter')
        ? AtSign
        : identity.includes('feishu') || identity.includes('lark')
          ? MessageSquareText
          : identity.includes('web-search') || identity.includes('exa')
            ? Globe2
            : identity.includes('app')
              ? AppWindow
              : PlugZap;

  return <Icon className={cn('size-5', className)} aria-hidden="true" />;
}
