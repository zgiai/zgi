'use client';

import { Star } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  useChatConnectorPreferences,
  useSetChatConnectorPreference,
} from '@/hooks/connectors/use-chat-connector-preferences';
import { useT } from '@/i18n';
import type { PersonalConnection } from '@/services/types/connector';

export function ConnectionChatDefaultButton({
  connection,
  showLabel = false,
}: {
  connection: PersonalConnection;
  showLabel?: boolean;
}) {
  const t = useT('connectors');
  const query = useChatConnectorPreferences();
  const mutation = useSetChatConnectorPreference();
  const preferences = query.isError ? undefined : query.data?.data;
  const entry = preferences?.defaults[connection.connector_id];
  const selected = entry?.mode === 'selected' && entry.connection_id === connection.id;
  const change = async () => {
    if (!preferences) return;
    try {
      await mutation.mutateAsync({
        connectorID: connection.connector_id,
        input: selected
          ? { mode: 'none', expected_revision: preferences.revision }
          : {
              mode: 'selected',
              connection_id: connection.id,
              expected_revision: preferences.revision,
            },
      });
      toast.success(t(selected ? 'defaults.cleared' : 'defaults.saved'));
    } catch {
      toast.error(t('defaults.saveFailed'));
    }
  };
  const label = t(
    query.isError ? 'defaults.loadFailed' : selected ? 'defaults.clear' : 'defaults.set'
  );
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className={cn('shrink-0', showLabel ? 'h-8 gap-1.5 px-2' : 'size-8 p-0')}
      aria-label={label}
      title={label}
      aria-pressed={selected}
      loading={mutation.isPending}
      disabled={
        mutation.isPending ||
        (!query.isError && (!preferences || (!selected && connection.state !== 'ready')))
      }
      onClick={() => (query.isError ? void query.refetch() : void change())}
    >
      <Star aria-hidden="true" className={cn('size-4', selected && 'fill-primary text-primary')} />
      {showLabel
        ? t(query.isError ? 'actions.retry' : selected ? 'defaults.badge' : 'defaults.setShort')
        : null}
    </Button>
  );
}
