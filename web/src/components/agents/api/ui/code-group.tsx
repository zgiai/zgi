'use client';

import React, { useRef, useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { Copy, Check } from 'lucide-react';
import hljs from 'highlight.js';
import 'highlight.js/styles/atom-one-dark.css';
import { useApiDocs } from './api-docs-context';

export interface CodeGroupProps {
  title?: string;
  tag?: string; // HTTP method, e.g. GET/POST
  label?: string; // endpoint label, e.g. /workflows/run
  targetCode?: string; // copy this code instead of children text
  children?: React.ReactNode;
  className?: string;
}

function methodClass(method?: string) {
  switch ((method || '').toUpperCase()) {
    case 'GET':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300';
    case 'POST':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300';
    case 'PUT':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300';
    case 'DELETE':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300';
    case 'PATCH':
      return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300';
    default:
      return 'bg-muted text-foreground';
  }
}

export function CodeGroup({ title, tag, label, targetCode, children, className }: CodeGroupProps) {
  const contentRef = useRef<HTMLDivElement>(null);
  const [copied, setCopied] = useState(false);
  const { apibase } = useApiDocs();

  // Replace {apibase} placeholder with actual value
  const replaceApibase = (text: string): string => text.replace(/\{apibase\}/g, apibase);

  // Replace {apibase} placeholder and apply syntax highlighting
  useEffect(() => {
    const root = contentRef.current;
    if (!root) return;
    const codes = root.querySelectorAll('pre code');
    codes.forEach(el => {
      try {
        const codeEl = el as HTMLElement;
        // Replace {apibase} placeholder in code text
        if (apibase && codeEl.textContent?.includes('{apibase}')) {
          codeEl.textContent = codeEl.textContent.replace(/\{apibase\}/g, apibase);
        }
        // Only highlight when language-xxx class exists
        const hasLanguage = Array.from(codeEl.classList).some(c => c.startsWith('language-'));
        if (!hasLanguage) return;
        hljs.highlightElement(codeEl);
      } catch {
        // ignore highlight errors
      }
    });
  }, [children, apibase]);

  const onCopy = async () => {
    let text = targetCode || '';
    if (!text) {
      const code = contentRef.current?.querySelector('pre code');
      text = code?.textContent || '';
    }
    text = replaceApibase(text);
    if (text) {
      try {
        await navigator.clipboard.writeText(text);
        setCopied(true);
        const timer = setTimeout(() => setCopied(false), 1500);
        return () => clearTimeout(timer);
      } catch {
        // ignore clipboard errors
      }
    }
  };

  return (
    <div className={cn('rounded-lg border bg-background shadow-sm max-w-3xl my-2', className)}>
      <div className="flex items-center justify-between gap-2 border-b px-2 py-1 bg-muted">
        <div className="flex items-center gap-2">
          {tag ? (
            <Badge className={cn('px-2 py-0.5 text-xs font-semibold border-0', methodClass(tag))}>
              {tag.toUpperCase()}
            </Badge>
          ) : null}
          {label ? <span className="font-mono text-xs sm:text-sm">{label}</span> : null}
        </div>
        <div className="flex items-center gap-2">
          {title ? <span className="text-xs font-medium">{title}</span> : null}
          <Button
            variant="ghost"
            isIcon
            className="w-7 h-7"
            onClick={onCopy}
            aria-label="Copy"
          >
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
          </Button>
        </div>
      </div>
      <div ref={contentRef} className="overflow-auto rounded-b-lg">
        {children}
      </div>
    </div>
  );
}

export default CodeGroup;
