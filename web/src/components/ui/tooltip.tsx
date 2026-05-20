'use client';

import * as React from 'react';
import * as TooltipPrimitive from '@radix-ui/react-tooltip';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '@/lib/utils';

const TooltipProvider = ({
  delayDuration = 0,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) => (
  <TooltipPrimitive.Provider delayDuration={delayDuration} {...props} />
);

const Tooltip = TooltipPrimitive.Root;

const TooltipTrigger = TooltipPrimitive.Trigger;

const tooltipVariants = cva(
  'z-50 w-fit rounded-lg px-4 py-2 text-xs font-medium shadow-tooltip transition-all duration-500 outline-none ' +
    'data-[state=delayed-open]:data-[side=top]:animate-float-in-top ' +
    'data-[state=delayed-open]:data-[side=bottom]:animate-float-in-bottom ' +
    'data-[state=delayed-open]:data-[side=left]:animate-float-in-left ' +
    'data-[state=delayed-open]:data-[side=right]:animate-float-in-right ' +
    'data-[state=closed]:data-[side=top]:animate-float-out-top ' +
    'data-[state=closed]:data-[side=bottom]:animate-float-out-bottom ' +
    'data-[state=closed]:data-[side=left]:animate-float-out-left ' +
    'data-[state=closed]:data-[side=right]:animate-float-out-right ' +
    'data-[state=closed]:opacity-0',
  {
    variants: {
      variant: {
        default: 'bg-popover/90 text-popover-foreground backdrop-blur-md border border-border/40',
        inverted: 'bg-zinc-950/90 text-zinc-50 backdrop-blur-md border border-white/10 shadow-2xl',
        primary:
          'bg-primary/90 text-primary-foreground backdrop-blur-md border border-primary/20 shadow-lg shadow-primary/10',
        destructive:
          'bg-destructive/10 text-destructive backdrop-blur-md border border-destructive/20 shadow-lg shadow-destructive/5',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content> &
    VariantProps<typeof tooltipVariants>
>(({ className, sideOffset = 10, variant, children, ...props }, ref) => (
  <TooltipPrimitive.Portal>
    <TooltipPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      data-slot="tooltip-content"
      className={cn(tooltipVariants({ variant, className }))}
      {...props}
    >
      {children}
    </TooltipPrimitive.Content>
  </TooltipPrimitive.Portal>
));
TooltipContent.displayName = TooltipPrimitive.Content.displayName;

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider };
