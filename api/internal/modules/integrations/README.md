# Integrations module layout

The `integrations` directory is organized by function. Its root is only a
package container; it contains documentation and focused subdirectories, not
Go source files.

```text
integrations/
├── providers/
│   ├── email/          # Gmail and generic IMAP/SMTP mail
│   ├── collaboration/  # WeCom, DingTalk, and Feishu
│   ├── developer/      # GitHub
│   ├── websearch/      # Exa
│   └── social/         # X
├── core/               # cross-provider contracts and orchestration
├── audit/              # audit-key primitives
├── catalog/            # authorized catalog health projection
├── credentials/        # credential encryption primitives
├── execution/          # execution audit and receipt persistence
├── localization/       # localized catalog text
├── preferences/        # AIChat connection preference domain and persistence
├── quota/              # quota persistence
├── metatools/          # Connected Apps meta-tools
└── toolprovider/       # runtime tool-provider bridge
```

## Directories

| Directory | Responsibility | Dependency direction |
| --- | --- | --- |
| `providers/email/gmail` | Gmail OAuth, messages, and actions | imports the core package |
| `providers/email/mail` | Generic IMAP/SMTP connection and actions | imports the core package |
| `providers/collaboration/wecom` | WeCom provider, suite install, and callbacks | imports the core package |
| `providers/collaboration/{dingtalk,feishu}` | Collaboration provider clients and actions | imports the core package |
| `providers/developer/github` | GitHub client and actions | imports the core package |
| `providers/websearch/exa` | Exa web-search client and actions | imports the core package |
| `providers/social/x` | X OAuth, client, and actions | imports the core package |
| `audit` | Audit key derivation | core-independent foundation |
| `catalog` | Authorized catalog health projection | imports the core package |
| `core` | Shared models, contracts, validation, registry, and orchestration | imports foundation packages only |
| `credentials` | Credential envelope encryption and memory clearing | core-independent foundation |
| `execution` | Gorm execution-audit and operation-receipt persistence | imports the core package |
| `localization` | Localized catalog text normalization | core-independent foundation |
| `preferences` | AIChat connection preference model, service, and persistence | imports the core package through narrow connection ports |
| `quota` | Redis-backed integration quota enforcement | core-independent foundation |
| `metatools` | AI-facing Connected Apps meta tools | imports the core package |
| `toolprovider` | Tool-provider bridge | imports the core package |

## Placement rules

1. Put provider-specific behavior under
   `providers/<capability>/<provider>`. Do not merge multiple Go packages into
   one directory merely because they share a capability.
2. Put reusable primitives in a focused foundation package that does not
   import `integrations/core`.
3. Put leaf projections and infrastructure adapters in a focused subpackage;
   they may import the core package, but the core package must not import them.
4. Keep only cross-provider models, interfaces, validation, registry behavior,
   and orchestration in `core` when splitting them would create a cycle. A
   provider API client, callback protocol, or endpoint never belongs there.
5. Do not add catch-all directories such as `common`, `helpers`, or `utils`.
6. Do not add Go source files directly under `integrations/`.
7. Keep tests beside the implementation package and run
   `go test ./internal/modules/integrations/...` after structural changes.
8. Do not keep generated logs or other runtime state in this source tree.

Gmail and generic IMAP/SMTP mail intentionally remain separate Go packages
under the same `email` capability. WeCom also remains one cohesive provider
package because its suite, callback crypto, and adapter share private state.
