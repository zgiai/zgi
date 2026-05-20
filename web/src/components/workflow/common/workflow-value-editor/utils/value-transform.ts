import type { JSONContent } from '@tiptap/core';

// Token patterns used for converting between string value and TipTap JSON document
// Support both two-part tokens like {{#source.key#}} and single-part special token {{#context#}}
export const TOKEN_REGEX_GLOBAL = /\{\{#([^.#}]+)(?:\.([^#}]+))?#\}\}/g;
export const TOKEN_REGEX_SINGLE = /\{\{#([^.#}]+)(?:\.([^#}]+))?#\}\}/;

// Convert string value (with {{#source.key#}} tokens) to TipTap JSONContent
export function valueToDoc(value: string): JSONContent {
  // Split by newline to preserve user-intended line breaks as separate paragraphs
  const lines = String(value ?? '').split('\n');

  const paragraphs: JSONContent[] = lines.map(line => {
    const contentNodes: JSONContent[] = [];
    let lastIndex = 0;

    line.replace(
      TOKEN_REGEX_GLOBAL,
      (match, sourceId: string, key: string | undefined, offset: number) => {
        if (offset > lastIndex) {
          contentNodes.push({ type: 'text', text: line.slice(lastIndex, offset) });
        }
        contentNodes.push({ type: 'variableToken', attrs: { sourceId, key: key ?? '' } });
        lastIndex = offset + match.length;
        return match;
      }
    );

    if (lastIndex < line.length) {
      contentNodes.push({ type: 'text', text: line.slice(lastIndex) });
    }

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

// Convert TipTap JSONContent back to string value with tokens
export function docToValue(json: JSONContent): string {
  try {
    const paragraphs = (json?.content as JSONContent[] | undefined) ?? [];

    const serializeNode = (node: JSONContent): string => {
      if (!node) return '';
      if (node.type === 'variableToken') {
        const sourceId = String(
          (node.attrs as { sourceId?: string; key?: string })?.sourceId ?? ''
        );
        const key = String((node.attrs as { sourceId?: string; key?: string })?.key ?? '');
        // For special single-part token (e.g., {{#context#}}) we store key as empty string
        if (key === '') return `{{#${sourceId}#}}`;
        return `{{#${sourceId}.${key}#}}`;
      }
      if (node.type === 'hardBreak') {
        return '\n';
      }
      if (typeof node.text === 'string') {
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
