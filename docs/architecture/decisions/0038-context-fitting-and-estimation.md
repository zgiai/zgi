# ADR 0038: Fitting Requests to the Context Window

- Status: Accepted
- Date: 2026-10-29

## Context

A review of context management found these problems:

1. **The estimate was far off for Chinese.** For every model other than OpenAI's, any string with one non-ASCII character was counted at two bytes per token. That is 1.5 tokens per Chinese character, where DeepSeek and Qwen use about 0.6. The runtime recorded the provider's real prompt tokens, but the context manager never used them. Compaction therefore began at about a third of the real window.
2. **A request could fail outright.**
   - A first message larger than the budget failed at once, either in the service ("query exceeds model context budget") or in the context manager, since there was no history to compact.
   - When compaction failed or could not make a request fit, the turn failed. This happens when switching from a 1M to a 128K model, or when the window is unknown and assumed to be 512K. Every later turn of the conversation then failed the same way.
3. **The prompt budget ignored the answer.** It was 80% of the window and did not subtract the output reserve.
4. **The summary prompt did not fit the product.** It was copied from a coding assistant, asked for files and code snippets, and was in English. It did not ask to keep what is known about the user, identifiers or references. The summary stored for later turns also told every later turn to "pick up the last task".
5. **Compaction was not capped.** Nothing limited how many times one turn could compact.
6. **Two paths broke other decisions.**
   - The DigitalHuman channel history policy (at most four turns, without text the channel context already carries) was never applied.
   - Compressing tool results moved every system message to the front, against ADR 0032 decision 3.

## Decision

1. **Estimation.**
   - The fallback estimator counts each character class at its own rate: ASCII at three bytes per token, CJK characters at 1.5 tokens each, and other characters at two bytes per token.
   - Model families whose tokenizers encode CJK compactly (DeepSeek, Qwen, GLM, Kimi, Doubao and others) count CJK at 0.8.
   - Every rate stays above what common tokenizers produce.
2. **Calibration.**
   - The context manager converts its budget into estimated tokens through a scale: the provider's prompt tokens per estimated token, clamped to 0.25 to 2.
   - The scale starts from the value recorded for the model on the message, or on the turn it answers, if the same estimator recorded it.
   - It is updated from every response.
3. **Room for the answer.** The prompt budget is the smaller of 80% of the window and the window minus the reserved answer.
4. **Fitting instead of failing.** When compaction fails, is capped, or still does not fit, the request is made to fit:
   - whole earlier turns are left out, oldest first, so tool calls keep their results;
   - then the oldest rounds of the current turn, keeping the newest;
   - a notice tells the model that earlier parts were left out;
   - if the current user message alone is still too large, its beginning and end are sent, and the original can be read with `read_context_artifact` (ADR 0037).

   The service leaves an oversized input of the native loop to the context manager instead of refusing it. Only a request whose system prompt and tools alone exceed the window still fails.
5. **Compaction.**
   - At most two semantic compactions run in one run.
   - The summary prompt asks for the user's requests, facts about the user and their preferences, work done with exact identifiers and references, decisions and approvals, errors, open items and the state needed to continue, in the user's language.
   - Only a summary made in the middle of a task asks to continue it.
6. **The two broken paths are fixed.**
   - A supplemental history policy is applied to the history a turn sends.
   - Compressing tool results keeps system messages in place.

## Consequences

- Chinese conversations use most of the window before compacting, so there are fewer and later compaction calls.
- A first message larger than the window runs. The model sees its beginning and end and can read the rest in parts.
- A conversation no longer becomes unusable after a model switch. It loses its oldest turns from the request instead; the transcript keeps them.
- Leaving history out loses information the summary would have kept. It is a fallback, recorded as `drop_history` in the context decision.
