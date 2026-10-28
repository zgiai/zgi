# ADR 0027: Scheduled Runs Can Inspect a Server, Never Change It

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0026 (read-only SSH commands can run without approval)
- Amends: ADR 0023's rule that scheduled and background runs never reach a server

## Context

A recurring inspection (check the services every morning, report what is wrong) is a natural task for the operations specialist. Until now there was no person to approve its calls, so it could not reach the server.

The platform already has owner-issued task grants for unattended Connector calls. A grant is bounded:

- it names one binding, its operations, an expiry, and a number of uses;
- each unattended call consumes one use.

Nothing stopped a grant from covering SSH operations, though. A grant on `server.command.run` would have let an unattended run execute any command. That contradicted ADR 0023.

## Decision

1. An unattended SSH call may use a task grant only when it is proven read-only:
   - the snapshot, service status, and journal actions are read-only;
   - a shell command must classify read-only under ADR 0026.

   A change, or a command that cannot be proven read-only, is blocked before the grant is consulted. It consumes nothing and never reaches the server.
2. A task grant on an SSH connection can cover only the snapshot, service status, journal, and shell command actions. Grants for the service write actions are refused when issued, and the grant form does not offer them.
3. The specialist's runbook and template say so: in a scheduled run it inspects and reports, never attempts a change, and lists any change it recommends for a person to approve.

## Consequences

- An owner can schedule read-only server inspection by granting the connection, bounded by expiry and uses.
- Changes always need a person, whatever was granted.
- Read-only still means "cannot change the server", not "reveals nothing". A granted unattended run reads logs and files, sensitive files excepted.
