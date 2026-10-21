'use client';

import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Checkbox } from '@/components/ui/checkbox';
import {
  useChatConnectorPreferences,
  useSetChatConnectorPreference,
} from '@/hooks/connectors/use-chat-connector-preferences';
import { useT } from '@/i18n';
import type { PersonalConnection } from '@/services/types/connector';

/** A separate, opt-in preference write; it never retries credential creation or attachment. */
export function useCreationDefault(enabled = true) {
  const t = useT('connectors');
  const query = useChatConnectorPreferences(enabled);
  const mutation = useSetChatConnectorPreference();
  const [selected, setSelected] = useState(false);
  const intent = useRef<{ selected: boolean; revision?: number; scope: string }>();
  const latestScope = useRef(query.scope);
  latestScope.current = query.scope;
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  useEffect(() => {
    setSelected(false);
    intent.current = undefined;
  }, [query.scope, enabled]);
  return {
    option: (disabled: boolean) =>
      enabled ? (
        <label className="flex items-start gap-2 text-sm">
          <Checkbox
            checked={selected}
            disabled={disabled}
            onCheckedChange={value => setSelected(value === true)}
          />
          <span>{t('defaults.firstUseCheckbox')}</span>
        </label>
      ) : null,
    begin: () => {
      intent.current = {
        selected: enabled && selected,
        revision: query.isError ? undefined : query.data?.data.revision,
        scope: query.scope,
      };
    },
    complete: async (connection: PersonalConnection) => {
      const frozen = intent.current;
      intent.current = undefined;
      const scope = frozen?.scope ?? query.scope;
      const isCurrent = () => alive.current && latestScope.current === scope;
      if (!isCurrent()) return false;
      if (!frozen?.selected) return true;
      try {
        if (frozen.revision === undefined) throw new Error('Default preferences unavailable');
        await mutation.mutateAsync({
          connectorID: connection.connector_id,
          input: {
            mode: 'selected',
            connection_id: connection.id,
            expected_revision: frozen.revision,
          },
        });
        if (alive.current && latestScope.current === frozen.scope) {
          toast.success(t('defaults.saved'));
        }
      } catch {
        if (alive.current && latestScope.current === frozen.scope) {
          toast.error(t('defaults.selectionSavedDefaultFailed'));
        }
      }
      return isCurrent();
    },
  };
}
