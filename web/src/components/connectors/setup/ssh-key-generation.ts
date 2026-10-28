/**
 * A platform-generated SSH key pair for a server connection (ADR 0030).
 *
 * The browser generates an Ed25519 pair with WebCrypto. The private key is
 * never shown: it travels once, like a pasted key, to the connection setup,
 * which stores it encrypted. The person only installs the public key on the
 * server. This module stays free of runtime imports so a Node script can test
 * it.
 */

export interface GeneratedSSHKeyPair {
  /** PKCS#8 PEM, accepted by the server as an unencrypted private key. */
  privateKeyPem: string;
  /** One authorized_keys line: ssh-ed25519 <base64> <comment>. */
  publicKey: string;
}

/** Whether this browser can generate Ed25519 keys. */
export function canGenerateSSHKey(): boolean {
  return typeof globalThis.crypto?.subtle?.generateKey === 'function';
}

function base64(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function sshString(bytes: Uint8Array): Uint8Array {
  const out = new Uint8Array(4 + bytes.length);
  new DataView(out.buffer).setUint32(0, bytes.length);
  out.set(bytes, 4);
  return out;
}

/** The OpenSSH public key line for a raw 32-byte Ed25519 public key. */
export function openSSHEd25519PublicKey(raw: Uint8Array, comment: string): string {
  const type = new TextEncoder().encode('ssh-ed25519');
  const typePart = sshString(type);
  const keyPart = sshString(raw);
  const blob = new Uint8Array(typePart.length + keyPart.length);
  blob.set(typePart, 0);
  blob.set(keyPart, typePart.length);
  const safeComment = comment.replace(/[^\w.@-]/g, '');
  return `ssh-ed25519 ${base64(blob)}${safeComment ? ` ${safeComment}` : ''}`;
}

/** Wrap DER bytes as PEM with 64-character lines. */
export function pem(label: string, der: Uint8Array): string {
  const body = base64(der)
    .replace(/(.{64})/g, '$1\n')
    .trimEnd();
  return `-----BEGIN ${label}-----\n${body}\n-----END ${label}-----\n`;
}

/** Generate a new pair; rejects when the browser lacks Ed25519. */
export async function generateSSHKeyPair(comment = 'zgi-ops'): Promise<GeneratedSSHKeyPair> {
  const pair = (await crypto.subtle.generateKey({ name: 'Ed25519' }, true, [
    'sign',
    'verify',
  ])) as CryptoKeyPair;
  const pkcs8 = new Uint8Array(await crypto.subtle.exportKey('pkcs8', pair.privateKey));
  const raw = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
  return {
    privateKeyPem: pem('PRIVATE KEY', pkcs8),
    publicKey: openSSHEd25519PublicKey(raw, comment),
  };
}

/** One command that installs the public key for the account that runs it. */
export function authorizedKeysCommand(publicKey: string): string {
  const line = publicKey.replace(/'/g, '');
  return `mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${line}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`;
}
