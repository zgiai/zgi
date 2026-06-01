import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import rehypeHighlight from 'rehype-highlight';
import rehypeRaw from 'rehype-raw';
import rehypeKatex from 'rehype-katex';
import type { PluggableList } from 'unified';
import 'highlight.js/styles/github.css';
import 'katex/dist/katex.min.css';
import { Button } from '@/components/ui/button';
import { Copy, Check, ExternalLink } from 'lucide-react';
import { API_URL } from '@/lib/config';
import { cn } from '@/lib/utils';
import { isVoidElement, safeCloneElement } from '@/utils/dom';
import { normalizeMarkdownMathDelimiters } from '@/utils/markdown';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n/translations';
import { transformThinkTags } from '@/components/common/markdown-think-block';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  TableCaption,
} from '@/components/ui/table';
import { Checkbox } from '@/components/ui/checkbox';
import { MarkdownMermaid } from '@/components/common/markdown-mermaid';

interface MarkdownViewerProps {
  content: string;
  className?: string;
  /** Render raw HTML embedded in Markdown. Disable for public or untrusted content. */
  allowRawHtml?: boolean;
  /** Optional highlight terms. Matching occurrences will be highlighted. */
  highlights?: string[];
  /** Whether the rendered message is still receiving streamed content. */
  isStreaming?: boolean;
  /** Stable identity used for streamed block render caches. */
  renderIdentity?: string;
}

import { MarkdownImage } from '@/components/common/markdown-image';

const remarkPluginsList: PluggableList = [remarkGfm, remarkMath];
const rehypePluginsList: PluggableList = [
  [rehypeHighlight, { ignoreMissing: true }],
  rehypeRaw,
  rehypeKatex,
];
const safeRehypePluginsList: PluggableList = [
  [rehypeHighlight, { ignoreMissing: true }],
  rehypeKatex,
];

function hasInlineFlag(x: unknown): x is { inline: boolean } {
  return !!x && typeof (x as { inline?: unknown }).inline === 'boolean';
}

function getClassName(x: unknown): string | undefined {
  const cls = (x as { className?: unknown })?.className;
  return typeof cls === 'string' ? cls : undefined;
}

// Type guard for nodes that carry a literal string value (mdast code/inlineCode)
function hasLiteralValue(x: unknown): x is { value: string } {
  return !!x && typeof (x as { value?: unknown }).value === 'string';
}

// Safely extract the original code text from nodeProps.node if available
function extractNodeValue(nodeProps: unknown): string | null {
  const node = (nodeProps as { node?: unknown })?.node;
  if (hasLiteralValue(node)) return node.value;
  return null;
}

// Type guard: GFM task list item checked flag
function hasChecked(x: unknown): x is { checked: boolean | null } {
  return !!x && typeof (x as { checked?: unknown }).checked !== 'undefined';
}

// Recursively collect all text from a ReactNode tree
function collectText(node: React.ReactNode): string {
  if (node == null) return '';
  if (typeof node === 'string') return node;
  if (Array.isArray(node)) return node.map(collectText).join('');
  if (React.isValidElement(node)) {
    const child = (node.props as { children?: React.ReactNode }).children;
    return collectText(child);
  }
  return '';
}

// Escape special regex characters
function escapeRegex(input: string): string {
  return input.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function isLatinLike(input: string): boolean {
  return /[A-Za-z0-9_]/.test(input);
}

function buildUnionRegex(terms: string[]): RegExp | null {
  const cleaned = Array.from(new Set(terms.map(t => t.trim()).filter(t => t.length > 0)));
  if (cleaned.length === 0) return null;
  const alt = cleaned
    .map(t => {
      const escaped = escapeRegex(t);
      return isLatinLike(t) ? `\\b${escaped}\\b` : escaped;
    })
    .join('|');
  try {
    return new RegExp(alt, 'gi');
  } catch {
    return null;
  }
}

function getScrollContainer(el: HTMLElement | null): HTMLElement | null {
  let cur: HTMLElement | null = el;
  while (cur && cur !== document.body) {
    const style = getComputedStyle(cur);
    const overflowY = style.overflowY || style.overflow;
    const canScroll = /(auto|scroll)/.test(overflowY);
    if (canScroll && cur.scrollHeight > cur.clientHeight) return cur;
    cur = cur.parentElement;
  }
  return null;
}

function escapeSelector(value: string): string {
  if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') {
    return CSS.escape(value);
  }

  return value.replace(/([ !"#$%&'()*+,./:;<=>?@[\\\]^`{|}~])/g, '\\$1');
}

function isFootnoteAnchorId(value: string): boolean {
  return /^(?:user-content-)?fn(?:ref)?-\d+$/.test(value) || value === 'footnote-label';
}

function normalizeAnchorId(value: string): string {
  return isFootnoteAnchorId(value) ? `md-${value}` : value;
}

function normalizeHashHref(href: string): string {
  if (!href.startsWith('#')) return href;
  const hash = decodeURIComponent(href.slice(1));
  return `#${normalizeAnchorId(hash)}`;
}

const MINERU_IMAGE_ENDPOINT = '/console/api/files/mineru-images';
const MARKDOWN_IMAGE_PATTERN = /!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)/g;
const LOCAL_IMAGE_EXTENSION_PATTERN = /\.(?:png|jpe?g|gif|webp|bmp|svg)$/i;

function normalizeApiBaseUrl(): string {
  return API_URL.replace(/\/+$/, '');
}

function stripMarkdownLinkBrackets(value: string): string {
  const trimmed = value.trim();
  if (trimmed.startsWith('<') && trimmed.endsWith('>')) {
    return trimmed.slice(1, -1).trim();
  }
  return trimmed;
}

function isWindowsAbsolutePath(value: string): boolean {
  return /^[A-Za-z]:[\\/]/.test(value);
}

function isMinerULocalImagePath(value: string): boolean {
  const normalized = value.replace(/\\/g, '/').toLowerCase();
  return (
    normalized.includes('/mineru/images/') ||
    normalized.includes('/hyperparse/mineru/images/') ||
    normalized.includes('/storage/mineru/images/')
  );
}

function hasLocalImageExtension(value: string): boolean {
  const pathOnly = value.split(/[?#]/, 1)[0];
  return LOCAL_IMAGE_EXTENSION_PATTERN.test(pathOnly);
}

function buildMinerUImageUrl(localPath: string): string {
  return `${normalizeApiBaseUrl()}${MINERU_IMAGE_ENDPOINT}?path=${encodeURIComponent(localPath)}`;
}

function normalizeMinerUImageSource(src: string): string {
  const value = stripMarkdownLinkBrackets(src);
  if (!value) return src;

  if (value.startsWith(MINERU_IMAGE_ENDPOINT)) {
    return `${normalizeApiBaseUrl()}${value}`;
  }

  if (
    (isWindowsAbsolutePath(value) || isMinerULocalImagePath(value)) &&
    hasLocalImageExtension(value)
  ) {
    return buildMinerUImageUrl(value);
  }

  return src;
}

function rewriteMinerUImageSources(markdown: string): string {
  return markdown.replace(MARKDOWN_IMAGE_PATTERN, (match, alt: string, rawSrc: string) => {
    const normalizedSrc = normalizeMinerUImageSource(rawSrc);
    if (normalizedSrc === rawSrc) return match;
    return `![${alt}](${normalizedSrc})`;
  });
}

interface MermaidFenceBlock {
  closed: boolean;
}

interface FenceState {
  char: '`' | '~';
  length: number;
  isMermaid: boolean;
}

function parseFenceOpen(line: string): FenceState | null {
  const match = /^(?: {0,3})(`{3,}|~{3,})(.*)$/.exec(line);
  if (!match) return null;
  const marker = match[1];
  const char = marker[0] as '`' | '~';
  const info = match[2].trim();
  if (char === '`' && info.includes('`')) return null;
  const firstToken = info.split(/\s+/, 1)[0]?.replace(/^[{.]+|[}]$/g, '').toLowerCase() ?? '';
  return {
    char,
    length: marker.length,
    isMermaid: firstToken === 'mermaid',
  };
}

function isFenceClose(line: string, fence: FenceState): boolean {
  const match = /^(?: {0,3})(`{3,}|~{3,})(?:[ \t]*)$/.exec(line);
  if (!match) return false;
  const marker = match[1];
  return marker[0] === fence.char && marker.length >= fence.length;
}

function collectMermaidFenceBlocks(markdown: string): MermaidFenceBlock[] {
  const blocks: MermaidFenceBlock[] = [];
  const lines = markdown.split(/\r?\n/);
  let fence: FenceState | null = null;

  for (const line of lines) {
    if (fence) {
      if (isFenceClose(line, fence)) {
        if (fence.isMermaid) blocks.push({ closed: true });
        fence = null;
      }
      continue;
    }

    fence = parseFenceOpen(line);
  }

  if (fence?.isMermaid) {
    blocks.push({ closed: false });
  }

  return blocks;
}

function hashString(value: string): string {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(36);
}

function cacheIdentityPart(value: string): string {
  return hashString(value || 'markdown-viewer');
}

const MarkdownViewer: React.FC<MarkdownViewerProps> = ({
  content,
  className,
  allowRawHtml = true,
  highlights,
  isStreaming = false,
  renderIdentity,
}) => {
  const [copiedText, setCopiedText] = React.useState<string | null>(null);
  const t = useT('webapp');
  const thinkSummary = t('chat.thoughtProcess');
  const viewerRef = React.useRef<HTMLDivElement | null>(null);
  const viewerInstanceId = React.useId();

  // Preprocess content to prevent markdown parser from breaking SVG blocks with blank lines
  const processedContent = React.useMemo(() => {
    if (!content) return '';

    // Helper to compress SVG content by removing newlines to prevent markdown parser fragmentation
    const compressSvg = (str: string) => {
      return str
        .replace(/>\s*[\r\n]+\s*</g, '><') // Remove newlines between tags
        .replace(/[\r\n]+/g, ' '); // Replace remaining newlines (e.g. in attributes) with spaces
    };

    let newContent = rewriteMinerUImageSources(content);

    // 1. Handle complete SVG blocks
    newContent = newContent.replace(/<svg[\s\S]*?<\/svg>/gi, match => compressSvg(match));

    // 2. Handle incomplete SVG block at the end (for streaming)
    const lastSvgIndex = newContent.lastIndexOf('<svg');
    const lastCloseSvgIndex = newContent.lastIndexOf('</svg>');

    // If there is an <svg> that hasn't been closed yet
    if (lastSvgIndex > -1 && lastSvgIndex > lastCloseSvgIndex) {
      const before = newContent.substring(0, lastSvgIndex);
      const openSvgPart = newContent.substring(lastSvgIndex);
      newContent = before + compressSvg(openSvgPart);
    }

    newContent = normalizeMarkdownMathDelimiters(newContent);
    newContent = transformThinkTags(newContent, thinkSummary);

    return newContent;
  }, [content, thinkSummary]);

  const mermaidFenceBlocks = React.useMemo(
    () => collectMermaidFenceBlocks(processedContent),
    [processedContent]
  );
  const markdownRenderIdentity = React.useMemo(
    () => cacheIdentityPart(renderIdentity || viewerInstanceId),
    [renderIdentity, viewerInstanceId]
  );

  const regex = React.useMemo(() => buildUnionRegex(highlights ?? []), [highlights]);
  const highlightClass =
    'inline rounded px-[2px] bg-highlight/5 text-highlight ring-1 ring-highlight whitespace-nowrap ';

  const handleCopy = React.useCallback(async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedText(text);
      window.setTimeout(() => setCopiedText(null), 1200);
    } catch {
      // Ignore clipboard errors
    }
  }, []);

  // Highlight a plain text string using the union regex
  const renderHighlightedText = React.useCallback(
    (text: string): React.ReactNode => {
      if (!regex) return text;
      const parts: React.ReactNode[] = [];
      let lastIndex = 0;
      const src = text;
      // Use while to collect matches sequentially
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      let match: RegExpExecArray | null;
      while ((match = regex.exec(src)) !== null) {
        const start = match.index;
        const end = regex.lastIndex;
        if (start > lastIndex) {
          parts.push(src.slice(lastIndex, start));
        }
        parts.push(
          <span key={`hl-${start}-${end}`} className={highlightClass}>
            {src.slice(start, end)}
          </span>
        );
        lastIndex = end;
      }
      if (lastIndex < src.length) {
        parts.push(src.slice(lastIndex));
      }
      return parts.length > 0 ? parts : text;
    },
    [regex]
  );

  // Recursively apply highlight to children, preserving element structure
  const renderChildrenWithHighlights = React.useCallback(
    (children: React.ReactNode): React.ReactNode => {
      const arr = React.Children.toArray(children);
      return arr.map((child, idx) => {
        if (typeof child === 'string') {
          return <React.Fragment key={`txt-${idx}`}>{renderHighlightedText(child)}</React.Fragment>;
        }
        if (React.isValidElement(child)) {
          if (isVoidElement(child.type)) {
            return safeCloneElement(child, child.key ?? `el-${idx}`);
          }
          const childChildren = (child.props as { children?: React.ReactNode }).children;
          if (childChildren == null) {
            return safeCloneElement(child, child.key ?? `el-${idx}`);
          }
          const nested = renderChildrenWithHighlights(childChildren);
          return safeCloneElement(child, child.key ?? `el-${idx}`, nested);
        }
        return child;
      });
    },
    [renderHighlightedText]
  );

  const handleHashAnchorClick = React.useCallback(
    (event: React.MouseEvent<HTMLAnchorElement>, href: string) => {
      const hash = href.trim();
      if (!hash.startsWith('#') || hash.length <= 1) return;

      const targetId = decodeURIComponent(hash.slice(1));
      const viewer = viewerRef.current;
      const target =
        viewer?.querySelector<HTMLElement>(`#${escapeSelector(targetId)}`) ??
        document.getElementById(targetId);

      if (!target) return;

      event.preventDefault();

      const container =
        getScrollContainer(event.currentTarget) ??
        getScrollContainer(viewer) ??
        document.scrollingElement;

      if (container instanceof HTMLElement && container.contains(target)) {
        const containerTop = container.getBoundingClientRect().top;
        const targetTop = target.getBoundingClientRect().top - containerTop + container.scrollTop;
        const nextTop = Math.max(0, targetTop - 16);
        container.scrollTo({ top: nextTop, behavior: 'smooth' });
      } else {
        target.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      }

      target.setAttribute('tabindex', '-1');
      target.focus({ preventScroll: true });
    },
    []
  );

  const renderHeading = React.useCallback(
    (Tag: 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6') =>
      function Heading({
        children,
        id,
        ...rest
      }: React.HTMLAttributes<HTMLHeadingElement> & { id?: string }) {
        const normalizedId = typeof id === 'string' ? normalizeAnchorId(id) : id;
        return (
          <Tag id={normalizedId} {...rest}>
            {renderChildrenWithHighlights(children)}
          </Tag>
        );
      },
    [renderChildrenWithHighlights]
  );

  let mermaidCodeBlockIndex = 0;

  return (
    <div ref={viewerRef} className={cn('md-viewer', className)}>
      <ReactMarkdown
        remarkPlugins={remarkPluginsList}
        rehypePlugins={allowRawHtml ? rehypePluginsList : safeRehypePluginsList}
        components={{
          h1: renderHeading('h1'),
          h2: renderHeading('h2'),
          h3: renderHeading('h3'),
          h4: renderHeading('h4'),
          h5: renderHeading('h5'),
          h6: renderHeading('h6'),
          // Links - open in new tab for external links
          a({ children, href, id, ...rest }) {
            const isExternal = href?.startsWith('http') || href?.startsWith('//');
            const normalizedHref = href?.startsWith('#') ? normalizeHashHref(href) : href;
            const isHashLink = normalizedHref?.startsWith('#');
            const normalizedId = typeof id === 'string' ? normalizeAnchorId(id) : id;
            return (
              <a
                href={normalizedHref}
                id={normalizedId}
                target={isExternal ? '_blank' : undefined}
                rel={isExternal ? 'noopener noreferrer' : undefined}
                onClick={event => {
                  if (isHashLink && normalizedHref) {
                    handleHashAnchorClick(event, normalizedHref);
                  }
                }}
                {...rest}
              >
                {renderChildrenWithHighlights(children)}
                {isExternal && <ExternalLink className="inline-block ml-1 w-3 h-3" />}
              </a>
            );
          },
          // Tables
          table({ children, ...rest }) {
            const cls = getClassName(rest);
            return (
              <div className="my-3 max-w-full min-w-0 overflow-x-auto rounded-lg border border-border">
                <Table className={cn('min-w-full text-sm', cls)}>{children}</Table>
              </div>
            );
          },
          thead({ children, ...rest }) {
            const cls = getClassName(rest);
            return <TableHeader className={cls}>{children}</TableHeader>;
          },
          tbody({ children, ...rest }) {
            const cls = getClassName(rest);
            return <TableBody className={cls}>{children}</TableBody>;
          },
          tr({ children, ...rest }) {
            const cls = getClassName(rest);
            return <TableRow className={cls}>{children}</TableRow>;
          },
          th({ children, ...rest }) {
            const cls = getClassName(rest);
            return <TableHead className={cls}>{renderChildrenWithHighlights(children)}</TableHead>;
          },
          td({ children, ...rest }) {
            const cls = getClassName(rest);
            return <TableCell className={cls}>{renderChildrenWithHighlights(children)}</TableCell>;
          },
          caption({ children, ...rest }) {
            const cls = getClassName(rest);
            return <TableCaption className={cls}>{children}</TableCaption>;
          },
          // Lists
          ul({ children, ...rest }) {
            const cls = getClassName(rest);
            return (
              <ul {...rest} className={cn('my-2 ml-5 list-disc space-y-1', cls)}>
                {children}
              </ul>
            );
          },
          ol({ children, ...rest }) {
            const cls = getClassName(rest);
            return (
              <ol {...rest} className={cn('my-2 ml-5 list-decimal space-y-1', cls)}>
                {children}
              </ol>
            );
          },
          br({ children: _c, ...rest }) {
            return <br {...rest} />;
          },
          hr({ children: _c, ...rest }) {
            return <hr {...rest} />;
          },
          pre({ children }) {
            return <>{children}</>;
          },
          img({ children: _c, ...rest }) {
            const { src, alt, className, ...props } = rest;
            return (
              <MarkdownImage
                src={src as string}
                alt={alt as string}
                className={className}
                {...props}
              />
            );
          },
          code(nodeProps) {
            const isInline = hasInlineFlag(nodeProps) ? nodeProps.inline : false;
            const codeClassName = getClassName(nodeProps);
            const children = (nodeProps?.children ?? []) as React.ReactNode;
            const textSource = extractNodeValue(nodeProps);
            const text = textSource ?? collectText(children);

            if (isInline) {
              const hasLang = !!(codeClassName && /language-([\w-]+)/.test(codeClassName));
              if (!hasLang) {
                return (
                  <Badge
                    variant="secondary"
                    className={cn(
                      'max-w-full whitespace-normal break-words align-[2px] font-mono text-[85%]',
                      codeClassName
                    )}
                  >
                    {text}
                  </Badge>
                );
              }
              return (
                <code
                  className={cn(
                    'max-w-full break-words rounded bg-muted px-1 py-0.5 text-[85%]',
                    codeClassName
                  )}
                >
                  {children}
                </code>
              );
            }

            const langMatch = /language-([\w-]+)/.exec(codeClassName || '');
            const language = langMatch?.[1]?.toLowerCase();
            const raw = text.replace(/\n$/, '');
            const isCopied = copiedText === raw;
            function renderCodeBlock(label: string) {
              return (
                <div className="group my-2 max-w-full min-w-0 overflow-hidden md-code-wrapper">
                  <div className="md-code-header">
                    <div className="md-code-lang">{label}</div>
                    <Button
                      type="button"
                      variant="ghost"
                      isIcon
                      aria-label="Copy code"
                      className={cn('h-6 w-6 copy-btn')}
                      onClick={() => handleCopy(raw)}
                    >
                      {isCopied ? (
                        <Check className="text-success" size={12} />
                      ) : (
                        <Copy className="text-foreground" size={12} />
                      )}
                    </Button>
                  </div>
                  <pre className="max-w-full overflow-x-auto overflow-y-hidden text-xs">
                    <code className={cn('block min-w-full w-max whitespace-pre', codeClassName)}>
                      {children}
                    </code>
                  </pre>
                </div>
              );
            }

            // Heuristic: language-less, single-line short code blocks render as a compact chip
            if (!langMatch && raw.indexOf('\n') === -1 && raw.length <= 80) {
              return (
                <span
                  className={cn(
                    'align-[2px] font-mono text-xs rounded bg-muted border px-0.5 py-0.5'
                  )}
                >
                  {raw}
                </span>
              );
            }
            if (language === 'mermaid') {
              const blockIndex = mermaidCodeBlockIndex;
              mermaidCodeBlockIndex += 1;
              const block = mermaidFenceBlocks[blockIndex];
              if (isStreaming && block?.closed === false) {
                return renderCodeBlock(langMatch?.[1] || 'mermaid');
              }
              return (
                <MarkdownMermaid
                  chart={raw}
                  cacheKey={`${markdownRenderIdentity}:mermaid:${blockIndex}:${hashString(raw)}`}
                />
              );
            }
            return renderCodeBlock(langMatch?.[1] || 'code');
          },
          // Apply highlights to common text containers; skip code blocks to avoid noise
          p({ children, node, ...rest }) {
            // Check if paragraph contains an image element in its hast node children
            const hasImage = node?.children?.some(
              child => child.type === 'element' && child.tagName === 'img'
            );

            if (hasImage) {
              return (
                <div {...rest} className={cn('relative my-4', rest.className)}>
                  {renderChildrenWithHighlights(children)}
                </div>
              );
            }

            return <p {...rest}>{renderChildrenWithHighlights(children)}</p>;
          },
          li({ children, id, ...rest }) {
            const cls = getClassName(rest);
            const normalizedId = typeof id === 'string' ? normalizeAnchorId(id) : id;
            const isTask = hasChecked(rest) && typeof rest.checked === 'boolean';
            if (isTask) {
              const checked = (rest as { checked: boolean }).checked;
              return (
                <li {...rest} id={normalizedId} className={cn('list-none my-1', cls)}>
                  <div className="flex items-start gap-2">
                    <Checkbox checked={checked} disabled className="translate-y-0.5" />
                    <div className="min-w-0">{renderChildrenWithHighlights(children)}</div>
                  </div>
                </li>
              );
            }
            return (
              <li {...rest} id={normalizedId} className={cn('my-1', cls)}>
                {renderChildrenWithHighlights(children)}
              </li>
            );
          },
          blockquote({ children, ...rest }) {
            return <blockquote {...rest}>{renderChildrenWithHighlights(children)}</blockquote>;
          },
          em({ children, ...rest }) {
            return <em {...rest}>{renderChildrenWithHighlights(children)}</em>;
          },
          strong({ children, ...rest }) {
            return <strong {...rest}>{renderChildrenWithHighlights(children)}</strong>;
          },
        }}
      >
        {processedContent}
      </ReactMarkdown>
    </div>
  );
};

export default MarkdownViewer;
