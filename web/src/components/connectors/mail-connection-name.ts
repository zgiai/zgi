import { isMailProvider, type MailProviderID } from './mail-display-model';

interface MailAccount {
  connector_id: string;
  name?: string;
  external_account_id?: string;
  external_display_name?: string;
}
const LABELS: Record<MailProviderID, [string, string]> = {
  qqmail: ['QQ邮箱', 'QQ Mail'],
  netease163: ['163邮箱', '163 Mail'],
  gmail: ['Gmail', 'Gmail'],
};
export function mailProviderName(id: string, locale = 'zh-Hans'): string {
  return isMailProvider(id) ? LABELS[id][locale.startsWith('en') ? 1 : 0] : id;
}
export function mailConnectionAlias(connection: MailAccount): string {
  const name = connection.name?.trim() ?? '';
  if (!isMailProvider(connection.connector_id)) return name;
  for (const prefix of LABELS[connection.connector_id]) {
    if (name.startsWith(`${prefix}-`)) return name.slice(prefix.length + 1);
  }
  return name;
}
export function mailConnectionName(
  connection: MailAccount,
  locale = 'zh-Hans'
): string | undefined {
  if (!isMailProvider(connection.connector_id)) return undefined;
  const provider = mailProviderName(connection.connector_id, locale);
  const alias = mailConnectionAlias(connection);
  const email = connection.external_account_id?.trim() || connection.external_display_name?.trim();
  const name =
    !alias || alias === email
      ? email
        ? locale.startsWith('en')
          ? `${email.split('@')[0]}'s ${provider}`
          : `${email.split('@')[0]}的${provider}`
        : provider
      : alias;
  return `${provider}-${name}`;
}
