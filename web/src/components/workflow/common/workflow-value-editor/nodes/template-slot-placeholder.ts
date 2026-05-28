import { Node as TiptapNode, mergeAttributes } from '@tiptap/core';
import type { Node as ProseMirrorNode } from '@tiptap/pm/model';
import { Plugin, TextSelection } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

const placeholderClassName =
  'zgi-template-slot-placeholder relative mx-0.5 inline rounded-[4px] bg-blue-500/[0.08] px-1 py-0 align-[0.02em] text-blue-500/45 decoration-transparent select-none dark:bg-blue-400/[0.1] dark:text-blue-300/45';
const activePlaceholderClassName = 'zgi-template-slot-placeholder-active';

export function templateSlotFallbackText(attrs: { name?: string; placeholder?: string }) {
  return attrs.placeholder || attrs.name || '  ';
}

export function findTemplateSlotPlaceholderAround(
  doc: ProseMirrorNode,
  pos: number,
  nodeTypeName = 'templateSlotPlaceholder'
) {
  const safePos = Math.max(0, Math.min(pos, doc.content.size));
  const $pos = doc.resolve(safePos);
  const nodeBefore = $pos.nodeBefore;
  if (nodeBefore?.type.name === nodeTypeName) {
    return {
      from: safePos - nodeBefore.nodeSize,
      to: safePos,
      node: nodeBefore,
    };
  }

  const nodeAfter = $pos.nodeAfter;
  if (nodeAfter?.type.name === nodeTypeName) {
    return {
      from: safePos,
      to: safePos + nodeAfter.nodeSize,
      node: nodeAfter,
    };
  }

  return null;
}

const TemplateSlotPlaceholder = TiptapNode.create<{
  name?: string;
  placeholder?: string;
}>({
  name: 'templateSlotPlaceholder',
  inline: true,
  group: 'inline',
  atom: true,
  selectable: false,
  draggable: false,

  addAttributes() {
    return {
      name: {
        default: '',
        renderHTML: attributes => ({ 'data-name': attributes.name }),
        parseHTML: element => (element as HTMLElement).getAttribute('data-name') ?? '',
      },
      placeholder: {
        default: '',
        renderHTML: attributes => ({ 'data-placeholder': attributes.placeholder }),
        parseHTML: element => (element as HTMLElement).getAttribute('data-placeholder') ?? '',
      },
    };
  },

  parseHTML() {
    return [{ tag: 'span[data-zgi-slot-placeholder]' }];
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      'span',
      mergeAttributes(HTMLAttributes, {
        'data-zgi-slot-placeholder': 'true',
        contenteditable: 'false',
        class: placeholderClassName,
      }),
      templateSlotFallbackText(node.attrs),
    ];
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        props: {
          decorations: state => {
            const { selection, doc } = state;
            if (!selection.empty) return DecorationSet.empty;

            const placeholder = findTemplateSlotPlaceholderAround(doc, selection.from, this.name);
            if (!placeholder || selection.from !== placeholder.to) {
              return DecorationSet.empty;
            }

            return DecorationSet.create(doc, [
              Decoration.node(placeholder.from, placeholder.to, {
                class: activePlaceholderClassName,
                'data-active': 'true',
              }),
            ]);
          },

          handleClick: (view, pos) => {
            const placeholder = findTemplateSlotPlaceholderAround(view.state.doc, pos, this.name);
            if (!placeholder) return false;

            view.dispatch(
              view.state.tr
                .setSelection(TextSelection.create(view.state.doc, placeholder.to))
                .scrollIntoView()
            );
            view.focus();
            return true;
          },

          handleTextInput: (view, from, _to, text) => {
            if (!text) return false;

            const placeholder = findTemplateSlotPlaceholderAround(view.state.doc, from, this.name);
            if (!placeholder || from !== placeholder.to) return false;

            const mark = view.state.schema.marks.templateSlot.create({
              name: placeholder.node.attrs.name,
              placeholder: placeholder.node.attrs.placeholder,
            });
            const replacement = view.state.schema.text(text, [mark]);
            const tr = view.state.tr.replaceWith(placeholder.from, placeholder.to, replacement);
            tr.setSelection(TextSelection.create(tr.doc, placeholder.from + text.length));
            tr.setStoredMarks([mark]);
            view.dispatch(tr.scrollIntoView());
            return true;
          },

          handleKeyDown: (view, event) => {
            if (event.key !== 'Backspace' && event.key !== 'Delete') return false;

            const { state } = view;
            const { selection } = state;
            if (!selection.empty) return false;

            const placeholder = findTemplateSlotPlaceholderAround(state.doc, selection.from, this.name);
            if (!placeholder) return false;

            const isDeleteBefore =
              event.key === 'Delete' && selection.from === placeholder.from;
            const isBackspaceAfter =
              event.key === 'Backspace' && selection.from === placeholder.to;
            if (!isDeleteBefore && !isBackspaceAfter) return false;

            event.preventDefault();
            const tr = state.tr.delete(placeholder.from, placeholder.to);
            tr.setSelection(TextSelection.create(tr.doc, placeholder.from));
            view.dispatch(tr.scrollIntoView());
            return true;
          },
        },
      }),
    ];
  },
});

export default TemplateSlotPlaceholder;
