import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

import {
  authorizedKeysCommand,
  generateSSHKeyPair,
  openSSHEd25519PublicKey,
} from '../src/components/connectors/setup/ssh-key-generation.ts';

const pair = await generateSSHKeyPair('zgi-ops@test');
assert.match(
  pair.privateKeyPem,
  /^-----BEGIN PRIVATE KEY-----\n[\s\S]+\n-----END PRIVATE KEY-----\n$/
);
assert.match(pair.publicKey, /^ssh-ed25519 [A-Za-z0-9+/=]+ zgi-ops@test$/);

// The private key must be the one the public key names: Node derives the
// public key from the PEM and the OpenSSH line must match it.
const { createPrivateKey, createPublicKey } = await import('node:crypto');
const derived = createPublicKey(createPrivateKey(pair.privateKeyPem)).export({
  format: 'der',
  type: 'spki',
});
const raw = new Uint8Array(derived.subarray(derived.length - 32));
assert.equal(openSSHEd25519PublicKey(raw, 'zgi-ops@test'), pair.publicKey);

// ssh-keygen, when present, accepts the public key line.
try {
  const dir = mkdtempSync(path.join(tmpdir(), 'zgi-key-'));
  writeFileSync(path.join(dir, 'key.pub'), `${pair.publicKey}\n`);
  const fingerprint = execFileSync('ssh-keygen', ['-lf', path.join(dir, 'key.pub')]).toString();
  assert.match(fingerprint, /ED25519/);
  rmSync(dir, { recursive: true, force: true });
} catch (error) {
  if (error?.code !== 'ENOENT') throw error;
}

const command = authorizedKeysCommand(`${pair.publicKey}'; rm -rf /`);
assert.ok(!command.includes("'; rm"), 'a quote in the key cannot break out of the command');
assert.match(command, /^mkdir -p ~\/\.ssh && chmod 700 ~\/\.ssh && echo 'ssh-ed25519 /);

// The setup panel offers the generated key first for new connections and
// sends it as an ordinary private key.
const panel = readFileSync('src/components/connectors/setup/serverops-ssh-setup-panel.tsx', 'utf8');
assert.match(panel, /!updateConnection && canGenerateSSHKey\(\) \? 'generated' : 'password'/);
assert.match(panel, /\[SERVEROPS_SSH_PRIVATE_KEY_FIELD\]: pair\.privateKeyPem/);
assert.ok(!/privateKeyPem\}/.test(panel), 'the private key is never rendered');

console.log('SSH key generation checks passed');
