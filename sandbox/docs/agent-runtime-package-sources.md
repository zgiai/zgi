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

## System-package version recovery

Installation-enabled images and E2B templates must advertise
`system_package_ledger_version: 2`. Updated Workers reject older installation
runtimes before readiness; rebuild the image/template before updating its approved
reference. Do not downgrade a version-locked session to a Worker/template that
only understands historical command replay. Offline/legacy Profiles without the
installation capability are unaffected.

After a successful guarded APT installation, the Runtime Agent queries its fixed
system package database and atomically records installed names, architectures and
versions, including dependencies and baseline packages. The ledger is bound to
the runtime Profile identity, CPU architecture and resolved package-source digest.
Repeated installation refreshes the resolved inventory even when its original
command was already recorded. Download commands are audit records, not installed
packages, and are not rerun during recovery. Virtual package requests are tracked
through the real installed provider inventory rather than a fictitious package.

On compute reconstruction, the Agent compares actual versions with the recorded
inventory. Matching packages need no installation or network request. Missing or
changed packages are installed with validated `name:architecture=version` pins;
only this internal recovery path can request downgrades, and package removal is
prohibited. A second database query verifies every recorded version before user
execution can proceed. Extra packages are not automatically removed.

Unavailable versions, dependency conflicts, query failures, mismatched identity,
malformed locks and failed verification block execution with a retryable error.
There is no automatic latest-version fallback. Fixed-version recovery therefore
requires the approved mirror to retain those versions; this feature does not add
an archive mirror or store `.deb` payloads/rootfs layers. Operators must maintain
retained package versions or explicitly migrate an environment, not delete its
Workspace to hide a recovery failure.

Historical unversioned ledgers remain readable. Their install commands are replayed
under the existing restrictions once, then the actual restored inventory is
captured as version 2. They cannot reproduce unknown historical versions; migration
must not be described as retroactively locking the original installation.

Ledgers remain untrusted Workspace data: only bounded, validated package/version
fields can construct privileged arguments. The 1 MiB ledger limit, 1,000 command
limit and 4,096 installed-package limit fail explicitly. Environment bindings are
consistency checks, not cryptographic attestation. Python/npm use the separate
retained-metadata contract below, not privileged system-package replay.

## Retained Python and Node versions

Installation-enabled images/templates must also advertise
`language_package_ledger_version: 1`; Workers reject an older installation runtime
before readiness. Build and approve the matching runtime and Worker together.
Offline/legacy Profiles are unaffected.

At each quiescent Workspace checkpoint, the Agent atomically records actual
distribution names, versions and metadata locations in a separate session ledger.
It reads the session virtual environment's `lib/python3.*/site-packages` metadata
(`.dist-info/METADATA` and directory-form `.egg-info/PKG-INFO`) and installed
`node_modules` package metadata throughout the Workspace. Nested and scoped Node
packages have distinct paths; confined pnpm/npm package-directory links are read
without recursively following link graphs. Preinstalled image packages are covered
by the immutable Profile identity, not copied into this session inventory.

Collection never starts Python, Node, npm hooks or user code as root. Metadata
must be a bounded regular file. Escaping links, metadata-file symlinks, FIFO/device
files, invalid identities, incomplete package directories and excessive inventories
fail checkpointing explicitly. Limits are 256 KiB per metadata file, 16 MiB total
metadata, 4,096 records, a 1 MiB ledger and the existing 10,000 Workspace entry limit.

Before the first Job in reconstructed compute, the Agent checks the retained
inventory against that ledger and its Profile/source/architecture identity. A
missing package, changed version/path, unexpected extra installation or malformed
ledger blocks user execution with `language_package_recovery_failed`. Repairing
the retained checkpoint permits retry; no automatic reinstall or latest-version
fallback occurs. Checkpointing cannot silently bless a damaged imported inventory.
Subsequent Jobs may intentionally upgrade or remove dependencies; the next
quiescent checkpoint records the new state. Callers must stop Jobs before capture,
as with the existing Workspace archive contract.

Historical checkpoints without this ledger remain readable and gain records on
their next checkpoint; original unknown versions cannot be recovered retroactively.
This checks metadata consistency, not every installed file or importability, and
does not attest package authenticity. Custom `pip --target` directories, editable
source trees and Node global installs are not claimed as inventoried. Retained
files remain the recovery mechanism: version-pinned wheel/npm replay and shared
cross-session package artifacts are separate future work.

Package queries and fixed-version installation use the documented Debian
[dpkg-query fields](https://manpages.debian.org/bookworm/dpkg/dpkg-query.1.en.html)
and [APT version selection](https://manpages.debian.org/bookworm/apt/apt-get.8.en.html).

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
