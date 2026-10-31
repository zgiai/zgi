# ADR 0048: Reply Language

- Status: Accepted
- Date: 2026-10-31

## Context

The native loop told the model to write "in the language of the user's latest request". Many users mix languages: they paste English error messages, code or documents into a Chinese question, or ask a short question in English inside a Chinese workspace. Following the latest request then switches the reply language for reasons the user did not intend. Kimi also drifted into Chinese in English conversations after generating a file, about one run in six.

The account already stores an interface language (`accounts.interface_language`), and the console sends the UI language as `Accept-Language`.

## Decision

1. **The reply language follows a rule with three levels.**
   - A language the user asks for, in the conversation or in their saved preferences.
   - Otherwise the user's interface language, when the run knows it. Writing in another language is not a request for it.
   - Otherwise the language the user writes in.
2. **The interface language comes from the conversation's source.**
   - `console` and `preview`: the account's `interface_language`, else the request's `Accept-Language`.
   - `webapp` and `external-api`: the request's `Accept-Language`. The account on these runs is the Agent's owner, not the person served.
   - `channel` and `task`: none. The runtime does not know these users' language, so the model follows their writing.
   - A global middleware records `Accept-Language` on the request context (`apptransport.WithLocale`), so every handler's runs can read it. Supported locales are `en-US` and `zh-Hans`.
3. **The interface language is stated after the user's request.** It is a system reminder appended to the request, sent and stored the same way, so the next turn replays it from the cache. In the system prompt or in the reminders before the request, Kimi still answered an English question in English every time; after the request it answered in Chinese.

## Validation

Two runtime eval scenarios send their own `Accept-Language`:

- `interface_language_mixed_zh`: Chinese interface, a question in English with no language requested; the reply should be Chinese.
- `requested_language_overrides_en`: Chinese interface, a Chinese request that asks for an English answer; the reply should be English.

| Placement | DeepSeek, mixed | Kimi, mixed | Both models, override |
|---|---|---|---|
| Reminder before the request | 3/3 | 0/3 | 6/6 |
| System prompt | 2/3 | 0/3 | 6/6 |
| After the request | 2/3 (the miss is a bare formula) | 2/3 | 6/6 |

With `--send-language`, the full suite on DeepSeek and Kimi has no other failures except one bare-number answer. Kimi's English file scenarios pass 12 of 12 with an interface language, and 11 of 12 without one, as before.

## Consequences

- About 40 tokens per turn when an interface language is known.
- A user with a Chinese interface who asks in English gets a Chinese answer unless they ask for English. One sentence, or a saved preference, changes that.
- External API clients that send no `Accept-Language` keep the old behaviour.
