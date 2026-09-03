'use client';

import { useState, type ReactNode } from 'react';
import Exa from 'modelicons/Exa';
import { Icons } from '@/components/ui/icons';
import { FeishuIcon } from './feishu-icon';
import { DingTalkIcon, WeComIcon, WeixinIcon } from './messaging-icons';
import { connectorBrand, safeConnectorIconSource } from './connector-icon-model';
import { Database, Plug } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ConnectorClassification } from '@/services/types/connector';

interface ConnectorIconProps {
  connectorId?: string;
  name: string;
  icon?: string;
  classification: ConnectorClassification;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const ROOT_SIZE = {
  sm: 'size-9 rounded-lg',
  md: 'size-10 rounded-xl',
  lg: 'size-14 rounded-2xl',
} as const;

const CONTENT_SIZE = {
  sm: 'size-5',
  md: 'size-7',
  lg: 'size-10',
} as const;

export function ConnectorIcon({
  connectorId,
  name,
  icon,
  classification,
  size = 'md',
  className,
}: ConnectorIconProps) {
  const contentClassName = CONTENT_SIZE[size];
  const fallback =
    classification === 'data_source' ? (
      <Database aria-hidden="true" className={contentClassName} />
    ) : (
      <Plug aria-hidden="true" className={contentClassName} />
    );

  const brand = connectorBrand(connectorId, icon);
  const source = safeConnectorIconSource(icon);
  const mark =
    brand === 'exa' ? (
      <Exa aria-hidden="true" className={contentClassName} />
    ) : brand === 'github' ? (
      <Icons.GitHub aria-hidden="true" className={contentClassName} />
    ) : brand === 'feishu' ? (
      <FeishuIcon aria-hidden="true" className={contentClassName} />
    ) : brand === 'dingtalk' ? (
      <DingTalkIcon aria-hidden="true" className={contentClassName} />
    ) : brand === 'weixin' ? (
      <WeixinIcon aria-hidden="true" className={contentClassName} />
    ) : brand === 'wecom' ? (
      <WeComIcon aria-hidden="true" className={contentClassName} />
    ) : undefined;
  return (
    <div
      role="img"
      aria-label={name}
      className={cn(
        'flex shrink-0 items-center justify-center overflow-hidden border bg-background text-foreground shadow-xs',
        ROOT_SIZE[size],
        className
      )}
    >
      {mark ??
        (source ? (
          <ConnectorImage
            key={source}
            src={source}
            className={contentClassName}
            fallback={fallback}
          />
        ) : (
          fallback
        ))}
    </div>
  );
}

function ConnectorImage({
  src,
  className,
  fallback,
}: {
  src: string;
  className: string;
  fallback: ReactNode;
}) {
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <span className={cn('relative flex items-center justify-center', className)}>
      {!loaded && fallback}
      {!failed && (
        <img
          src={src}
          alt=""
          referrerPolicy="no-referrer"
          className={cn('absolute inset-0 size-full object-contain', !loaded && 'opacity-0')}
          onLoad={() => setLoaded(true)}
          onError={() => {
            setLoaded(false);
            setFailed(true);
          }}
        />
      )}
    </span>
  );
}
