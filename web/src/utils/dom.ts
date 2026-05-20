import React from 'react';

export const VOID_TAGS = new Set<keyof JSX.IntrinsicElements>([
  'area',
  'base',
  'br',
  'col',
  'embed',
  'hr',
  'img',
  'input',
  'link',
  'meta',
  'param',
  'source',
  'track',
  'wbr',
]);

export function isVoidElement(type: unknown): type is keyof JSX.IntrinsicElements {
  return typeof type === 'string' && VOID_TAGS.has(type as keyof JSX.IntrinsicElements);
}

export function safeCloneElement(
  el: React.ReactElement<unknown>,
  key: React.Key | undefined,
  nextChildren?: React.ReactNode
): React.ReactElement<unknown> {
  if (isVoidElement(el.type)) {
    return React.cloneElement(el, { key }) as React.ReactElement<unknown>;
  }
  if (typeof nextChildren === 'undefined') {
    return React.cloneElement(el, { key }) as React.ReactElement<unknown>;
  }
  return React.cloneElement(el, { key }, nextChildren) as React.ReactElement<unknown>;
}
