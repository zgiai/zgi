# ADR 0023: An Operations Specialist Ready for Production

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0007 (per-user connector credentials), ADR 0022 (one template catalog)
- Amends: the SSH Server Operations connector (`serverops_ssh`), which was read-only

## Context

The operations specialist template could only inspect a server, and starting one did not lead anywhere useful:

- **Inspection only.** The SSH connector had three fixed actions: a system snapshot, a service's status, and a service's journal tail. A specialist could find that a service was down but not bring it back.
- **No skill, no memory.** The template bound no skill and set no memory instructions, so how to work was left to the persona alone.
- **Published before it could work.** Creating from the template published the DigitalHuman whether or not a server was connected.
- **Hard-coded requirement.** The template's need for a server was a single flag, and nothing after creation asked anyone to meet it.

The approval for every SSH call already exists:

- A DigitalHuman's connector call is held for a fresh interactive confirmation (the connector confirmation event).
- Without an interactive person the call is refused, so scheduled and background runs never reach a server.
- That event is the connector's own, separate from workflow approval forms.

## Decision

### 1. Controlled write actions on the SSH connector

- **Four actions are added**, each for one systemd service unit named by the same strict pattern as the read actions:
  - `server.service.restart`
  - `server.service.start`
  - `server.service.stop`
  - `server.service.reload`
- **No arbitrary command, path, or argument exists.** The command is fixed per action:
  - `systemctl <verb> -- '<unit>'`, optionally behind `sudo -n` (see below);
  - followed by a fresh read of the unit's state, which is returned with the result;
  - a failed command returns its bounded error text.
- **Classification:**
  - Every write action has the catalog effect `update`, is not idempotent, and is high risk.
  - It inherits the existing rule: a fresh per-call interactive confirmation, and no background or scheduled use.
- **The approver sees the exact command.**
  - The confirmation preview of every SSH action, read or write, gains a `Command` field holding the command line that will run (for example `sudo -n systemctl restart -- 'nginx.service'`). It is rendered as code, next to the target account and host.
  - The preview is built from the same code that builds the command, so it cannot differ from what runs.
  - It is shown in the connector confirmation card of the conversation and of run details. Workflow approval forms are not involved.
- **Privilege is part of the connection, chosen when it is set up.**
  - A new optional field `privilege` is either empty (run as the SSH user) or `sudo`. With `sudo`, commands run as `sudo -n`, so a password prompt fails instead of hanging.
  - Read actions keep running unprivileged.
- **Write actions are opt-in.**
  - A binding's allowed operations stay the operator's choice, as before.
  - When a connection is added during creation or binding, only read actions are selected by default; write actions must be ticked.
- The connector's plugin release moves from 1.0.0 to 1.1.0, so existing installations take the new operations through the ordinary startup update. The 1.0.0 contract stays valid and pinned to its digest. Adding `privilege` is an exception reviewed against both digests, as earlier credential-field additions were.
- Existing connections allow only the read actions. To bind the write actions or choose `sudo`, the owner reconnects.
- A failed write's error text reaches operators' logs only. The model and the user see that it failed, then read the status and journal to learn why.

### 2. Templates declare requirements, and a DigitalHuman keeps them

- **A template's connector need becomes a list.** Each entry names the connector, the operations it needs, and whether it is required. This replaces the `requires_connector` flag.
- **The built-in operations specialist:**
  - requires an `serverops_ssh` connection with the read actions;
  - offers the write actions;
  - requires the new `ops-runbook` skill.
- **A DigitalHuman created from a template keeps the template's requirements** as its setup requirements (`digital_humans.setup_requirements`), so they outlive any later change to the template.
- **Requirements are met by the draft:**
  - a required skill is bound;
  - a required connector need has a bound connection of that connector whose allowed operations include the needed ones.
- **Publishing is refused while a required item is unmet**, with an error that names what is missing.
  - The check is in the API, so no client can skip it.
  - DigitalHumans without setup requirements publish as before.

### 3. Creation leads to a setup checklist

- **Creation stops at a draft when anything required is missing.** It publishes right away only when the creation form already met every requirement.
- **The configuration page shows a setup checklist** while requirements are unmet. Each item:
  - says what is needed and why;
  - opens the place to do it, such as adding the server connection with the existing SSH setup panel, or binding the skill;
  - turns done as soon as the draft meets it.

  Publishing becomes possible when the list is complete.

### 4. The operations specialist's working method

- **The `ops-runbook` skill** carries the method. It declares its connector requirement (`connector_requirements`).
  - **Before any change:** confirm the target, inspect first, state the plan.
  - **One change at a time:** each step states the exact command and why, waits for the approval, then reads the state again to verify.
  - **Report:** observations kept apart from inferences.
  - **Out of reach:** what it cannot do, such as deploys, config edits, and anything outside its units, is reported and handed to a person.
- **The template's persona and rules** describe the write actions and their approval. They also make clear that it cannot run unattended or on a schedule.
- **Its memory instructions** keep each server's role, the services that matter, recurring failures, and the fixes that worked.

## Consequences

- An operations specialist can restore a failed service, one approved command at a time, with the exact command visible before it runs.
- A specialist cannot be published without the server connection and skill it needs, and the checklist shows the way there.
- **Still out of reach:**
  - arbitrary commands, file edits, and deploys;
  - unattended or scheduled operations.

  These need their own decision.
- Requirements are snapshotted per DigitalHuman. A template's later change does not move existing DigitalHumans.
