'use client';

import * as React from 'react';
import * as AvatarPrimitive from '@radix-ui/react-avatar';

import { cn } from '@/lib/utils';

const Avatar = React.forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Root>
>(({ className, ...props }, ref) => (
  <AvatarPrimitive.Root
    ref={ref}
    className={cn('relative flex h-10 w-10 shrink-0 overflow-hidden rounded-full', className)}
    {...props}
  />
));
Avatar.displayName = AvatarPrimitive.Root.displayName;

const AvatarImage = React.forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Image>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Image>
>(({ className, ...props }, ref) => (
  <AvatarPrimitive.Image
    ref={ref}
    className={cn('aspect-square h-full w-full', className)}
    {...props}
  />
));
AvatarImage.displayName = AvatarPrimitive.Image.displayName;

const AvatarFallback = React.forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Fallback>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Fallback>
>(({ className, ...props }, ref) => (
  <AvatarPrimitive.Fallback
    ref={ref}
    className={cn(
      'flex h-full w-full items-center justify-center rounded-full bg-primary/10 text-primary',
      className
    )}
    {...props}
  />
));
AvatarFallback.displayName = AvatarPrimitive.Fallback.displayName;

// Safe Avatar wrapper component
interface SafeAvatarProps {
  src?: string | null;
  alt?: string | null;
  fallback?: string | null;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

const SafeAvatar = React.forwardRef<HTMLDivElement, SafeAvatarProps>(
  ({ src, alt, fallback, className, size = 'md' }, ref) => {
    const getDisplayName = (name: string | null | undefined): string => {
      if (!name || typeof name !== 'string') return '?';
      return name.trim() ? name.trim().charAt(0).toUpperCase() : '?';
    };

    const sizeClasses = {
      sm: 'h-6 w-6 text-xs',
      md: 'h-8 w-8 text-sm',
      lg: 'h-10 w-10 text-base',
    };

    const hasSrc = typeof src === 'string' && src.trim().length > 0;
    const [imgError, setImgError] = React.useState<boolean>(false);
    React.useEffect(() => {
      // Reset error when src changes so the image attempts to load again
      setImgError(false);
    }, [src]);

    return (
      <Avatar ref={ref} className={cn('relative', sizeClasses[size], className)}>
        {/* Fallback hidden when image src provided and no error */}
        <AvatarFallback
          className={cn(
            'absolute inset-0 font-semibold pointer-events-none transition-opacity',
            hasSrc && !imgError ? 'opacity-0' : 'opacity-100'
          )}
        >
          {getDisplayName(fallback || alt)}
        </AvatarFallback>

        {hasSrc && !imgError ? (
          <img
            key={src}
            src={src as string}
            alt={alt || 'User avatar'}
            className="absolute inset-0 h-full w-full object-cover z-10"
            decoding="async"
            loading="eager"
            crossOrigin="anonymous"
            referrerPolicy="no-referrer"
            onError={() => setImgError(true)}
          />
        ) : null}
      </Avatar>
    );
  }
);
SafeAvatar.displayName = 'SafeAvatar';

export { Avatar, AvatarImage, AvatarFallback, SafeAvatar };
