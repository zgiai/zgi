import { mailConnectionName } from '../mail-connection-name';
import { mailDisplayKey } from '../mail-display-model';
import type { PersonalConnection } from '../../../services/types/connector.ts';

export interface PersonalConnectionGroup {
  connectorID: string;
  connections: PersonalConnection[];
}

export function personalConnectionDisplayName(
  connection: PersonalConnection,
  fallback: string,
  locale = 'zh-Hans'
): string {
  return (
    mailConnectionName(connection, locale) ||
    connection.name.trim() ||
    connection.external_display_name?.trim() ||
    fallback
  );
}

export function personalConnectionSecondaryIdentity(
  connection: PersonalConnection,
  fallback: string
): string {
  return (
    connection.external_display_name?.trim() || connection.external_account_id?.trim() || fallback
  );
}

export function groupPersonalConnections(
  connections: readonly PersonalConnection[]
): PersonalConnectionGroup[] {
  const grouped = new Map<string, PersonalConnection[]>();
  connections.forEach(connection => {
    const connectorID = mailDisplayKey(connection.connector_id.trim());
    if (!connectorID) return;
    const items = grouped.get(connectorID) ?? [];
    items.push(connection);
    grouped.set(connectorID, items);
  });
  return Array.from(grouped.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([connectorID, items]) => ({
      connectorID,
      connections: [...items].sort((left, right) =>
        personalConnectionDisplayName(left, left.id).localeCompare(
          personalConnectionDisplayName(right, right.id)
        )
      ),
    }));
}
