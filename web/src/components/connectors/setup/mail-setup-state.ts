import type {
  ConnectorMailUsage,
  ConnectorMailVerification,
  PersonalConnection,
} from '../../../services/types/connector.ts';
export { supportsMailSetup, mailSetupEnabled } from './app-setup-state';

/** Preserve dots, plus suffixes and domain aliases: the login is the account identity. */
export function normalizeMailEmail(value: string): string | undefined {
  if (
    Array.from(value).some(
      character =>
        character.charCodeAt(0) < 32 ||
        (character.charCodeAt(0) >= 127 && character.charCodeAt(0) <= 159)
    ) ||
    /[\u200b-\u200f\u202a-\u202e\u2060-\u206f\ufeff]/.test(value)
  ) {
    return undefined;
  }
  const email = value.trim();
  return email.length <= 254 &&
    /^[^\s<>(),;:"\\@]+@[^\s<>(),;:"\\@]+\.[^\s<>(),;:"\\@]+$/.test(email)
    ? email.slice(0, email.lastIndexOf('@') + 1) +
        email.slice(email.lastIndexOf('@') + 1).toLowerCase()
    : undefined;
}

/** Google displays app passwords in groups. Strip only the documented copy separators. */
export function normalizeMailAppPassword(value: string): string | undefined {
  const password = value.replace(/[ \t\r\n\u00a0]/g, '');
  return /^[\x21-\x7e]{16}$/.test(password) ? password : undefined;
}

export function connectionMailUsage(connection?: PersonalConnection): ConnectorMailUsage {
  if (!connection) return 'read_send';
  const usage = connection.provider_evidence?.mail?.usage;
  if (usage === 'read' || usage === 'send' || usage === 'read_send') return usage;
  const operations = connection.usable_operation_ids ?? [];
  const prefix = connection.connector_id;
  const read = ['folder.list', 'message.list', 'message.get'].some(id =>
    operations.includes(`${prefix}.${id}`)
  );
  const send = operations.includes(`${prefix}.message.send`);
  return send ? (read ? 'read_send' : 'send') : 'read';
}

export function clearMailValues(values: Record<string, string>): void {
  for (const name of Object.keys(values)) {
    values[name] = '';
    delete values[name];
  }
}

export function mailFailureMessage(
  code?: string
):
  | 'mail.policyRestricted'
  | 'mail.authenticationRejected'
  | 'mail.tlsFailed'
  | 'mail.failed'
  | 'mail.clientIdentityRequired'
  | 'mail.authMechanismUnsupported' {
  if (
    code === 'connector_mail_client_identity_required' ||
    code === 'connector.mail.client_identity_required'
  ) {
    return 'mail.clientIdentityRequired';
  }
  if (
    code === 'connector_mail_auth_mechanism_unsupported' ||
    code === 'connector.mail.auth_mechanism_unsupported'
  ) {
    return 'mail.authMechanismUnsupported';
  }
  if (code === 'connector_mail_policy_restricted' || code === 'connector.mail.policy_restricted') {
    return 'mail.policyRestricted';
  }
  if (
    code === 'connector_mail_authentication_rejected' ||
    code === 'connector.mail.authentication_rejected' ||
    code === 'connector_mail_app_password_required' ||
    code === 'connector.mail.app_password_required'
  ) {
    return 'mail.authenticationRejected';
  }
  if (code === 'connector_mail_tls_required' || code === 'connector.mail.tls_required') {
    return 'mail.tlsFailed';
  }
  return 'mail.failed';
}

/** Whitelist this response projection; never retain the Axios error or raw provider response. */
export function mailFailureVerification(error: unknown): ConnectorMailVerification | undefined {
  const candidate = (error as { response?: { data?: { mail_verification?: unknown } } } | null)
    ?.response?.data?.mail_verification;
  if (!candidate || typeof candidate !== 'object') return;
  const values = candidate as Record<string, unknown>;
  const statuses = ['not_requested', 'verified', 'failed'];
  if (
    (values.usage !== 'read' && values.usage !== 'send' && values.usage !== 'read_send') ||
    !statuses.includes(String(values.imap_status)) ||
    !statuses.includes(String(values.smtp_status))
  ) {
    return;
  }
  return {
    usage: values.usage,
    imap_status: values.imap_status as ConnectorMailVerification['imap_status'],
    smtp_status: values.smtp_status as ConnectorMailVerification['smtp_status'],
  };
}

export function normalizeMailProfileEmail(provider: string, value: string): string | undefined {
  const email = normalizeMailEmail(value);
  if (provider === 'gmail') return email;
  const domain =
    provider === 'qqmail' ? 'qq.com' : provider === 'netease163' ? '163.com' : undefined;
  return domain && email?.endsWith(`@${domain}`) ? email : undefined;
}
export function normalizeMailSecret(provider: string, value: string): string | undefined {
  if (provider === 'gmail') return normalizeMailAppPassword(value);
  if (value.length > 256) return;
  // NetEase may render the 16-letter code in groups when it is copied from
  // the settings page. Treat those whitespace separators as presentation
  // only; the IMAP/SMTP credential itself never contains them.
  const secret = provider === 'netease163' ? value.replace(/[\s\uFEFF]/gu, '') : value.trim();
  if (provider === 'qqmail') return /^[\x21-\x7e]{16}$/.test(secret) ? secret : undefined;
  if (provider === 'netease163') return /^[A-Za-z]{16}$/.test(secret) ? secret : undefined;
}
export function mailSettingsURL(provider: string): string {
  if (provider === 'qqmail') return 'https://help.mail.qq.com/detail/106/985';
  if (provider === 'netease163') {
    return 'https://help.mail.126.com/faqDetail.do?code=d7a5dc8471cd0c0e8b4b8f4f8e49998b374173cfe9171305fa1ce630d7f67ac286624f309a1a7089';
  }
  return 'https://myaccount.google.com/apppasswords';
}
