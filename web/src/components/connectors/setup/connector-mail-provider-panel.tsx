'use client';

import { useState } from 'react';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useLocale } from 'next-intl';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem } from '@/services/types/connector';
import { selectMailProvider } from '../mail-display-model';
import { mailProviderName } from '../mail-connection-name';
import { ConnectorMailSetupPanel } from './connector-mail-setup-panel';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';

type Props = Omit<ConnectorKeySetupPanelProps, 'connector'> & {
  providers: ConnectorCatalogItem[];
  preferredProvider?: string;
};
export function ConnectorMailProviderPanel(props: Props) {
  const actor = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  return (
    <MailProviderSelection
      key={JSON.stringify([org, actor, props.target, props.preferredProvider])}
      {...props}
    />
  );
}
function MailProviderSelection({ providers, preferredProvider, ...props }: Props) {
  const t = useT('connectors');
  const locale = useLocale();
  const [provider, setProvider] = useState(
    () => selectMailProvider(providers, preferredProvider)?.id
  );
  const [busy, setBusy] = useState(false);
  const selected = selectMailProvider(providers, provider);
  if (!selected) return null;
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="mail-provider">{t('mail.provider')}</Label>
        <Select
          value={selected.id}
          disabled={busy || providers.length === 1}
          onValueChange={value => {
            if (!busy) setProvider(value);
          }}
        >
          <SelectTrigger id="mail-provider">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {providers.map(item => (
              <SelectItem
                key={item.id}
                value={item.id}
                disabled={item.setup_availability?.enabled === false}
              >
                {mailProviderName(item.id, locale)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <ConnectorMailSetupPanel
        key={selected.id}
        {...props}
        connector={selected}
        onBusyChange={setBusy}
      />
    </div>
  );
}
