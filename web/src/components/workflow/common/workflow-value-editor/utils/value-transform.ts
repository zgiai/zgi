import type { JSONContent } from '@tiptap/core';

// Token patterns used for converting between string value and TipTap JSON document
// Support both two-part tokens like {{#source.key#}} and single-part special token {{#context#}}
export const TOKEN_REGEX_GLOBAL = /\{\{#([^.#}]+)(?:\.([^#}]+))?#\}\}/g;
export const TOKEN_REGEX_SINGLE = /\{\{#([^.#}]+)(?:\.([^#}]+))?#\}\}/;

const ZGI_BLOCK_REGEX_GLOBAL =
  /<zgi:(slot|knowledge|skill|database|table)\b([^>]*)>([\s\S]*?)<\/zgi:(slot|knowledge|skill|database|table)>/g;
const ZGI_ATTR_REGEX = /([a-zA-Z_][\w-]*)="([^"]*)"/g;

export interface ValueTransformOptions {
  templateBlocksEnabled?: boolean;
}

function decodeTemplateText(value: string): string {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');
}

function encodeTemplateText(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function encodeTemplateAttribute(value: string): string {
  return encodeTemplateText(value).replace(/"/g, '&quot;');
}

function characterLength(value: string): number {
  return Array.from(value).length;
}

function parseZGIAttributes(input: string): Record<string, string> {
  const attrs: Record<string, string> = {};
  input.replace(ZGI_ATTR_REGEX, (_match, key: string, rawValue: string) => {
    attrs[key] = decodeTemplateText(rawValue);
    return _match;
  });
  return attrs;
}

function slotToInlineNode(attrs: Record<string, string>, label: string): JSONContent {
  const name = attrs.name || '';
  const placeholder = attrs.placeholder || '';
  const isEmpty = label.length === 0;

  if (isEmpty) {
    return {
      type: 'templateSlotPlaceholder',
      attrs: {
        name,
        placeholder,
      },
    };
  }

  return {
    type: 'text',
    text: label,
    marks: [
      {
        type: 'templateSlot',
        attrs: {
          name,
          placeholder,
        },
      },
    ],
  };
}

function parseInlineTokens(line: string, options?: ValueTransformOptions): JSONContent[] {
  const contentNodes: JSONContent[] = [];
  let lastIndex = 0;
  const pattern = options?.templateBlocksEnabled ? ZGI_BLOCK_REGEX_GLOBAL : TOKEN_REGEX_GLOBAL;
  pattern.lastIndex = 0;

  line.replace(pattern, (...args: unknown[]) => {
    const match = String(args[0] ?? '');
    const offset = Number(args[args.length - 2] ?? 0);
    if (offset > lastIndex) {
      contentNodes.push({ type: 'text', text: line.slice(lastIndex, offset) });
    }

    if (options?.templateBlocksEnabled) {
      const kind = String(args[1] ?? '');
      const rawAttrs = String(args[2] ?? '');
      const rawContent = String(args[3] ?? '');
      const closingKind = String(args[4] ?? '');
      const attrs = parseZGIAttributes(rawAttrs);
      const label = decodeTemplateText(rawContent);

      if (kind !== closingKind) {
        contentNodes.push({ type: 'text', text: match });
      } else if (kind === 'slot') {
        contentNodes.push(slotToInlineNode(attrs, label));
      } else if (kind === 'knowledge' || kind === 'database' || kind === 'table') {
        contentNodes.push({
          type: 'variableToken',
          attrs: {
            sourceId: kind,
            key: attrs.id || '',
            title: kind,
            label,
            syntax: 'zgi',
          },
        });
      } else if (kind === 'skill') {
        contentNodes.push({
          type: 'variableToken',
          attrs: {
            sourceId: 'skill',
            key: attrs.id || '',
            title: 'Skill',
            label,
            syntax: 'zgi',
          },
        });
      } else {
        contentNodes.push({ type: 'text', text: match });
      }
    } else {
      const sourceId = String(args[1] ?? '');
      const key = typeof args[2] === 'string' ? args[2] : '';
      contentNodes.push({ type: 'variableToken', attrs: { sourceId, key } });
    }

    lastIndex = offset + match.length;
    return match;
  });

  if (lastIndex < line.length) {
    contentNodes.push({ type: 'text', text: line.slice(lastIndex) });
  }

  return contentNodes;
}

// Convert string value (with {{#source.key#}} tokens) to TipTap JSONContent
export function valueToDoc(value: string, options?: ValueTransformOptions): JSONContent {
  // Split by newline to preserve user-intended line breaks as separate paragraphs
  const lines = String(value ?? '').split('\n');

  const paragraphs: JSONContent[] = lines.map(line => {
    const contentNodes = parseInlineTokens(line, options);

    // Avoid producing completely empty paragraphs; keep only when we must preserve at least one line
    // Return an empty paragraph when there is no content so TipTap Placeholder can display
    if (contentNodes.length === 0) {
      return { type: 'paragraph' } as JSONContent;
    }

    return { type: 'paragraph', content: contentNodes } as JSONContent;
  });

  const docContent = paragraphs.length > 0 ? paragraphs : [{ type: 'paragraph' }];

  return { type: 'doc', content: docContent } as JSONContent;
}

export function getTemplateAwareCharacterCount(
  value: string,
  options?: ValueTransformOptions
): number {
  const source = String(value ?? '');
  if (!options?.templateBlocksEnabled) {
    return characterLength(source);
  }

  let count = 0;
  let lastIndex = 0;
  ZGI_BLOCK_REGEX_GLOBAL.lastIndex = 0;
  source.replace(ZGI_BLOCK_REGEX_GLOBAL, (...args: unknown[]) => {
    const match = String(args[0] ?? '');
    const offset = Number(args[args.length - 2] ?? 0);
    if (offset > lastIndex) {
      count += characterLength(source.slice(lastIndex, offset));
    }

    const kind = String(args[1] ?? '');
    const rawContent = String(args[3] ?? '');
    const closingKind = String(args[4] ?? '');
    if (kind !== closingKind) {
      count += characterLength(match);
    } else if (
      kind === 'slot' ||
      kind === 'knowledge' ||
      kind === 'skill' ||
      kind === 'database' ||
      kind === 'table'
    ) {
      count += characterLength(decodeTemplateText(rawContent));
    } else {
      count += characterLength(match);
    }

    lastIndex = offset + match.length;
    return match;
  });

  if (lastIndex < source.length) {
    count += characterLength(source.slice(lastIndex));
  }
  return count;
}

export function stripZGISlotBlocksForPromptOptimization(value: string): string {
  const source = String(value ?? '');
  ZGI_BLOCK_REGEX_GLOBAL.lastIndex = 0;
  return source.replace(ZGI_BLOCK_REGEX_GLOBAL, (...args: unknown[]) => {
    const match = String(args[0] ?? '');
    const kind = String(args[1] ?? '');
    const rawContent = String(args[3] ?? '');
    const closingKind = String(args[4] ?? '');

    if (kind === 'slot' && closingKind === 'slot') {
      return decodeTemplateText(rawContent);
    }

    return match;
  });
}

// Convert TipTap JSONContent back to string value with tokens
export function docToValue(json: JSONContent): string {
  try {
    const paragraphs = (json?.content as JSONContent[] | undefined) ?? [];

    const serializeNode = (node: JSONContent): string => {
      if (!node) return '';
      if (node.type === 'variableToken') {
        const sourceId = String(
          (node.attrs as { sourceId?: string; key?: string; syntax?: string })?.sourceId ?? ''
        );
        const key = String(
          (node.attrs as { sourceId?: string; key?: string; syntax?: string })?.key ?? ''
        );
        const syntax = String((node.attrs as { syntax?: string })?.syntax ?? '');
        const label = String((node.attrs as { label?: string })?.label ?? '');
        if (
          syntax === 'zgi' &&
          (sourceId === 'knowledge' ||
            sourceId === 'skill' ||
            sourceId === 'database' ||
            sourceId === 'table')
        ) {
          return `<zgi:${sourceId} id="${encodeTemplateAttribute(key)}">${encodeTemplateText(label)}</zgi:${sourceId}>`;
        }
        // For special single-part token (e.g., {{#context#}}) we store key as empty string
        if (key === '') return `{{#${sourceId}#}}`;
        return `{{#${sourceId}.${key}#}}`;
      }
      if (node.type === 'templateSlot') {
        const name = String((node.attrs as { name?: string; placeholder?: string })?.name ?? '');
        const placeholder = String(
          (node.attrs as { name?: string; placeholder?: string })?.placeholder ?? ''
        );
        const inner = ((node.content as JSONContent[] | undefined) ?? [])
          .map(serializeNode)
          .join('');
        return `<zgi:slot name="${encodeTemplateAttribute(name)}" placeholder="${encodeTemplateAttribute(placeholder)}">${encodeTemplateText(inner)}</zgi:slot>`;
      }
      if (node.type === 'templateSlotPlaceholder') {
        const name = String((node.attrs as { name?: string; placeholder?: string })?.name ?? '');
        const placeholder = String(
          (node.attrs as { name?: string; placeholder?: string })?.placeholder ?? ''
        );
        return `<zgi:slot name="${encodeTemplateAttribute(name)}" placeholder="${encodeTemplateAttribute(placeholder)}"></zgi:slot>`;
      }
      if (node.type === 'hardBreak') {
        return '\n';
      }
      if (typeof node.text === 'string') {
        const templateSlotMark = ((node.marks as JSONContent[] | undefined) ?? []).find(
          mark => mark.type === 'templateSlot'
        );
        if (templateSlotMark) {
          const attrs =
            (templateSlotMark.attrs as {
              name?: string;
              placeholder?: string;
            }) ?? {};
          const name = String(attrs.name ?? '');
          const placeholder = String(attrs.placeholder ?? '');

          return `<zgi:slot name="${encodeTemplateAttribute(name)}" placeholder="${encodeTemplateAttribute(placeholder)}">${encodeTemplateText(node.text)}</zgi:slot>`;
        }
        return node.text;
      }
      // Recursively handle any nested content arrays (defensive)
      const inner = (node.content as JSONContent[] | undefined) ?? [];
      return inner.map(serializeNode).join('');
    };

    const parts: string[] = [];
    for (let i = 0; i < paragraphs.length; i++) {
      const p = paragraphs[i];
      const pContent = (p?.content as JSONContent[] | undefined) ?? [];
      parts.push(pContent.map(serializeNode).join(''));
      // Insert a newline between paragraphs (TipTap uses paragraphs for Enter)
      if (i < paragraphs.length - 1) parts.push('\n');
    }
    return parts.join('');
  } catch {
    return '';
  }
}
