# Chat process notes and execution events

## Process notes

Safe, task-level assistant notes accompanying registered business tools or runtime
tools appear in chronological order with their events. Internal identifiers and
notes exceeding the existing per-note or per-turn limits remain filtered.

These notes are page-local presentation state, not durable message history.
Completion, silent history updates, selecting a conversation, and starting another
message retain the already displayed notes. Replacing the same message clears its
old projection. A full page reload or controller teardown removes the local state;
history still contains the final answer and durable execution events.

## Linux execution events

Shell cards distinguish running, successful, failed, canceled, timed-out and
unconfirmed jobs. Tool transport completion alone does not imply a successful
process exit. Stderr alone does not imply failure either.

Runtime tool duration measures the handler, including environment preparation and
waiting for execution. Legacy zero or unknown durations are hidden rather than
reported as sub-millisecond execution.

The default card uses localized action and status labels. Exit codes and internal
identifiers are available in collapsed technical details. File export events use
download-oriented labels; actual files continue to use the existing download cards.

Incremental console acceptance with the current Web build retains all ten events
in an E2B CLI file/tool sample after selecting another completed conversation and
returning. Its task-level process notes remain expandable alongside friendly
Shell status/duration cards. A separate failed Python command shows the failed
state and collapsed technical details. This verifies page-local retention, not
retention across a full page reload.

The durable `execution_display` result contains only whitelisted runtime and
diagnostic classifications and output-presence flags. Python/Node missing-module
and missing-command diagnostics get safe explanatory summaries. Raw commands,
package names, stdout and stderr are not copied into the event trace. They remain
available to the agent through its existing model result. Unknown errors use a
generic summary rather than exposing diagnostic bodies or guessing a cause.
