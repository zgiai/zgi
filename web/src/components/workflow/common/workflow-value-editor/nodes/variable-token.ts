import { Node as TiptapNode, mergeAttributes } from '@tiptap/core';
import { TOKEN_REGEX_SINGLE } from '../utils/value-transform';

// TipTap node representing an atomic variable token
const VariableToken = TiptapNode.create<{
  sourceId: string;
  key: string;
  title?: string; // node title for display
  label?: string; // human readable token label
  invalid?: boolean;
  syntax?: string;
}>({
  name: 'variableToken',
  inline: true,
  group: 'inline',
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      sourceId: {
        default: '',
        renderHTML: () => ({}),
        parseHTML: element =>
          (element as HTMLElement).getAttribute('data-token')?.match(TOKEN_REGEX_SINGLE)?.[1] ?? '',
      },
      key: {
        default: '',
        renderHTML: () => ({}),
        parseHTML: element =>
          (element as HTMLElement).getAttribute('data-token')?.match(TOKEN_REGEX_SINGLE)?.[2] ?? '',
      },
      title: {
        default: '',
        renderHTML: () => ({}),
        parseHTML: element => (element as HTMLElement).getAttribute('data-title') ?? '',
      },
      label: {
        default: '',
        renderHTML: () => ({}),
        parseHTML: element => (element as HTMLElement).getAttribute('data-label') ?? '',
      },
      invalid: {
        default: false,
        renderHTML: () => ({}),
        parseHTML: element =>
          ((element as HTMLElement).getAttribute('aria-invalid') ?? '') === 'true',
      },
      syntax: {
        default: '',
        renderHTML: () => ({}),
        parseHTML: element => (element as HTMLElement).getAttribute('data-syntax') ?? '',
      },
    };
  },

  parseHTML() {
    return [
      {
        tag: 'span[data-token]',
        getAttrs: element => {
          const token = (element as HTMLElement).getAttribute('data-token') || '';
          const m = token.match(TOKEN_REGEX_SINGLE);
          if (!m) return false;
          return { sourceId: m[1], key: m[2] };
        },
      },
    ];
  },

  renderHTML({ node, HTMLAttributes }) {
    const token = node.attrs.key
      ? `{{#${node.attrs.sourceId}.${node.attrs.key}#}}`
      : `{{#${node.attrs.sourceId}#}}`;

    const sourceId = (node.attrs.sourceId as string) || '';
    const title = (node.attrs.title as string) || sourceId || '';
    const displayKey =
      (node.attrs.label as string) ||
      (node.attrs.key as string) ||
      (sourceId === 'context' ? 'context' : '');

    // Adopt Badge secondary style classes for consistent visuals
    const badgeBase =
      'max-w-full inline-flex items-center justify-center rounded-[4px] border px-2 py-0.5 text-xs font-medium w-fit whitespace-nowrap shrink-0 [&>svg]:size-3 gap-1 [&>svg]:pointer-events-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive transition-[color,box-shadow] overflow-hidden';
    const badgeSecondary = 'border-border bg-background text-secondary-foreground';

    const children: Array<string | Array<string | Record<string, string> | string[]>> = [
      ['span', { class: 'truncate', title }, title],
    ];
    if (displayKey) {
      children.push(['span', { class: 'text-xs text-highlight' }, `(${displayKey})`]);
    }

    return [
      'span',
      mergeAttributes(HTMLAttributes, {
        'data-token': token,
        'data-title': title,
        'data-label': node.attrs.label || undefined,
        'data-syntax': node.attrs.syntax || undefined,
        contenteditable: 'false',
        class: `${badgeBase} ${badgeSecondary} mx-0.5 align-baseline ${node.attrs.invalid ? 'border-destructive' : ''}`,
        'aria-invalid': node.attrs.invalid ? 'true' : undefined,
      }),
      ...children,
    ];
  },
});

export default VariableToken;
