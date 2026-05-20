'use client';

import React from 'react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

export interface RowProps {
  children?: React.ReactNode;
  className?: string;
}
export function Row({ children, className }: RowProps) {
  return <div className={cn('grid grid-cols-1 gap-6 lg:grid-cols-2', className)}>{children}</div>;
}

export interface ColProps {
  children?: React.ReactNode;
  sticky?: boolean;
  className?: string;
}
export function Col({ children, sticky, className }: ColProps) {
  return (
    <div className={cn('space-y-4', sticky ? 'lg:sticky lg:top-16 lg:self-start' : '', className)}>
      {children}
    </div>
  );
}

export interface HeadingProps {
  url: string;
  method: string;
  title: string;
  name?: string; // anchor id (can include leading #)
}
export function Heading({ url, method, title, name }: HeadingProps) {
  const anchorId = (name || '')?.replace(/^#/, '') || undefined;
  return (
    <div className="space-y-2">
      <h2 id={anchorId} className="text-[28px] font-semibold tracking-tight mt-8 group">
        {title}
        {anchorId ? (
          <a
            href={`#${anchorId}`}
            className="ml-2 text-muted-foreground text-sm opacity-0 group-hover:opacity-100 transition-opacity"
          >
            #
          </a>
        ) : null}
      </h2>
      <div className="flex items-center gap-2">
        <Badge className="px-2 py-0.5 text-xs font-semibold">{method.toUpperCase()}</Badge>
        <code className="font-mono">{url}</code>
      </div>
    </div>
  );
}

export interface PropertiesProps {
  children?: React.ReactNode;
  className?: string;
}
export function Properties({ children, className }: PropertiesProps) {
  return <div className={cn('divide-y rounded-md border', className)}>{children}</div>;
}

export interface PropertyProps {
  name: string;
  type?: string;
  propKey?: string;
  children?: React.ReactNode;
}
export function Property({ name, type, propKey, children }: PropertyProps) {
  return (
    <div className="p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-medium">{name}</span>
          {type ? <span className="text-muted-foreground text-xs">{type}</span> : null}
        </div>
        {propKey ? <code className="font-mono text-xs">{propKey}</code> : null}
      </div>
      {children ? <div className="mt-2 text-sm text-muted-foreground">{children}</div> : null}
    </div>
  );
}

export interface SubPropertyProps {
  name: string;
  type?: string;
  children?: React.ReactNode;
}
export function SubProperty({ name, type, children }: SubPropertyProps) {
  return (
    <div className="ml-4 rounded-md border p-2">
      <div className="flex items-center gap-2">
        <span className="font-medium">{name}</span>
        {type ? <span className="text-muted-foreground text-xs">{type}</span> : null}
      </div>
      {children ? <div className="mt-1 text-sm text-muted-foreground">{children}</div> : null}
    </div>
  );
}

export interface ParagraphProps extends React.HTMLAttributes<HTMLParagraphElement> {
  children?: React.ReactNode;
  className?: string;
}
export function Paragraph({ children, className, ...props }: ParagraphProps) {
  return (
    <p
      className={cn(
        'my-3 first:mt-0 last:mb-0 leading-7 text-sm text-secondary-foreground break-words',
        className
      )}
      {...props}
    >
      {children}
    </p>
  );
}

// helpers: extract text and create slug
function childrenToText(children: React.ReactNode): string {
  if (children == null) return '';
  if (typeof children === 'string') return children;
  if (typeof children === 'number') return String(children);
  if (Array.isArray(children)) return children.map(childrenToText).join('');
  if (React.isValidElement(children)) return childrenToText(children.props?.children);
  return '';
}

function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[\s\u3000]+/g, '-') // spaces (including full-width)
    .replace(/[^a-z0-9-]/g, '') // keep alphanum and hyphen
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

// Headings (h1-h6): consistent styling, generated slugs, and hover anchors
interface BaseHeadingProps {
  children?: React.ReactNode;
  id?: string;
  className?: string;
}

function Anchor({ id }: { id?: string }) {
  if (!id) return null;
  return (
    <a
      href={`#${id}`}
      className="ml-2 text-muted-foreground text-xs opacity-0 group-hover:opacity-100 transition-opacity"
      aria-label={`Link to ${id}`}
    >
      #
    </a>
  );
}

export function H1({ children, id, className }: BaseHeadingProps) {
  const computedId = id || slugify(childrenToText(children));
  return (
    <h1
      id={computedId}
      className={cn('group scroll-m-20 text-[34px] font-bold tracking-tight', className)}
    >
      {children}
      <Anchor id={computedId} />
    </h1>
  );
}

export function H2({ children, id, className }: BaseHeadingProps) {
  const computedId = id || slugify(childrenToText(children));
  return (
    <h2
      id={computedId}
      className={cn('group scroll-m-20 text-[28px] font-semibold tracking-tight mt-8', className)}
    >
      {children}
      <Anchor id={computedId} />
    </h2>
  );
}

export function H3({ children, id, className }: BaseHeadingProps) {
  const computedId = id || slugify(childrenToText(children));
  return (
    <h3
      id={computedId}
      className={cn('group scroll-m-20 text-[22px] font-semibold tracking-tight mt-6', className)}
    >
      {children}
      <Anchor id={computedId} />
    </h3>
  );
}

export function H4({ children, id, className }: BaseHeadingProps) {
  const computedId = id || slugify(childrenToText(children));
  return (
    <h4
      id={computedId}
      className={cn('group scroll-m-20 text-[18px] font-semibold tracking-tight mt-4', className)}
    >
      {children}
      <Anchor id={computedId} />
    </h4>
  );
}

export function H5({ children, id, className }: BaseHeadingProps) {
  const computedId = id || slugify(childrenToText(children));
  return (
    <h5
      id={computedId}
      className={cn('group scroll-m-20 text-[16px] font-semibold tracking-tight mt-3', className)}
    >
      {children}
      <Anchor id={computedId} />
    </h5>
  );
}

export function H6({ children, id, className }: BaseHeadingProps) {
  const computedId = id || slugify(childrenToText(children));
  return (
    <h6
      id={computedId}
      className={cn('group scroll-m-20 text-[14px] font-bold tracking-tight mt-2', className)}
    >
      {children}
      <Anchor id={computedId} />
    </h6>
  );
}

export interface InlineCodeProps {
  children?: React.ReactNode;
  className?: string;
}
export function InlineCode({ children, className }: InlineCodeProps) {
  const cls = typeof className === 'string' ? className : undefined;
  const hasLanguage = !!cls && /language-/.test(cls);
  const text = String(children ?? '');
  const isMultiline = /\n/.test(text);

  if (hasLanguage || isMultiline) {
    return <code className={cn('bg-muted px-1 py-0.5 text-[85%]', className)}>{children}</code>;
  }

  return (
    <Badge variant="secondary" className={cn('px-1.5 py-0.5 text-xs font-mono', className)}>
      {children}
    </Badge>
  );
}

// List components (ul/ol/li) used by the MDX render map
export interface UlProps {
  children?: React.ReactNode;
  className?: string;
}
export function Ul({ children, className }: UlProps) {
  return (
    <ul className={cn('list-disc pl-6 my-2 space-y-1 marker:text-secondary-foreground', className)}>
      {children}
    </ul>
  );
}

export interface OlProps {
  children?: React.ReactNode;
  className?: string;
}
export function Ol({ children, className }: OlProps) {
  return (
    <ol
      className={cn('list-decimal pl-6 my-2 space-y-1 marker:text-secondary-foreground', className)}
    >
      {children}
    </ol>
  );
}

export interface LiProps {
  children?: React.ReactNode;
  className?: string;
}
export function Li({ children, className }: LiProps) {
  return (
    <li className={cn('leading-6 my-1 text-sm text-secondary-foreground', className)}>
      {children}
    </li>
  );
}

export default {
  Row,
  Col,
  Heading,
  Properties,
  Property,
  SubProperty,
  Paragraph,
  Ul,
  Ol,
  Li,
  InlineCode,
  H1,
  H2,
  H3,
  H4,
  H5,
  H6,
};
