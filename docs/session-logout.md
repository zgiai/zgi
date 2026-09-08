# Console session logout

`POST /console/api/logout` accepts an `Authorization: Bearer <access-token>`
header and an optional JSON body containing `refresh_token`. Console clients
send both credentials for the current session and clear local session state even
if the request fails. A local sign-out alone does not prove server-side revocation.

The server validates the access-token signature and records a SHA-256 token
fingerprint in Redis until that JWT's expiry. Authentication checks this record;
a revoked JWT is rejected. Each newly issued JWT has a unique `jti`, so signing in
again within the same second cannot recreate the revoked token. Previously issued
JWTs without `jti` are still checked by fingerprint.

The submitted refresh token must belong to the authenticated account. Its current
and legacy records are removed atomically; pointers for a different session are
preserved. Repeated logout is idempotent. An expired, correctly signed access token
can identify its own refresh token for logout, but is never accepted for normal
authentication. Older clients omitting the body revoke only their presented access
token; they must upgrade to revoke the refresh credential too.

Revocation storage is part of the authentication security boundary. Redis must be
available and retain these keys until expiry; avoid eviction or flushing of
security records. When the check cannot complete, protected requests fail closed
with the existing system-error response (HTTP 500), not an invalid-session response.
Clients may retry after recovery without discarding credentials. Redis checks are
bounded to two seconds. This adds a Redis read to JWT validation and must be
included in deployment capacity and availability checks.

This endpoint revokes the presented credential pair, not every device or account
session. It is not a global sign-out or a refresh-token-family revocation protocol.
Verify browser navigation, reload protection, and concurrent refresh behavior
separately; successful backend unit tests alone do not prove end-to-end logout.
