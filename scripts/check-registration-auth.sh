#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Running registration and invitation backend tests"
(
  cd "${repo_root}/api"
  go test ./internal/modules/user/auth/... ./internal/util ./pkg/email -count=1
  go test ./middleware \
    -run 'Test(CurrentWorkspaceRequired|ShouldSkipTenantResolutionForOnboardingRoutes)' \
    -count=1
  go test ./internal/modules/workspace/handler \
    -run 'Test(GetInvitationInfo|AcceptInvitation|OrganizationInvite|MemberActivationURL|MembersHandlerInvite|WorkspaceStatistics|UpdateWorkspace)' \
    -count=1
  go test ./internal/modules/workspace/service \
    -run 'Test(InviteMemberDefaultsCreateUsableWorkspaceContext|WorkspaceMemberDefaultsNormalizeRoleID|DirectAddOrganizationMember|InviteCurrentOrganizationMember)' \
    -count=1
)

echo "Running registration and invitation frontend checks"
(
  cd "${repo_root}/web"
  pnpm exec eslint --max-warnings=17 \
    'src/app/(auth)/invite/[token]/page.tsx' \
    'src/app/(auth)/layout.tsx' \
    'src/app/console/layout.tsx' \
    'src/app/onboarding/layout.tsx' \
    'src/app/onboarding/organization/page.tsx' \
    'src/components/activate-form.tsx' \
    'src/components/auth/complete-registration-form.tsx' \
    'src/components/auth/login-form.tsx' \
    'src/components/auth/sso-callback-handler.tsx' \
    'src/i18n/modules/auth/en-US.ts' \
    'src/i18n/modules/auth/zh-Hans.ts' \
    'src/i18n/route-modules.ts' \
    'src/services/auth.service.ts' \
    'src/services/organization.service.ts' \
    'src/services/types/auth.ts'
  pnpm type-check
)
