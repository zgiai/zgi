# ZGI Short Link Root Redirect Contract

This flow verifies the public short-link contract:

- the backend resolver returns the stored target path for a valid short token
- invalid or missing short tokens fail clearly
- `WebBaseURL/{shortToken}` redirects to the existing business route
- existing root-level routes such as `/console` are not captured by the short-link route

Required variables:

- Kest `--base-url`, for example `http://127.0.0.1:2679`
- `web_base_url`, for example `http://127.0.0.1:3000`
- `short_token`, for example `abc234ef`
- `short_token_upper`, for example `ABC234EF`
- `approval_path`, for example `/a/approval-token`
- `invalid_short_token`, for example `abc`
- `unknown_short_token`, for example `def345gh`

```flow
@flow id=zgi-shortlink-root-redirect
@name ZGI short-link root redirect contract
```

```step
@id resolve-short-token
@name Resolve valid short token through backend API

GET /console/api/short-link-resolutions/{{short_token}}
Accept: application/json

[Asserts]
status == 200
code == "0"
data.target_path == "{{approval_path}}"
```

```step
@id reject-invalid-token
@name Reject invalid short token shape

GET /console/api/short-link-resolutions/{{invalid_short_token}}
Accept: application/json

[Asserts]
status == 400
code == "199001"
```

```step
@id resolve-missing-token
@name Return not found for unknown short token

GET /console/api/short-link-resolutions/{{unknown_short_token}}
Accept: application/json

[Asserts]
status == 404
code == "404001"
```

```step
@id web-root-redirects-short-token
@name Web root redirects short token to business route
@type exec

result=$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' "{{web_base_url}}/{{short_token_upper}}")
status_code=${result%% *}
redirect_url=${result#* }
expected_url="{{web_base_url}}{{approval_path}}"
printf 'status=%s\nlocation=%s\n' "$status_code" "$redirect_url"
test "$status_code" = "307"
test "$redirect_url" = "$expected_url"
```

```step
@id web-root-rejects-invalid-token
@name Web root returns 404 for invalid short token shape
@type exec

status_code=$(curl -sS -o /dev/null -w '%{http_code}' "{{web_base_url}}/{{invalid_short_token}}")
printf 'status=%s\n' "$status_code"
test "$status_code" = "404"
```

```step
@id web-root-rejects-missing-token
@name Web root returns 404 when resolver misses token
@type exec

status_code=$(curl -sS -o /dev/null -w '%{http_code}' "{{web_base_url}}/{{unknown_short_token}}")
printf 'status=%s\n' "$status_code"
test "$status_code" = "404"
```

```step
@id console-route-still-works
@name Console route is not captured by short-link route
@type exec

status_code=$(curl -sS -o /dev/null -w '%{http_code}' "{{web_base_url}}/console")
printf 'status=%s\n' "$status_code"
test "$status_code" = "200"
```
