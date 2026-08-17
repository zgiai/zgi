'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type {
  IntegrationConnection,
  IntegrationConnectionHealthState,
  IntegrationProviderHealthState,
} from '@/services/types/integration';
import { resolveConnectionHealthState } from './integration-utils';

const CONNECTION_VARIANTS: Record<
  IntegrationConnectionHealthState,
  'success' | 'secondary' | 'warning' | 'destructive' | 'outline'
> = {
  ready: 'success',
  testing: 'secondary',
  degraded: 'warning',
  expired: 'destructive',
  revoked: 'destructive',
  error: 'destructive',
  disabled: 'secondary',
  unknown: 'outline',
};

const PROVIDER_VARIANTS: Record<
  IntegrationProviderHealthState,
  'success' | 'warning' | 'destructive' | 'outline'
> = {
  ready: 'success',
  configured: 'outline',
  setup_required: 'warning',
  degraded: 'warning',
  unavailable: 'destructive',
  unknown: 'outline',
};

export function IntegrationConnectionHealthBadge({
  connection,
}: {
  connection: IntegrationConnection;
}) {
  const t = useT('integrations');
  const state = resolveConnectionHealthState(connection);
  return <Badge variant={CONNECTION_VARIANTS[state]}>{t(`health.connection.${state}`)}</Badge>;
}

export function IntegrationProviderHealthBadge({
  state,
}: {
  state: IntegrationProviderHealthState;
}) {
  const t = useT('integrations');
  return <Badge variant={PROVIDER_VARIANTS[state]}>{t(`health.provider.${state}`)}</Badge>;
}
