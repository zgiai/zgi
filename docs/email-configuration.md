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

```dotenv
EMAIL_PROVIDER=resend
EMAIL_FROM_NAME=ZGI Platform
EMAIL_FROM_ADDRESS=system@notify.example.com
EMAIL_RESEND_API_KEY=replace-with-provider-key
EMAIL_RESEND_BASE_URL=https://api.resend.com
EMAIL_MAIL_TEMPLATE_BRAND_NAME=ZGI
EMAIL_MAIL_TEMPLATE_LOGO_URL=https://example.com/logo.png
EMAIL_CONSOLE_WEB_URL=https://console.example.com
```

`EMAIL_RESEND_BASE_URL` is a base URL, not the complete endpoint. ZGI normalizes the trailing slash and appends `/emails`. A compatible proxy must accept `Authorization: Bearer ...` and return a non-empty message `id` for a successful request.

## SMTP

STARTTLS on port 587 is the usual server configuration:

```dotenv
EMAIL_PROVIDER=smtp
EMAIL_FROM_NAME=ZGI Platform
EMAIL_FROM_ADDRESS=system@notify.example.com
EMAIL_SMTP_SERVER=smtp.example.com
EMAIL_SMTP_PORT=587
EMAIL_SMTP_USERNAME=system@notify.example.com
EMAIL_SMTP_PASSWORD=replace-with-app-password
EMAIL_SMTP_SECURITY=starttls
```

For servers using TLS immediately on connection, commonly port 465, set `EMAIL_SMTP_SECURITY=implicit_tls`. Use `none` only on a trusted private network where the SMTP server intentionally has no TLS. With `starttls`, startup delivery fails if the server does not advertise STARTTLS.

## Compatibility keys

Existing deployments can continue using `EMAIL_MAIL_TYPE`, `MAIL_TYPE`, `EMAIL_MAIL_DEFAULT_SEND_FROM`, `EMAIL_RESEND_API_URL`, `EMAIL_PORT`, `EMAIL_SMTP_USE_TLS`, and `EMAIL_SMTP_OPPORTUNISTIC_TLS`. New deployments should use the canonical keys above. Canonical keys take precedence when both forms are present.

Never commit provider keys or SMTP passwords. Put real values in the deployment secret manager or an ignored `.env` file.
