'use client';

import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';

import Link from 'next/link';

import { Icons } from '@/components/ui/icons';
import { useAuthStore } from '@/store/auth-store';
import { useLogout } from '@/hooks';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from '@/components/ui/dropdown-menu';
import { Button } from '@/components/ui/button';
import { SafeAvatar } from '@/components/ui/avatar';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useAutoProfile } from '@/hooks/use-profile';
import { getLogoutRedirectUrl } from '@/lib/config';
import { markAuthRedirectInProgress } from '@/lib/auth/logout-state';
import { setPendingLogoutRedirect } from '@/utils/logout-redirect';
import { useLocale } from '@/hooks/use-locale';
import { useUpdateInterfaceLanguage } from '@/hooks/use-update-interface-language';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { LANGUAGES } from '@/lib/constants';
import type { Locale } from '@/lib/i18n';
import { Building2 } from 'lucide-react';

export function UserMenu() {
  const router = useRouter();
  const tNav = useT('navigation');
  const tAuth = useT('auth');
  const tUi = useT('ui');
  const { locale, isEnabled: isLanguageSwitchEnabled } = useLocale();
  const updateLanguageMutation = useUpdateInterfaceLanguage();

  // Authentication state
  const user = useAuthStore.use.user();
  const isLoggingOut = useAuthStore.use.isLoggingOut();
  const logoutMutation = useLogout();
  const isLogoutDisabled = isLoggingOut || logoutMutation.isPending;
  const { canAccessOrganizationDashboard } = useAccountCapabilities();

  // Organizations hook (with data fetching & permission handling)
  const { organizations, currentOrganization, switchOrganization } = useOrganizations(true);

  const { data: profile } = useAutoProfile({ staleTime: 1_800_000 });
  const displayName = profile?.name || user?.name || tNav('profile');
  const displayEmail = profile?.email || user?.email || '';
  const displayOrganization = currentOrganization?.short_name || currentOrganization?.name || null;
  // Logout handler
  const handleLogout = async () => {
    if (isLogoutDisabled) {
      return;
    }

    const logoutRedirectUrl = getLogoutRedirectUrl();
    setPendingLogoutRedirect(logoutRedirectUrl);
    try {
      await logoutMutation.mutateAsync();
    } catch {
      // Local cleanup and redirect are handled by the logout mutation lifecycle.
    }

    if (typeof window !== 'undefined') {
      markAuthRedirectInProgress();
      window.location.replace(logoutRedirectUrl);
      return;
    }
    router.push(logoutRedirectUrl);
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" isIcon className="rounded-full overflow-hidden">
          <SafeAvatar
            src={profile?.avatar_url || user?.avatar_url || null}
            alt={profile?.name || user?.name || null}
            fallback={profile?.name || user?.name || null}
            size="lg"
          />
          <span className="sr-only">{tNav('profile')}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="p-0">
          <div className="flex items-center gap-3 px-2 py-2.5">
            <SafeAvatar
              src={profile?.avatar_url || user?.avatar_url || null}
              alt={displayName}
              fallback={displayName}
              size="md"
            />
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-foreground">{displayName}</div>
              <div className="truncate text-xs text-muted-foreground">{displayEmail}</div>
            </div>
          </div>
          {displayOrganization ? (
            <div className="flex items-center gap-2 border-t border-border/60 px-2 py-2 text-[11px] text-muted-foreground">
              <Building2 className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">
                {tNav('organizations')}: {displayOrganization}
              </span>
            </div>
          ) : null}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {/* Organizations section rendered as a sub-menu */}
        {organizations.length > 0 && (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger inset>{tNav('organizations')}</DropdownMenuSubTrigger>
            <DropdownMenuSubContent sideOffset={6} className="w-60 max-h-64 overflow-y-auto">
              {organizations.map(org => (
                <DropdownMenuItem
                  key={org.id}
                  onClick={() => switchOrganization(org)}
                  disabled={org.id === currentOrganization?.id}
                >
                  {org.id === currentOrganization?.id && (
                    <Icons.Check className="h-4 w-4 text-primary" />
                  )}
                  <span className="truncate">{org.short_name || org.name}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
        {canAccessOrganizationDashboard && (
          <DropdownMenuItem asChild>
            <Link href="/dashboard" className="!cursor-pointer">
              <Icons.LayoutDashboard className="h-4 w-4" />
              <span>{tNav('dashboard')}</span>
            </Link>
          </DropdownMenuItem>
        )}
        {isLanguageSwitchEnabled && (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger className="gap-2">
              <Icons.languages className="h-4 w-4" />
              <span>{tUi('selectLanguage')}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent sideOffset={6} className="w-44">
              {LANGUAGES.map(lang => (
                <DropdownMenuItem
                  key={lang.value}
                  onClick={() => updateLanguageMutation.mutate(lang.value as Locale)}
                  disabled={locale === lang.value || updateLanguageMutation.isPending}
                >
                  {locale === lang.value && <Icons.Check className="h-4 w-4 text-primary" />}
                  <span className="truncate">{lang.label}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
        {/* Profile */}
        <DropdownMenuItem asChild>
          <Link href="/profile" className="!cursor-pointer">
            <Icons.User className="h-4 w-4" />
            <span>{tNav('profile')}</span>
          </Link>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {/* Logout */}
        <DropdownMenuItem
          onClick={handleLogout}
          variant="destructive"
          className="!cursor-pointer"
          disabled={isLogoutDisabled}
        >
          <Icons.LogOut className="h-4 w-4" />
          <span>{tAuth('logout')}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
