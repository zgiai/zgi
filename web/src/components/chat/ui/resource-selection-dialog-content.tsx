'use client';

import type { ComponentProps, ReactNode } from 'react';
import {
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

/** Existing chat Skill selection layout, with host-owned content and commands. */
export function ResourceSelectionDialogContent({
  title,
  description,
  children,
  footer,
  bodyClassName,
  headerClassName,
  footerClassName,
  ...props
}: Omit<ComponentProps<typeof DialogContent>, 'title'> & {
  title: ReactNode;
  description?: ReactNode;
  footer?: ReactNode;
  bodyClassName?: string;
  headerClassName?: string;
  footerClassName?: string;
}) {
  return (
    <DialogContent {...props} {...(!description ? { 'aria-describedby': undefined } : {})}>
      <DialogHeader className={headerClassName}>
        <DialogTitle>{title}</DialogTitle>
        {description ? <DialogDescription>{description}</DialogDescription> : null}
      </DialogHeader>
      <DialogBody className={bodyClassName}>{children}</DialogBody>
      {footer ? <DialogFooter className={footerClassName}>{footer}</DialogFooter> : null}
    </DialogContent>
  );
}
