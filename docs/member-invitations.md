# Organization member invitations

When an organization administrator directly adds a member, workspace assignment is optional.

- If a workspace is assigned, the activation email is bound to the workspace returned by the completed membership operation. Activation displays that destination and selects it as the member's current workspace.
- If no workspace is assigned, the invitation remains organization-only. Activation must not select an arbitrary workspace or create a personal organization.
- The server resolves the destination from the stored, recipient-bound invitation token. Changing a URL parameter must not redirect an invitation into another workspace.
- The activation preflight rejects invitations whose assigned workspace or organization is unavailable.
- Existing active accounts authenticate before accepting an invitation; activation must not replace an active account's password.

Invitations issued before a destination propagation fix may still contain organization-only metadata. Already activated members can select an assigned workspace using the workspace switcher; this does not grant new membership. Do not reset an account or rewrite its permissions to repair the selected workspace.

Run the registration and invitation regression gate from the repository root:

```bash
bash scripts/check-registration-auth.sh --backend-only
```

The gate includes the direct-add handler and email-to-activation destination tests. These local tests do not replace deployment verification or a real invited-member browser test.
