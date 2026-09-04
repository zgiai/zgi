# Connectors module

The Connector module owns catalog definitions, personal Key/application connections,
target bindings, access constraints, confirmations and permanent invocation claims.
The old integration implementation and online migration runtime have been removed.

```text
connectors/
├── core/            # dependency-free domain contracts and invariants
├── catalog/         # Connector Plugin admission and catalog projection
├── connections/     # Personal Key lifecycle and canonical persistence
├── bindings/        # stable chat/agent/workflow Target Bindings
├── audit/           # lifecycle decisions and submitted-operation outcomes
├── credentials/     # c1 secret envelopes with identity and purpose isolation
├── providers/       # GitHub, Exa, Feishu, DingTalk, WeCom and WeChat HTTP drivers
├── onboarding/      # durable application, bot registration and session login
├── execution/       # native claims, authorization and provider submission
├── dispatch/        # host-neutral native entry point
└── runtimeconfirmation/ # frozen confirmation continuation lifecycle
```

Dependency rules:

1. `core` imports no provider, route, container, or legacy integration package.
2. Catalog, connection, binding, and audit services depend on narrow interfaces.
3. No production or test package may import `internal/modules/integrations`.
   Host authority adapters belong in the container or consumer, outside this module.
4. Connector code never writes user Keys/App Secrets/device codes to manifests, setup attempts, logs,
   errors, audit payloads, outcomes, or Target Bindings.
5. Every Personal Connection query is scoped by organization and owner before
   credentials may be decrypted.
6. Provider execution has one native path. Unknown or terminal claims never replay.
   Connection and Binding restrictions both apply, including after a replacement.
7. Do not add generic `common`, `helpers`, or `utils` packages.
8. Keep tests beside implementation and run `go test ./internal/modules/connectors/...`.
9. Feishu registration uses the compiled v3 template (40 application scopes; no
   card-read permission). Setup metadata and in-flight requests are version/digest
   pinned. Template updates cannot change a saved credential, auto-start registration,
   revoke an external grant, or authorize an event consumer.
   Name search uses the tenant-capable Directory API and returns only names/Open IDs
   within the app's authorized directory. It never uses user OAuth or sends messages.
   Plugin 1.2.0 adds tenant information and one-page chat user membership reads
   without changing that template. Member pages may exceed the requested count;
   their dedicated projection preserves complete pages within the 1,000-member
   and 1-MiB local limits, rejecting oversized pages rather than losing records.
   Additive catalog operations do not expand stored Connection/Binding constraints.
10. DingTalk, WeCom and WeChat use the closed extension 1.2.0 / contract V3.
    Setup handlers and protocol version 1 are compiled registrations. The browser
    cannot select a registration host, source, driver or verification policy.
    All three are enabled by default. Public protocol parameters are compiled
    into the adapters; no messaging admission environment variable is required.
    Actual account authorization and credential verification remain mandatory.
11. Messaging verification preserves unknown permissions. DingTalk sends remain
    conditional; WeCom sends and WeChat replies need a private conversation handle
    scoped to owner, connection, credential generation, target and binding revision.
    Public API schema discovery and local poll timeouts are never verification.
12. WeChat verification saves its cursor and initial messages in the connection
    transaction. Explicit pulls consume that buffer. Private state and conversation
    handles use separate c1 purposes; they never enter target configuration.

Chat uses live bindings. Agent and Workflow drafts and every published version
use independent identities. Workflow bindings also include the node identity.
Agent/Workflow repairs commit with the host save; published bindings are immutable.

Historical data conversion lives in the independent `tools/connector-migrate`
module. Runtime lifecycle and execution must not query historical integration,
mapping, rollout, reversal or conversion tables. See [Connector setup](../../../../docs/connectors.md)
for setup behavior, operational limits and schema rollout requirements.
