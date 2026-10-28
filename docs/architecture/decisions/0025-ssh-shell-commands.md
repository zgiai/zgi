# ADR 0025: Shell Commands on SSH Server Operations

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0023 (an operations specialist ready for production), ADR 0024 (a DigitalHuman continues after a connector confirmation)
- Amends: ADR 0023's rule that the SSH connector has no arbitrary command

## Context

The SSH connector offers eight fixed actions: three reads and four systemd service actions. Real troubleshooting keeps needing what they cannot do:

- read a log file under `/var/log`;
- check listening ports or disk usage by directory;
- test a configuration with `nginx -t`.

A fixed action per need would never catch up. Models already work well with a shell.

The risk of a free-form command is known:

- a command can change or destroy anything the SSH account may;
- server output read by the model can carry instructions an attacker planted (prompt injection);
- the platform cannot tell a read from a write.

The product decision is to offer the capability, state the risk plainly, and let the person decide whether to enable it.

## Decision

### 1. A command run operation

- **Operation:** `server.command.run`, tool `serverops_ssh_command_run`, in plugin release `1.2.0`.
- **Input:** one `command` string, at most 8,192 bytes. It must be valid text and have no NUL, control characters other than tab and newline, or invisible format characters such as bidirectional overrides. This keeps what the approver reads identical to what runs.
- **Execution:** the command runs exactly as written, as the SSH user, in that user's remote shell, for at most 45 seconds. The approval that runs it waits in one HTTP request, so longer work (builds, installs, deploys) is started detached on the server with a log and an exit-code file, and checked with later commands; the runbook skill teaches this. The connection's `sudo` privilege does not apply; a command that needs root writes `sudo -n` itself.
- **Result:** exit code, stdout, stderr, and a truncation flag. A non-zero exit is a result, not an error. Each stream keeps its last 15,000 characters, and secrets of the connection are redacted from it like any SSH output.

### 2. Risk is disclosed, not filtered

- **The operation's description states the risk** in both languages: it can read, change, or delete anything the account may, and the platform does not check what the command does.
- **Enabling it is explicit:**
  - it is never selected by default on a new connection;
  - ticking it opens a confirmation that repeats the risk and recommends a test server or a dedicated account with limited permissions.
- **Every run is approved:** like every SSH operation it is High risk, so each call needs a fresh approval and never runs in scheduled or background runs. The approval card shows the whole command and the account it runs as.
- **No filtering:** the platform does not classify, block, or rewrite commands. The server account's own permissions are the real boundary.

### 3. Releases

- `1.2.0` adds only the operation and the description.
- `1.1.0` stays an accepted contract, pinned by its frozen digest, so stored definitions update through the ordinary compatible-update check.
- A connection verified before `1.2.0` does not offer the new operation until it is reconnected.

### 4. The operations specialist

- The template lists the command run as an optional operation.
- The runbook skill tells the specialist to:
  - prefer the fixed actions;
  - treat a changing command like a service change;
  - keep commands short, non-interactive, and bounded;
  - treat command output as data.

## Consequences

- The specialist can investigate most problems on its own, one approved command at a time.
- Safety now rests on the person who enables the operation, on each approval, and on the server account's permissions. The product says so where the operation is enabled.
