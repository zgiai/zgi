# ADR 0026: Read-Only SSH Commands Can Run Without Approval

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0023 (an operations specialist ready for production), ADR 0025 (shell commands on SSH Server Operations)
- Amends: ADR 0023's rule that every SSH call needs a fresh approval

## Context

Every SSH operation is High risk, so every call asks the person first.

In a real troubleshooting session the operations specialist asked twelve times before it found a root cause, and every call only read the server. Asking for each read has two costs:

- people stop reading what they approve;
- nothing can inspect a server unattended.

Mature coding agents separate the risk of a command from the decision to ask. Codex, for example:

- runs commands it can prove read-only;
- asks before anything else;
- lets an administrator forbid commands outright.

The proof comes from parsing the command, not from matching text.

## Decision

### 1. Every command is classified

`serverops.ClassifyCommand` parses a command with a deliberately narrow grammar and returns one class:

- **Read-only.** Every simple command is in a read-only allowlist with safe arguments. Examples:
  - `systemctl status/show/cat/is-active/list-units`;
  - `journalctl` without its maintenance flags;
  - `ls`, `cat`, `head`, `tail` without `-f`;
  - `grep`, `find` without `-exec` or `-delete`;
  - `git status/log/diff/show`;
  - `docker ps/logs/inspect`;
  - `ss`, `ps`, `df`, `free`;
  - `curl` sending GET to a local address only.
  
  The only operators are `&&`, `||`, `;`, and `|`. The only redirections are `2>&1`, output to `/dev/null`, and reading a file with `<`.
- **Change.** Anything else, including:
  - `sudo`;
  - `$` expansion, backticks, or subshells;
  - any other redirection;
  - anything the grammar cannot parse.
- **Forbidden.** Commands that destroy the server or cut off access:
  - `rm -r` on `/` or a top-level system directory;
  - `mkfs`;
  - `dd` writing to a device;
  - `shutdown`, `reboot`, `poweroff`, `halt`;
  - `systemctl reboot` or `poweroff`;
  - a fork bomb.

A read-only command that names a sensitive file is a change, because it would hand a secret to the model. Sensitive files include:

- shadow files and sudoers;
- `.env` files;
- private keys;
- `/proc/*/environ`;
- credential files such as `.pgpass`, `.netrc`, cloud credentials, Docker and Kubernetes configs.

Paths are compared after normalizing slashes and dot segments, and shadow files also by name. A shell wildcard proves nothing about which file the remote shell reads, so a wildcard argument is read-only only for programs that report names, sizes, types or checksums (`ls`, `du`, `stat`, `wc`, checksums), and a wildcard in an input redirection always asks.

The allowlist checks arguments, not only programs. For example, `ss -K` (which closes sockets) and `ss -D` ask. `getent` reads only databases without secrets (not `shadow` or `gshadow`). `jq` cannot read the environment. `grep` and `diff` cannot recurse into a directory, because that reads every file under it.

The fixed snapshot, service status, and journal actions are read-only.

### 2. A binding chooses how it asks

A DigitalHuman's SSH binding carries `approval_policy`:

- **`always_ask`** (default, the previous behavior): every call asks.
- **`read_only_auto`**: a read-only call runs without asking. Every other call still asks, as before.

The binding's connector constraint stores the policy as authorization mode `read_only_auto`. At execution the platform lowers a High SSH call to trusted-low only when all of these hold:

- the binding has that mode;
- the target is a DigitalHuman;
- the call classifies read-only.

Nothing else changes. Auto-run calls still record their decision and outcome and appear in the run's process with their command.

A call whose approval card is already showing keeps that card when the policy changes, so the person's decision still settles it.

### 3. Forbidden commands never reach approval

A forbidden command fails input validation. The person is never asked to approve it, and the specialist is told it was refused.

### 4. Unattended runs

Scheduled and background runs still cannot reach a server. Read-only unattended inspection is a later step: it needs the owner's task authorization for the same binding.

## Consequences

- An investigation like the one above asks only for the changes it proposes.
- Read-only means "cannot change the server", not "reveals nothing". The person chooses `read_only_auto` knowing the specialist will read logs and files without asking, sensitive files excepted.
- The allowlist fails closed: a read the grammar cannot prove simply asks. It grows with tests, never with pattern matching on raw text.
