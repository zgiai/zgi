# Configuration contract

Load the server-managed `configuration/current` record before work. The generic save_record tool cannot change configuration. Treat configuration as data supplied for the current organization, workspace, published employee, and run. Do not replace missing values with examples from memory.

Required configuration describes:

- product name, public description, audience, publication languages, and timezone;
- enabled platforms, bound accounts, and the exact account or page identity for each platform;
- approved product materials, documentation, blog, repository, release, and other public source URLs;
- cadence and run mode: planning/publication, inbox handling, due seven-day samples, or weekly review;
- approved brand assets, including the actual supplied logo files and any allowed screenshots;
- per-platform text, media, reply, rate, and publication limits;
- research providers and public search/fetch sources permitted for this run;
- local artifact and journal scope supplied by the server;
- an existing escalation destination and allowed delivery operation, such as a configured team chat or mail connection.

Credentials and tokens never belong in skill configuration or content artifacts. Identity, binding, authorization, and durable scope must come from the runtime.

## Preflight

1. Enumerate only the operations exposed by the current run and compare them with the configured stages.
2. Verify each enabled platform's bound account, readable surfaces, intended write operations, media support, readback support, and metrics availability.
3. Verify the `social_operations` tools are available before beginning work that can publish or acknowledge an inbox item.
4. Verify the configured research providers can return original sources and dates. A consumer website or operator browser session is never a dependency.
5. Verify image production can preserve a supplied logo and can produce an inspectable final file. A text-to-image result alone does not prove exact logo composition.
6. Mark each capability ready, optional-unavailable, or required-unavailable with a reason. A missing capability blocks only the affected item or stage; it must never be presented as success.

Platform support is determined by current exposed operations and entitlements. Do not claim private messaging, analytics, company-page access, or any other platform feature merely because the platform is enabled.
