/**
 * Pure, side-effect-free helpers for the Server Operations SSH Connector setup
 * surface. The dedicated panel and its regression script both consume this
 * module, so it deliberately has no imports and no module-scope browser state.
 *
 * Every validator mirrors the frozen catalog contract in
 * api/internal/modules/connectors/catalog/serverops_ssh.go. The backend remains
 * the authority: these helpers only keep an obviously malformed value from
 * being submitted, and a rejected value is never normalized into validity.
 */

export const SERVEROPS_SSH_CONNECTOR_ID = 'serverops_ssh';
export const SERVEROPS_SSH_SETUP_METHOD = 'ssh_credentials';

/** Connector Center owns standalone connection setup; embedded hosts may opt in explicitly. */
export const CONNECTOR_CENTER_ROUTE_ROOT = '/console/connectors';

export const SERVEROPS_SSH_HOST_FIELD = 'host';
export const SERVEROPS_SSH_PORT_FIELD = 'port';
export const SERVEROPS_SSH_USERNAME_FIELD = 'username';
export const SERVEROPS_SSH_PRIVATE_KEY_FIELD = 'private_key';
export const SERVEROPS_SSH_PASSWORD_FIELD = 'password';
export const SERVEROPS_SSH_PASSPHRASE_FIELD = 'passphrase';
export const SERVEROPS_SSH_HOST_KEY_FIELD = 'host_key_sha256';

export const SERVEROPS_SSH_DEFAULT_PORT = '22';

// Bounds mirror the catalog definition; private_key is 16 KiB here, not 1 MiB.
const HOST_MAX_BYTES = 255;
const PORT_MAX_BYTES = 5;
const USERNAME_MAX_BYTES = 128;
const PRIVATE_KEY_MAX_BYTES = 16 * 1024;
const PASSPHRASE_MAX_BYTES = 4096;
const FINGERPRINT_MAX_BYTES = 128;
const MIN_PORT = 1;
const MAX_PORT = 65535;
const FINGERPRINT_PREFIX = 'SHA256:';
const SHA256_BYTES = 32;
const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

/** The minimal catalog identity this module needs; a full catalog item satisfies it. */
export interface ServerOpsSSHConnectorIdentity {
  id: string;
  setup_method?: string | null;
}

export type ServerOpsSSHFieldError =
  | 'host'
  | 'port'
  | 'username'
  | 'password'
  | 'private_key'
  | 'passphrase'
  | 'host_key_sha256';

export type ServerOpsSSHAuthMethod = 'password' | 'private_key';

export function createServerOpsSSHFormValues(): Record<string, string> {
  return {
    [SERVEROPS_SSH_HOST_FIELD]: '',
    [SERVEROPS_SSH_PORT_FIELD]: SERVEROPS_SSH_DEFAULT_PORT,
    [SERVEROPS_SSH_USERNAME_FIELD]: '',
    [SERVEROPS_SSH_PASSWORD_FIELD]: '',
    [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: '',
    [SERVEROPS_SSH_PASSPHRASE_FIELD]: '',
    [SERVEROPS_SSH_HOST_KEY_FIELD]: '',
  };
}

/** Return a copy with every write-only secret blanked. Non-secret fields survive retry. */
export function clearServerOpsSSHSecretValues(
  values: Record<string, string>
): Record<string, string> {
  return {
    ...values,
    [SERVEROPS_SSH_PASSWORD_FIELD]: '',
    [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: '',
    [SERVEROPS_SSH_PASSPHRASE_FIELD]: '',
  };
}

/** Clear secret fields in place, for unmount cleanup where no re-render happens. */
export function destroyServerOpsSSHSecretValues(values: Record<string, string>): void {
  values[SERVEROPS_SSH_PASSWORD_FIELD] = '';
  values[SERVEROPS_SSH_PRIVATE_KEY_FIELD] = '';
  values[SERVEROPS_SSH_PASSPHRASE_FIELD] = '';
}

/** Blank the temporary request object and drop every key after the request settles. */
export function destroyServerOpsSSHCredentialPayload<T extends object>(payload: T): void {
  const values = payload as Record<string, string | undefined>;
  for (const name of Object.keys(values)) {
    values[name] = '';
    delete values[name];
  }
}

export function isServerOpsSSHConnector(connector: ServerOpsSSHConnectorIdentity): boolean {
  return (
    connector.id === SERVEROPS_SSH_CONNECTOR_ID &&
    connector.setup_method === SERVEROPS_SSH_SETUP_METHOD
  );
}

/**
 * Connector Center is the default standalone surface. Embedded hosts such as
 * Agent and Digital Human can explicitly opt into the same dedicated panel so
 * credential creation stays in the current task instead of forcing a route
 * change.
 */
export function isConnectorCenterRoute(pathname?: string | null): boolean {
  if (!pathname) return false;
  const path = pathname.split(/[?#]/, 1)[0];
  const normalized = path.length > 1 && path.endsWith('/') ? path.slice(0, -1) : path;
  return (
    normalized === CONNECTOR_CENTER_ROUTE_ROOT ||
    normalized.startsWith(`${CONNECTOR_CENTER_ROUTE_ROOT}/`)
  );
}

export function supportsServerOpsSSHSetup(
  connector: ServerOpsSSHConnectorIdentity,
  pathname?: string | null,
  embedded = false
): boolean {
  return isServerOpsSSHConnector(connector) && (embedded || isConnectorCenterRoute(pathname));
}

/**
 * The connector catalog owns setup dispatch. Any host that reaches the shared
 * ConnectorKeySetupPanel may use the SSH panel; route checks belong only to
 * legacy deep-link guidance, not to the credential capability itself.
 */
export function supportsServerOpsSSHConnectorSetup(
  connector: ServerOpsSSHConnectorIdentity
): boolean {
  return isServerOpsSSHConnector(connector);
}

export function isServerOpsSSHHostValid(value: string): boolean {
  return (
    value !== '' &&
    value === value.trim() &&
    utf8ByteLength(value) <= HOST_MAX_BYTES &&
    !hasASCIIControlOrSpace(value)
  );
}

export function isServerOpsSSHUsernameValid(value: string): boolean {
  return (
    value !== '' &&
    value === value.trim() &&
    utf8ByteLength(value) <= USERNAME_MAX_BYTES &&
    !hasASCIIControl(value)
  );
}

/** Canonical decimal only: no signs, whitespace, leading zeros, or out-of-range values. */
export function isServerOpsSSHPortValid(value: string): boolean {
  if (value === '' || value.length > PORT_MAX_BYTES || !/^[0-9]+$/.test(value)) return false;
  const port = Number(value);
  if (!Number.isInteger(port) || port < MIN_PORT || port > MAX_PORT) return false;
  return String(port) === value;
}

/** PEM/OpenSSH key text: non-blank, bounded, and free of control bytes other than CR/LF. */
export function isServerOpsSSHPrivateKeyValid(value: string): boolean {
  if (value === '' || utf8ByteLength(value) > PRIVATE_KEY_MAX_BYTES || value.trim() === '') {
    return false;
  }
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code === 0x0a || code === 0x0d) continue;
    if (isForbiddenControl(code)) return false;
  }
  return true;
}

/** A supplied passphrase must be non-blank and single-line; omission is the unencrypted form. */
export function isServerOpsSSHPassphraseValid(value: string): boolean {
  return (
    value.trim() !== '' && utf8ByteLength(value) <= PASSPHRASE_MAX_BYTES && !hasASCIIControl(value)
  );
}

/**
 * Only the canonical OpenSSH form is admitted: the "SHA256:" prefix followed by
 * the unpadded standard base64 of exactly 32 bytes. The form is optional during
 * the initial probe, but any supplied value is strict and canonical.
 */
export function isServerOpsSSHHostKeyFingerprintValid(value: string): boolean {
  if (value === '' || utf8ByteLength(value) > FINGERPRINT_MAX_BYTES) return false;
  if (!value.startsWith(FINGERPRINT_PREFIX)) return false;
  if (value !== value.trim() || hasASCIIControlOrSpace(value)) return false;
  const encoded = value.slice(FINGERPRINT_PREFIX.length);
  const digest = decodeCanonicalBase64(encoded);
  if (!digest || digest.length !== SHA256_BYTES) return false;
  return FINGERPRINT_PREFIX + encodeBase64Unpadded(digest) === value;
}

/** The first invalid field, in form order. A blank optional passphrase is valid. */
export function serverOpsSSHFieldError(
  values: Record<string, string>,
  authMethod: ServerOpsSSHAuthMethod = 'private_key'
): ServerOpsSSHFieldError | undefined {
  if (!isServerOpsSSHHostValid(values[SERVEROPS_SSH_HOST_FIELD] ?? '')) return 'host';
  if (!isServerOpsSSHPortValid(values[SERVEROPS_SSH_PORT_FIELD] ?? '')) return 'port';
  if (!isServerOpsSSHUsernameValid(values[SERVEROPS_SSH_USERNAME_FIELD] ?? '')) return 'username';
  if (authMethod === 'password') {
    if (!isServerOpsSSHPasswordValid(values[SERVEROPS_SSH_PASSWORD_FIELD] ?? '')) {
      return 'password';
    }
  } else {
    if (!isServerOpsSSHPrivateKeyValid(values[SERVEROPS_SSH_PRIVATE_KEY_FIELD] ?? '')) {
      return 'private_key';
    }
    const passphrase = values[SERVEROPS_SSH_PASSPHRASE_FIELD];
    if (passphrase && passphrase.trim() !== '' && !isServerOpsSSHPassphraseValid(passphrase)) {
      return 'passphrase';
    }
  }
  if (!isServerOpsSSHHostKeyFingerprintValid(values[SERVEROPS_SSH_HOST_KEY_FIELD] ?? '')) {
    return 'host_key_sha256';
  }
  return undefined;
}

export function isServerOpsSSHCredentialFormValid(
  values: Record<string, string>,
  authMethod: ServerOpsSSHAuthMethod = 'private_key'
): boolean {
  return serverOpsSSHFieldError(values, authMethod) === undefined;
}

/**
 * Build the exact request value map. A blank or whitespace-only optional
 * passphrase is omitted entirely, because the backend rejects a supplied blank
 * value; submitted values are never trimmed or otherwise normalized.
 */
export function buildServerOpsSSHCredentialPayload(
  values: Record<string, string>,
  authMethod: ServerOpsSSHAuthMethod = 'private_key'
): Record<string, string> {
  const payload: Record<string, string> = {
    [SERVEROPS_SSH_HOST_FIELD]: values[SERVEROPS_SSH_HOST_FIELD] ?? '',
    [SERVEROPS_SSH_PORT_FIELD]: values[SERVEROPS_SSH_PORT_FIELD] ?? '',
    [SERVEROPS_SSH_USERNAME_FIELD]: values[SERVEROPS_SSH_USERNAME_FIELD] ?? '',
    [SERVEROPS_SSH_HOST_KEY_FIELD]: values[SERVEROPS_SSH_HOST_KEY_FIELD] ?? '',
  };
  if (authMethod === 'password') {
    payload[SERVEROPS_SSH_PASSWORD_FIELD] = values[SERVEROPS_SSH_PASSWORD_FIELD] ?? '';
  } else {
    payload[SERVEROPS_SSH_PRIVATE_KEY_FIELD] = values[SERVEROPS_SSH_PRIVATE_KEY_FIELD] ?? '';
    const passphrase = values[SERVEROPS_SSH_PASSPHRASE_FIELD];
    if (passphrase && passphrase.trim() !== '') {
      payload[SERVEROPS_SSH_PASSPHRASE_FIELD] = passphrase;
    }
  }
  return payload;
}

/** Non-secret result of a host-key probe; never carries credential material. */
export interface ServerOpsSSHHostKeyObservation {
  host_key_sha256: string;
  hostname: string;
  request_id: string;
}

/** Exact write-only request values accepted by the read-only host-key probe. */
export interface ServerOpsSSHProbePayload {
  host: string;
  port: string;
  username: string;
  private_key?: string;
  password?: string;
  passphrase?: string;
  host_key_sha256?: string;
}

/**
 * The probe mirrors the final credential validation for host, port, username,
 * private key, and passphrase, but the pinned fingerprint is optional: the
 * backend captures the host key when it is omitted. A supplied fingerprint is
 * still held to the same canonical form as the final strict save.
 */
export function serverOpsSSHProbeFieldError(
  values: Record<string, string>,
  authMethod: ServerOpsSSHAuthMethod = 'private_key'
): ServerOpsSSHFieldError | undefined {
  if (!isServerOpsSSHHostValid(values[SERVEROPS_SSH_HOST_FIELD] ?? '')) return 'host';
  if (!isServerOpsSSHPortValid(values[SERVEROPS_SSH_PORT_FIELD] ?? '')) return 'port';
  if (!isServerOpsSSHUsernameValid(values[SERVEROPS_SSH_USERNAME_FIELD] ?? '')) return 'username';
  if (authMethod === 'password') {
    if (!isServerOpsSSHPasswordValid(values[SERVEROPS_SSH_PASSWORD_FIELD] ?? '')) {
      return 'password';
    }
  } else {
    if (!isServerOpsSSHPrivateKeyValid(values[SERVEROPS_SSH_PRIVATE_KEY_FIELD] ?? '')) {
      return 'private_key';
    }
    const passphrase = values[SERVEROPS_SSH_PASSPHRASE_FIELD];
    if (passphrase && passphrase.trim() !== '' && !isServerOpsSSHPassphraseValid(passphrase)) {
      return 'passphrase';
    }
  }
  const fingerprint = values[SERVEROPS_SSH_HOST_KEY_FIELD] ?? '';
  if (fingerprint !== '' && !isServerOpsSSHHostKeyFingerprintValid(fingerprint)) {
    return 'host_key_sha256';
  }
  return undefined;
}

/**
 * Build the probe request value map. The optional passphrase and the optional
 * pinned fingerprint are omitted when blank, so the backend captures the host
 * key instead of receiving an empty value; supplied values are never trimmed.
 */
export function buildServerOpsSSHProbePayload(
  values: Record<string, string>,
  authMethod: ServerOpsSSHAuthMethod = 'private_key'
): ServerOpsSSHProbePayload {
  const payload: ServerOpsSSHProbePayload = {
    [SERVEROPS_SSH_HOST_FIELD]: values[SERVEROPS_SSH_HOST_FIELD] ?? '',
    [SERVEROPS_SSH_PORT_FIELD]: values[SERVEROPS_SSH_PORT_FIELD] ?? '',
    [SERVEROPS_SSH_USERNAME_FIELD]: values[SERVEROPS_SSH_USERNAME_FIELD] ?? '',
  };
  if (authMethod === 'password') {
    payload[SERVEROPS_SSH_PASSWORD_FIELD] = values[SERVEROPS_SSH_PASSWORD_FIELD] ?? '';
  } else {
    payload[SERVEROPS_SSH_PRIVATE_KEY_FIELD] = values[SERVEROPS_SSH_PRIVATE_KEY_FIELD] ?? '';
    const passphrase = values[SERVEROPS_SSH_PASSPHRASE_FIELD];
    if (passphrase && passphrase.trim() !== '') {
      payload[SERVEROPS_SSH_PASSPHRASE_FIELD] = passphrase;
    }
  }
  const fingerprint = values[SERVEROPS_SSH_HOST_KEY_FIELD] ?? '';
  if (fingerprint !== '') {
    payload[SERVEROPS_SSH_HOST_KEY_FIELD] = fingerprint;
  }
  return payload;
}

/** Non-secret host/user/port identity a probe observation belongs to. */
export function serverOpsSSHObservationTargetSignature(values: Record<string, string>): string {
  return [
    values[SERVEROPS_SSH_HOST_FIELD] ?? '',
    values[SERVEROPS_SSH_PORT_FIELD] ?? '',
    values[SERVEROPS_SSH_USERNAME_FIELD] ?? '',
  ].join('\u0000');
}

/**
 * Full non-secret signature used to invalidate a stale confirmation. It holds
 * no credential material, so it is safe to keep alongside the observation.
 */
export function serverOpsSSHObservationSignature(
  values: Record<string, string>,
  authMethod: ServerOpsSSHAuthMethod = 'private_key'
): string {
  return [
    serverOpsSSHObservationTargetSignature(values),
    authMethod,
    values[SERVEROPS_SSH_HOST_KEY_FIELD] ?? '',
  ].join('\u0000');
}

const PASSWORD_MAX_BYTES = 4096;

export function isServerOpsSSHPasswordValid(value: string): boolean {
  return (
    value.trim() !== '' && utf8ByteLength(value) <= PASSWORD_MAX_BYTES && !hasASCIIControl(value)
  );
}

function isForbiddenControl(code: number): boolean {
  return code < 0x20 || code === 0x7f;
}

function hasASCIIControl(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    if (isForbiddenControl(value.charCodeAt(index))) return true;
  }
  return false;
}

function hasASCIIControlOrSpace(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (isForbiddenControl(code) || code === 0x20) return true;
  }
  return false;
}

function utf8ByteLength(value: string): number {
  let bytes = 0;
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code < 0x80) {
      bytes += 1;
    } else if (code < 0x800) {
      bytes += 2;
    } else if (
      code >= 0xd800 &&
      code <= 0xdbff &&
      index + 1 < value.length &&
      value.charCodeAt(index + 1) >= 0xdc00 &&
      value.charCodeAt(index + 1) <= 0xdfff
    ) {
      bytes += 4;
      index += 1;
    } else {
      bytes += 3;
    }
  }
  return bytes;
}

/** Reject malformed or non-canonical unpadded standard base64. */
function decodeCanonicalBase64(value: string): Uint8Array | undefined {
  if (value.length % 4 === 1) return undefined;
  const output = new Uint8Array(Math.floor((value.length * 3) / 4));
  let buffer = 0;
  let bits = 0;
  let offset = 0;
  for (let index = 0; index < value.length; index += 1) {
    const sextet = BASE64_ALPHABET.indexOf(value[index]);
    if (sextet < 0) return undefined;
    buffer = (buffer << 6) | sextet;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      output[offset] = (buffer >> bits) & 0xff;
      offset += 1;
    }
  }
  // Unused trailing bits are dropped by the encoder; the caller re-encodes and
  // compares, so a non-zero pad is rejected as non-canonical.
  return output.slice(0, offset);
}

function encodeBase64Unpadded(bytes: Uint8Array): string {
  let encoded = '';
  for (let index = 0; index < bytes.length; index += 3) {
    const remaining = bytes.length - index;
    const chunk =
      (bytes[index] << 16) |
      ((remaining > 1 ? bytes[index + 1] : 0) << 8) |
      (remaining > 2 ? bytes[index + 2] : 0);
    encoded += BASE64_ALPHABET[(chunk >> 18) & 0x3f];
    encoded += BASE64_ALPHABET[(chunk >> 12) & 0x3f];
    if (remaining > 1) encoded += BASE64_ALPHABET[(chunk >> 6) & 0x3f];
    if (remaining > 2) encoded += BASE64_ALPHABET[chunk & 0x3f];
  }
  return encoded;
}
