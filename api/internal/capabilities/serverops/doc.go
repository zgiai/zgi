// Package serverops is the reusable, transport-neutral SSH server operations
// capability.
//
// It owns three fixed read-only actions (system snapshot, systemd service
// status, and journal tail), four controlled service write actions (restart,
// start, stop, and reload of one systemd service unit, each a fixed systemctl
// verb followed by a fresh state read), the strict parsers for their bounded
// output protocols, and an immutable SSH transport with mandatory host-key
// verification. It deliberately carries no registry identity, no credential
// storage, no database, HTTP, or container wiring, and no arbitrary command
// surface: no action accepts a command, argument, path, or shell fragment, and
// the only privilege escalation is the fixed "sudo -n" prefix a connection may
// select for the write actions. CommandLine renders the exact command line of
// any action, without side effects, for display to an approver.
//
// The capability does not depend on the skills, integrations, or connector
// modules, and no runtime registers it on its own. contract.go publishes the
// static Skill manifest an adapter may expose (SkillID "server-operations" with
// three read-only and four service write tools); it describes the capability
// without executing it. An adapter that builds a Connection owns the connection
// identity and the credential source, keeps the credential in its own store,
// and must obtain explicit user approval for every single use: this package
// holds no credential and performs no approval of its own.
//
// Every call is request-scoped. The capability never retains a connection, a
// credential, a signer, or an SSH client, and it zeroes every internal copy of
// the private key and passphrase before returning on both the success and the
// error path.
package serverops
