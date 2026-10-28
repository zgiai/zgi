# ADR 0031: A Server Watch Starts a Task When a Service Goes Down

- Status: Accepted
- Date: 2026-10-28
- Builds on: ADR 0027, ADR 0028, ADR 0029

## Context

The operations specialist can inspect a server every morning (ADR 0027). An outage at 10 a.m., though, waits until the next morning or until somebody notices.

Asking a model every few minutes whether the server is fine would be slow and costly, and almost every answer would be "yes". What people expect is closer to an alert rule: a cheap check that runs all the time and calls the specialist only when something changed.

## Decision

1. A DigitalHuman task can carry one **server watch**. A watch has:
   - one of the DigitalHuman's SSH connections;
   - 1 to 10 systemd services;
   - a check interval from 1 to 60 minutes (default 5);
   - a failure threshold from 1 to 5 consecutive failed checks (default 2).

   A person who may change the task sets it up, and only the owner of the connection can point a watch at it, as for task grants.
2. The platform runs the check without a model. It reads each service's state with the fixed, read-only service status action (`systemctl show`), over the connection's pinned host key. The check is the platform's own probe, not a call by the DigitalHuman:
   - it consumes no task grant and needs no approval;
   - it cannot run anything else.
3. A service is down when it is not `active`. The host is down when the check cannot reach or log in to it; services are then not judged.

   After the threshold of consecutive failures the watch raises **one** alert per outage, and it does not raise another until that service or host has been seen healthy again. Alerts that come within 10 minutes of the last run wait for the next check.
4. An alert starts a run of the watch's task, with the trigger `server_event`. The run's instruction names what went down, its state, and when. Everything else is the task's usual run:
   - unattended authorization: grants, `trusted_unattended`, organization rules;
   - the report, inbox, notify channels and delivery.

   Without unattended trust the specialist therefore investigates and reports, and lists any recovery for a person to approve. With `trusted_unattended` it may restart the service itself, as the task's instructions say.
5. A watch is skipped, and says why, when:
   - its task is paused, cancelled or archived (a task that finished its own schedule still answers its watch);
   - the task already has a queued or running run;
   - the connection owner's account is no longer active, a member of the workspace, or enabled in the organization (checked before any credential is read);
   - the DigitalHuman no longer has an active binding on the connection;
   - the connection is not ready.

   A skipped alert is raised again at the next check.
6. After a `server_event` run the task returns to waiting, whatever its schedule, so the next outage can start it again.

## Consequences

- An outage reaches the specialist within the interval times the threshold, about 10 minutes by default. No model runs while everything is healthy.
- Each check opens one SSH session per watched service. A watch with ten services checked every minute is the most a task can cost a server.
- The watch reads only service states. It does not watch logs, disk or HTTP endpoints; those can be added later as other check kinds behind the same trigger.
