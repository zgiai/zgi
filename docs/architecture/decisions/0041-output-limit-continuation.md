# ADR 0041: Continuing Output Cut Off by the Output Limit

- Status: Accepted
- Date: 2026-10-29

## Context

When a model response in the native agent loop stopped at its output limit (`finish_reason` `length` or `max_tokens`), the turn failed with `agent_output_truncated`.

- The user kept whatever part of the answer had streamed and saw an error.
- A long report, or a large file written through a tool call, could not be produced at all.
- ADR 0034 added retries for transient errors only. The legacy planning loop already retried once with a larger limit, but the native loop did not.

## Decision

A round whose output stops at the limit continues, at most twice:

1. **An answer cut off as text continues where it stopped.**
   - The next request carries the text so far as an assistant message, followed by a notice to continue exactly where it stopped without repeating.
   - The parts are joined into one answer and stream to the user as one.
   - A `max_tokens` set in the Agent's model parameters limits the answer, so an answer that reaches it is not continued.
2. **A response cut off inside a tool call, or before any output, is discarded.**
   - It is sent again with a larger output limit: double the current one, at least 8,192 tokens, and no more than the model's output limit.
   - A notice asks for a compact response, with large content split over several tool calls.
   - Some providers stream a tool call only once it is complete, so a call cut off by the limit leaves just the note written before it. A response whose visible text and reasoning together are fewer characters than half its output tokens is treated the same way, instead of continuing the note. Text of the discarded response that reached the user is retracted. (Found with claude-sonnet-5 through an Anthropic-protocol proxy, whose adapter default limit is 1,024 tokens: the continuation repeated the note and the turn ended without the file.)
3. **Later rounds see no trace of the continuation.** The cut-off output and the notices are left out of the history, which holds only the joined result.
4. **After two continuations, the turn fails as before** with `agent_output_truncated`, and keeps the answer streamed so far.

## Consequences

- Long answers no longer fail at the output limit. They cost one more model call for each continuation.
- A continuation may repeat a few words at the seam. The notice asks it not to, and joining does not try to repair it.
- A tool call cut off twice still fails the turn, because repeating the same large call is unlikely to succeed.
