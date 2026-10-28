# ADR 0029: Organization Rules for Server Commands

- Status: Accepted
- Date: 2026-10-28
- Builds on: ADR 0026, ADR 0027, ADR 0028

## Context

Each DigitalHuman's SSH connection chooses how it asks before a call:

- always ask;
- run proven read-only commands without asking;
- fully trusted, optionally including scheduled tasks.

These choices are the connection owner's. An organization still needs limits that hold whatever an owner chose. For example, nothing pushes to Git from a server, or restarting the database always needs a person. Codex calls these rules: command prefixes that are forbidden or always prompt.

## Decision

1. An organization keeps up to 100 server command rules. Each rule has:
   - a command prefix of 1 to 12 words;
   - an action, **forbid** or **always ask**;
   - an optional note.

   Organization owners and administrators add and remove rules on the dashboard's organization settings page. Members can read them.
2. Rules only tighten:
   - **forbid** refuses the call before approval, under every policy, with `connector_command_forbidden`;
   - **always ask** keeps the call high risk under read-only-auto and full trust. Interactive runs therefore ask, and unattended runs are blocked even with a task grant or unattended trust.

   No rule lets a command run that would otherwise ask.
3. A rule matches when:
   - the first word names the program;
   - the remaining words appear in order, other arguments allowed between them;
   - it is checked against every simple command of a shell command.

   Wrappers such as sudo, command, exec, nohup and env are unwrapped. A unit name matches with or without `.service`. The fixed service actions match as the `systemctl` or `journalctl` command they run. A command the grammar cannot parse is matched on its raw words, which may over-match but never under-match.
4. A rule store that cannot be read fails closed: the call does not run.

## Consequences

- Administrators can forbid or gate commands for every DigitalHuman in the organization, whatever their owners chose.
- Rules are prefixes, not a sandbox. A determined command can still reach the same effect another way: a script, a different program, or an alias defined in the same command. The command allowlist, approvals, and the SSH account's own permissions remain the boundary.
