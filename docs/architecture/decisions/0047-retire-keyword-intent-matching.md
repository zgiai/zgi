# ADR 0047: Retiring Keyword Intent Matching

- Status: Accepted
- Date: 2026-10-31

## Context

The runtime used to classify each turn with a model turn-intent classifier, and fell back to keyword lists when the classifier gave no answer. The classifier is gone: nothing fills `chatRequestParts.ModelTurnIntent` from a model. The keyword lists are now the primary path. The branches that read a model intent are unreachable, apart from the one write in `chatRequestPartsWithQueryAndIntent`, and that write is itself triggered by a keyword match.

The remaining matchers read the user's message and change what the run does.

| Matcher | Matches | Drives |
|---|---|---|
| `isContinuationIntent`, `isExplicitContinuationCommand` (turn_continuation_intent.go) | 继续, 接着, continue, go on, next step | history isolation, the continuation task state message, the turn boundary message, the carried-over operation plan, and a `continuation_query_rule` strategy that wins over every other strategy |
| `queryReferencesRecentExecutionContext` (context_budget.go) | 刚才, 之前, 这个文件, save it, that file | whether the recent execution context is injected |
| `splitStagedContinuationQuery` and the staged markers (skill_loop.go) | 等我说继续, when I say continue | cuts the request at the marker, sets `WaitForContinue`, and rewrites the resumed request |
| `nativeInitialActiveSkillIDs` (skill_loop.go) | a skill's ID or name in the request | activates the skill before the first round |
| `latestUserExplicitlyRequestsInlineFileBody` (skillloop/runner.go) | 展示全文, show the full, and negations such as 不要展示全文 | whether an answer may carry a generated file's body |
| `containsTaskResumeIntent` (digitalhuman/task_runtime.go) | 恢复, resume, without a 不 / 不要 / 别 prefix | sets `lifecycle_status=active` on a paused task |
| `hasImageEditIntent` (image/service) and `shouldUseImageContextForPrompt` (img-chat.tsx) | 这张, 刚才, 继续, continue | attaches the latest image as the reference (out of scope, see decision 7) |

Keyword lists miss paraphrases and other languages, misread negation and quotation, and grow with every incident. The model already reads the whole conversation, so it can make these decisions itself. The runtime should state what happened and leave the interpretation to the model.

## Decision

1. **Context is chosen by state, not by wording.**
   - The recent execution context is included whenever the previous turn ran tools or produced files, within its existing budget. The user's words no longer decide this.
   - The open task state is included when the previous turn left work open: it was stopped, or its operation plan has incomplete work. It is phrased as state, and tells the model to ignore it when the latest request is a different task.
   - The turn boundary message is always included, unchanged by continuation.
   - History is never isolated because of a continuation guess. Isolation follows only `HistoryPolicy` and the structured strategy.
2. **The continuation strategy goes.**
   - `continuation_query_rule`, `contextualContinuationStrategy` and the continuation branch of the operation plan are removed. The strategy stays `model_decides`.
   - A prior open operation plan is shown to the model with the continuation state. The model adopts or replaces it through `update_plan`, instead of the runtime merging the old phases.
   - Structured resumes keep working: the resume endpoints (ADR 0042), approvals, connector confirmations and client actions continue from persisted state, not from text.
   - `chatRequestParts.ModelTurnIntent` goes with every branch that read it. The sidebar strategies that only it or the continuation rule reached go too: Agent management, file read, delete and create, navigation, and the file-target parser `explicitRequestedManagedFileTargetsFromQuery`.
3. **Staged instructions are left to the model.**
   - The staged splitter and `WaitForContinue` are removed. The model reads "do A, and do B when I say continue" itself, and records B as an open plan phase. The open phase then reaches the next turn through the continuation state in decision 1.
4. **Skills are not activated by name matching.** The model activates skills through the skill entry (ADR 0044). A skill the user selects in the UI still arrives as structured `SkillIDs`.
5. **File delivery does not read the request.** While a file-generation step is open, intermediate answers stay off, whatever the request says; the final answer may still carry the file's text. No extra guidance is added: both inline-body scenarios pass without it, and a sentence on how to present the file made Kimi answer English requests in Chinese (6 of 9 runs against 1 of 9).
6. **DigitalHuman task resume is a tool argument.** `lifecycle_status` is the only signal. The query fallback for older model snapshots is removed; the argument's description already tells the model to set `active` to resume a paused task.
7. **Image references are out of scope.** `hasImageEditIntent` and `shouldUseImageContextForPrompt` stay for now. The image service has no model in the loop to make this decision, so replacing them needs a product decision on an explicit reference, which is left for later.
8. **The skill restored after navigation comes from the plan's next step** (added 2026-10-31). `preferredRestoredSkillAfterClientAction` read the step's wording (生成, 保存, generate, save) to choose between the file skills. It now takes the skill from the step's `expected_action` or runtime binding, and falls back to the page's usual skills; the model can activate any other.
9. **Exact reply vocabularies stay.** `channelTaskApprovalAction` matches a whole reply against a short list (确认, 好, 取消, no) and treats anything else as a revision. That is a button vocabulary, not intent guessing.

## Validation

The runtime eval gained an `intent` category of nine conversations, run three times per model before and after the change:

- **Staged:** 分三步…每次只做一步，等我说继续, then 继续 twice; and the English "stop after the first one until I tell you to go on", then "finish the rest". The first turn must not contain the later results.
- **Paraphrased continuation:** 刚才没弄完的接着弄.
- **Not a continuation:** 不要继续算了，换个问题, and 继续 inside a quoted sentence.
- **Reference to earlier results:** a follow-up file request with a marker word ("Take that file") and without one (价格都涨 20%).
- **Inline body:** a file request that asks for the full text in the chat, and one that asks for the file only.

| Model | Before | After |
|---|---|---|
| DeepSeek (`deepseek-v4-flash`) | 22 of 27 conversations | 25 of 27 |
| Kimi (`kimi-k2.6`) | 27 of 27 | 25 of 27 |

Every intent check passes after the change on both models. The remaining failures are language checks: a turn answered with a bare number, or Kimi answering an English request in Chinese after generating a file. Kimi does that about one run in six with the old code as well. The twelve earlier scenarios are unchanged on DeepSeek and Kimi. Claude was run once on the first version (it is expensive); it matched the baseline on the intent group.

The console sidebar was checked by hand: a staged request stopped after its first step, 继续 finished it, and "不用继续了，换个问题" answered the new question.

The DigitalHuman resume change is covered by the existing task tests; the tool argument was already the path current models use.

## Consequences

- About 2,200 lines of production code go (the matchers, the model-intent branches, and the sidebar strategies only they reached), with about 2,500 lines of tests that pinned that behavior.
- The recent execution context appears on more turns. It is placed after the history, so the cached prefix does not change. Its cost is bounded by its budget.
- Without the staged splitter, the model alone keeps B from running in the first turn. The staged scenarios measure this, and passed on every model.
- The image keyword lists remain until the image reference is redesigned.
