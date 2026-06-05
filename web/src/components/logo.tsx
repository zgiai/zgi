import { ROUTES } from '@/constants/routes';
import { APP_NAME, DARK_LOGO_URL, LOGO_REDIRECT_URL, LOGO_URL } from '@/lib/config';
import { useSafeTheme } from '@/providers/theme-provider';
import Link from 'next/link';

function LogoContent({ showName = true }) {
  const { isDark } = useSafeTheme();

  return (
    <>
      <img
        src={isDark ? DARK_LOGO_URL : LOGO_URL}
        alt={APP_NAME}
        className="max-h-10 max-w-32"
        loading="lazy"
      />
      {showName && <span>{APP_NAME}</span>}
    </>
  );
}

export function Logo({ routerToHome = true, showName = true }) {
  if (!routerToHome) {
    return (
      <div className="flex items-center gap-2 font-semibold">
        <LogoContent showName={showName} />
      </div>
    );
  }

  const isExternalRedirect = /^([a-zA-Z][a-zA-Z\d+\-.]*:)?\/\//.test(LOGO_REDIRECT_URL);

  if (isExternalRedirect) {
    return (
      <a href={LOGO_REDIRECT_URL} className="flex items-center gap-2 font-semibold">
        <LogoContent showName={showName} />
      </a>
    );
  }

  return (
    <Link
      href={LOGO_REDIRECT_URL || ROUTES.CONSOLE.HOME}
      className="flex items-center gap-2 font-semibold"
    >
      <LogoContent showName={showName} />
    </Link>
  );
}
