'use client';

import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import { Loader2 } from 'lucide-react';

import { cn } from '@/lib/utils';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap text-sm font-semibold transition-all focus-ring shrink-0 disabled:cursor-not-allowed disabled:!border-border disabled:!bg-muted disabled:!text-muted-foreground disabled:!shadow-none disabled:hover:!bg-muted disabled:hover:!text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default:
          'border border-primary bg-primary text-primary-foreground shadow-none hover:border-primary-hover hover:bg-primary-hover active:border-primary-active active:bg-primary-active',
        destructive:
          'border border-destructive/10 bg-destructive text-white shadow-none hover:bg-destructive/90 focus-visible:ring-destructive/30',
        outline:
          'border border-border bg-background shadow-none hover:border-border-strong hover:bg-accent hover:text-accent-foreground dark:bg-input/30 dark:border-input dark:hover:bg-input/50',
        secondary:
          'border border-transparent bg-secondary text-secondary-foreground shadow-none hover:bg-secondary-hover',
        ghost: 'hover:bg-secondary/80 hover:text-accent-foreground dark:hover:bg-accent/50',
        link: 'text-muted-foreground underline-offset-4 hover:text-primary hover:underline',
      },
      size: {
        xs: 'h-[26px] rounded-[3px] px-2 text-xs',
        sm: 'h-[30px] rounded-[3px] px-3 text-xs',
        default: 'h-[34px] rounded-[4px] px-3.5 text-[13px]',
        lg: 'h-[38px] rounded-[4px] px-5 text-sm',
        xl: 'h-10 rounded-[4px] px-7 text-base',
        '2xl': 'h-11 rounded-[4px] px-9 text-lg',
      },
      isIcon: {
        true: 'aspect-square p-0',
      },
      interactive: {
        true: 'interactive',
        subtle: 'interactive-subtle shadow-none',
        false: '',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
      isIcon: false,
      interactive: true,
    },
  }
);

interface ButtonProps extends React.ComponentProps<'button'>, VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  loading?: boolean;
}

/**
 * Primary Button component with zweb-inspired premium effects.
 * Includes spring animations, inner lighting, and built-in loading states.
 */
const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      variant,
      size,
      isIcon,
      interactive,
      asChild = false,
      loading,
      children,
      disabled,
      ...props
    },
    ref
  ) => {
    const Comp = asChild ? Slot : 'button';

    if (asChild) {
      return (
        <Comp
          ref={ref}
          data-slot="button"
          className={cn(buttonVariants({ variant, size, isIcon, interactive, className }))}
          {...props}
        >
          {children}
        </Comp>
      );
    }

    return (
      <Comp
        ref={ref}
        data-slot="button"
        className={cn(buttonVariants({ variant, size, isIcon, interactive, className }))}
        disabled={disabled || loading}
        {...props}
      >
        {loading && <Loader2 className="size-4 animate-spin" />}
        {!loading && children}
      </Comp>
    );
  }
);
Button.displayName = 'Button';

export { Button, buttonVariants };
