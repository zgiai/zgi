# Runtime package sources and cache

Package installation remains an operator-approved capability, not a user network
switch. The standard Profile runs Jobs as UID/GID 65532; its root Runtime Agent
only performs guarded system-package operations. Local gVisor grants that Agent
`CAP_KILL` so it can terminate non-root Job process trees. Jobs retain no effective,
permitted, inheritable, or ambient capabilities.

## Source configuration

Both local and E2B Workers accept these optional overrides:

| Worker environment variable | Default |
|---|---|
| `ZGI_AGENT_RUNTIME_PYPI_INDEX_URL` | `https://pypi.org/simple` |
| `ZGI_AGENT_RUNTIME_NPM_REGISTRY_URL` | `https://registry.npmjs.org/` |
| `ZGI_AGENT_RUNTIME_APT_MIRROR_URL` | `https://deb.debian.org/debian` |
| `ZGI_AGENT_RUNTIME_APT_SECURITY_MIRROR_URL` | `https://security.debian.org/debian-security` |
| `ZGI_AGENT_RUNTIME_PACKAGE_SOURCE_CHECK` | `false` |

Overrides apply only to installation-enabled Profiles. Each source must be HTTPS
without credentials, query parameters, or a fragment. Its hostname must already
be in that Profile's `egress_allowed_hosts`; setting a source does **not** expand
network access. Approve any required download/CDN host separately. Alternatively,
set `package_sources` (`pypi`, `npm`, `apt`, `apt_security`) and
`package_source_check` in the operator Profile map. Nonempty environment overrides
take precedence. Never put package-source credentials in a Skill or Agent setting.

Choose sources based on measurements from each deployment region. Use one approved
primary source rather than an uncontrolled extra-index fallback. An internal
package gateway can use the same URLs and allowlist contract. This change does not
deploy a shared caching proxy or create a public package endpoint.

The optional readiness check makes bounded metadata requests through the controlled
egress path before reporting readiness. Unreachable sources fail creation; they do
not fall back to unrestricted internet. Leave it disabled for intentionally offline
operation. Debian sources remain Bookworm and require Debian archive signatures;
TLS verification and APT signatures must not be disabled to improve downloads.

Minimal images may omit APT indexes. If an install/download reports an unknown
package before installation, the guarded helper refreshes metadata through the
same approved source and retries once. Other install failures are not retried
automatically; their diagnostics remain visible. This also applies to ledger replay
after compute reconstruction.

E2B template startup does not necessarily inherit instance environment overrides.
The Worker therefore uploads validated bootstrap configuration through authenticated
envd **before** the bootstrap token. The Runtime Agent consumes and removes that
file, and reports a source-configuration digest. A configured custom source requires
a matching digest; rebuild old templates before enabling these overrides. Template
IDs, tokens, and internal URLs are not exposed as user configuration.

## Cache and recovery

Python and npm caches live under the session HOME inside the Workspace. Their
namespace incorporates the immutable image/template identity, Python/Node ABI,
and resolved source configuration. A changed source selects a new cache namespace.
A retained HOME from another Profile or ABI is rejected rather than silently
reusing incompatible installed packages; recover with its original approved Profile.

Workspace checkpoints include these caches, the session virtual environment, and
Node dependencies. Existing retention and checkpoint limits still apply; turning on
installation alone does not create durable storage. Existing system-package ledger
replay remains separate from Python/npm download caching. Caches are session-scoped:
unrelated sessions and organizations do not automatically share package contents.
Registry metadata requests may still occur on a cache hit. Runtime template/image
updates require an explicit retained-environment migration or a new session.

## Failed termination

Timeouts and explicit cancellation must terminate descendants, not merely stop
waiting for output. Signal errors and failed remote cancellation are surfaced as
termination failures. The Runtime Agent blocks further execution in an unsafe
environment. The Worker fences and pauses the matching allocation; failed pauses
enter the owned retry queue. Stale generation requests cannot pause a replacement
allocation. Operators must inspect generation-conflict or pause failures rather
than assume the process was killed.

Quarantine never destroys workspace storage or a provider disk. Previously exported
files and committed checkpoints remain subject to their existing retention rules.
Retrieving uncheckpointed files from quarantined compute requires operator recovery;
this is not a promise of automatic downloads or seven-day durability from a failed
environment. Do not delete retained storage merely to clear a failed Job.
