import { Mark, mergeAttributes } from '@tiptap/core';
import type { Mark as ProseMirrorMark, Node as ProseMirrorNode } from '@tiptap/pm/model';
import { Plugin, TextSelection } from '@tiptap/pm/state';

const slotClassName =
  'zgi-template-slot mx-0.5 inline rounded-[4px] bg-blue-500/10 px-1 py-0 align-[0.02em] text-blue-600 decoration-transparent dark:bg-blue-400/15 dark:text-blue-300';

function hasSameSlotMark(mark: ProseMirrorMark | undefined, target: ProseMirrorMark): boolean {
  return (
    Boolean(mark) &&
    mark?.type === target.type &&
    mark.attrs.name === target.attrs.name &&
    mark.attrs.placeholder === target.attrs.placeholder
  );
}

export function findTemplateSlotRange(
  doc: ProseMirrorNode,
  pos: number,
  markTypeName = 'templateSlot'
) {
  const safePos = Math.max(0, Math.min(pos, doc.content.size));
  const $pos = doc.resolve(safePos);
  const parent = $pos.parent;
  if (!parent.inlineContent) return null;

  let offset = 0;
  for (let index = 0; index < parent.childCount; index += 1) {
    const child = parent.child(index);
    const from = $pos.start() + offset;
    const to = from + child.nodeSize;
    const isInside = safePos >= from && safePos <= to;

    if (isInside && child.isText) {
      const slotMark = child.marks.find(mark => mark.type.name === markTypeName);
      if (!slotMark) return null;

      let rangeFrom = from;
      let rangeTo = to;

      for (let left = index - 1, leftOffset = offset; left >= 0; left -= 1) {
        const sibling = parent.child(left);
        leftOffset -= sibling.nodeSize;
        const siblingMark = sibling.marks.find(mark => hasSameSlotMark(mark, slotMark));
        if (!sibling.isText || !siblingMark) break;
        rangeFrom = $pos.start() + leftOffset;
      }

      let rightOffset = offset + child.nodeSize;
      for (let right = index + 1; right < parent.childCount; right += 1) {
        const sibling = parent.child(right);
        const siblingMark = sibling.marks.find(mark => hasSameSlotMark(mark, slotMark));
        if (!sibling.isText || !siblingMark) break;
        rangeTo = $pos.start() + rightOffset + sibling.nodeSize;
        rightOffset += sibling.nodeSize;
      }

      return {
        from: rangeFrom,
        to: rangeTo,
        mark: slotMark,
        text: doc.textBetween(rangeFrom, rangeTo, '', ''),
      };
    }

    offset += child.nodeSize;
  }

  return null;
}

export function findTemplateSlotRangeAround(
  doc: ProseMirrorNode,
  pos: number,
  markTypeName = 'templateSlot'
) {
  return (
    findTemplateSlotRange(doc, pos, markTypeName) ||
    findTemplateSlotRange(doc, pos + 1, markTypeName) ||
    findTemplateSlotRange(doc, pos - 1, markTypeName)
  );
}

const TemplateSlot = Mark.create<{
  name?: string;
  placeholder?: string;
}>({
  name: 'templateSlot',
  inclusive: true,
  excludes: '',

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
    return [
      {
        tag: 'span[data-zgi-slot]',
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'span',
      mergeAttributes(HTMLAttributes, {
        'data-zgi-slot': 'true',
        class: slotClassName,
      }),
      0,
    ];
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        props: {
          handleClick: (view, pos) => {
            const range = findTemplateSlotRangeAround(view.state.doc, pos, this.name);
            if (!range) return false;

            const safePos = Math.max(range.from, Math.min(pos, range.to));
            view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, safePos)));
            view.focus();
            return true;
          },

          handleKeyDown: (view, event) => {
            if (event.key !== 'Backspace' && event.key !== 'Delete') return false;

            const { state } = view;
            const { selection } = state;
            const range =
              findTemplateSlotRangeAround(state.doc, selection.from, this.name) ||
              findTemplateSlotRangeAround(state.doc, selection.to, this.name);
            if (!range) return false;

            const shouldRestoreFromSelection =
              !selection.empty && selection.from <= range.from && selection.to >= range.to;
            const shouldRestoreFromSingleDelete =
              selection.empty &&
              range.text.length <= 1 &&
              ((event.key === 'Backspace' &&
                selection.from > range.from &&
                selection.from <= range.to) ||
                (event.key === 'Delete' &&
                  selection.from >= range.from &&
                  selection.from < range.to));

            if (!shouldRestoreFromSelection && !shouldRestoreFromSingleDelete) return false;

            event.preventDefault();
            const placeholderNode = state.schema.nodes.templateSlotPlaceholder.create({
              name: range.mark.attrs.name,
              placeholder: range.mark.attrs.placeholder,
            });
            const tr = state.tr.replaceWith(range.from, range.to, placeholderNode);
            tr.setSelection(TextSelection.create(tr.doc, range.from + placeholderNode.nodeSize));
            view.dispatch(tr.scrollIntoView());
            return true;
          },
        },
      }),
    ];
  },
});

export default TemplateSlot;
