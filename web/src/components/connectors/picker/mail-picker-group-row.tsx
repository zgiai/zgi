'use client';
import { useRef } from 'react';
import { Mail, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useT } from '@/i18n';
import type { ConnectorChatDefault } from '@/services/types/connector';
import { ConnectorPickerGroupRow } from './connector-picker-group-row';
import type { ConnectorPickerMenuGroup } from './connector-picker-menu-model';
import type { ConnectorPickerRow } from './connector-picker-model';
import type { MailPickerGroup } from './mail-picker-groups';

export function MailPickerGroupRow({
  group,
  preferences,
  ...props
}: {
  group: MailPickerGroup;
  preferences?: Record<string, ConnectorChatDefault>;
  targetLabel: string;
  disabled: boolean;
  onSelect: (row: ConnectorPickerRow, trigger?: HTMLButtonElement | null) => void;
  onDetach: (group: ConnectorPickerMenuGroup) => void;
}) {
  const t = useT('connectors');
  const trigger = useRef<HTMLButtonElement>(null);
  const handoff = useRef(false);
  const create = group.providers.flatMap(provider =>
    provider.createRow ? [provider.createRow] : []
  );
  return (
    <section className="space-y-2 rounded-xl border bg-background p-3" aria-label={group.name}>
      <div className="flex items-center justify-between gap-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <Mail aria-hidden="true" className="size-5" />
          {group.name}
          <span className="text-muted-foreground">{group.count}</span>
        </h3>
        {create.length > 0 && (
          <DropdownMenu
            onOpenChange={open => {
              if (open) handoff.current = false;
            }}
          >
            <DropdownMenuTrigger asChild>
              <Button ref={trigger} size="sm" variant="outline" disabled={props.disabled}>
                <Plus aria-hidden="true" className="size-4" />
                {t('connections.addConnection')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              onCloseAutoFocus={event => {
                if (handoff.current) event.preventDefault();
              }}
            >
              {create.map(row => (
                <DropdownMenuItem
                  key={row.connector.id}
                  onSelect={() => {
                    handoff.current = true;
                    props.onSelect(row, trigger.current);
                  }}
                >
                  {row.connector.name}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      {group.providers
        .filter(provider => provider.count > 0 || provider.currentBinding)
        .map(provider => (
          <ConnectorPickerGroupRow
            key={provider.id}
            {...props}
            group={provider}
            preference={preferences?.[provider.id]}
            hideCreate
          />
        ))}
    </section>
  );
}
