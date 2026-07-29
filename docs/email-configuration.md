# Email registration and delivery

ZGI supports two delivery providers through one application-facing email service:

- `resend`: Resend or an API compatible with `POST /emails`, Bearer authentication, and the standard Resend request/response shape.
- `smtp`: standard SMTP with required STARTTLS, implicit TLS, or an explicitly unencrypted connection.

Email registration uses a one-time state machine. Sending a code creates a 10-minute challenge token. A successful code check consumes that challenge and returns a different 10-minute verified token. Only the verified token can create an account, and it is consumed when registration finishes. Five incorrect code attempts revoke the challenge. Re-sending to the same normalized email address has a one-minute cooldown.

## Enable public email registration

Both feature flags are required:

```dotenv
PUBLIC_DEPLOYMENT_ENABLED=true
ALLOW_REGISTER=true
```

`PUBLIC_DEPLOYMENT_ENABLED` keeps account/workspace creation in public-deployment mode. `ALLOW_REGISTER` is the operator-controlled signup switch.

## Resend or a compatible API

Official Resend only requires two values:

```dotenv
EMAIL_FROM="ZGI Platform <system@notify.example.com>"
RESEND_API_KEY=replace-with-provider-key
```

For a Resend-compatible proxy, add its base URL:

```dotenv
RESEND_BASE_URL=https://mail.example.com/v1
```

`RESEND_BASE_URL` is a base URL, not the complete endpoint. ZGI normalizes the trailing slash and appends `/emails`. A compatible proxy must accept `Authorization: Bearer ...` and return a non-empty message `id` for a successful request.
Production deployments must use HTTPS because the Resend API key is sent as a Bearer credential. Plain HTTP proxy URLs are accepted only in local or development mode.

Optional branding and invitation-link settings:

```dotenv
EMAIL_MAIL_TEMPLATE_BRAND_NAME=ZGI
EMAIL_MAIL_TEMPLATE_LOGO_URL=https://example.com/logo.png
EMAIL_CONSOLE_WEB_URL=https://console.example.com
```

## SMTP

STARTTLS on port 587 is the usual server configuration:

```dotenv
EMAIL_PROVIDER=smtp
EMAIL_FROM="ZGI Platform <system@notify.example.com>"
EMAIL_SMTP_SERVER=smtp.example.com
EMAIL_SMTP_PORT=587
EMAIL_SMTP_USERNAME=system@notify.example.com
EMAIL_SMTP_PASSWORD=replace-with-app-password
EMAIL_SMTP_SECURITY=starttls
```

For servers using TLS immediately on connection, commonly port 465, set `EMAIL_SMTP_SECURITY=implicit_tls`. Use `none` only on a trusted private network where the SMTP server intentionally has no TLS. With `starttls`, startup delivery fails if the server does not advertise STARTTLS.

When SMTP is selected with the preferred `EMAIL_PROVIDER=smtp` key, omitted `EMAIL_SMTP_SECURITY` defaults to `starttls`. Existing deployments that select SMTP with a legacy provider key or set the legacy TLS flags keep their previous behavior; migrate them to an explicit `EMAIL_SMTP_SECURITY` value when practical.

## Compatibility keys

Existing deployments can continue using `EMAIL_FROM_NAME`, `EMAIL_FROM_ADDRESS`, `EMAIL_MAIL_DEFAULT_SEND_FROM`, `EMAIL_RESEND_API_KEY`, `EMAIL_RESEND_BASE_URL`, `EMAIL_RESEND_API_URL`, `EMAIL_MAIL_TYPE`, `MAIL_TYPE`, `EMAIL_PORT`, `EMAIL_SMTP_USE_TLS`, and `EMAIL_SMTP_OPPORTUNISTIC_TLS`. New Resend deployments should prefer `EMAIL_FROM`, `RESEND_API_KEY`, and—only for proxies—`RESEND_BASE_URL`. The shorter keys take precedence when both forms are present.

Never commit provider keys or SMTP passwords. Put real values in the deployment secret manager or an ignored `.env` file.
