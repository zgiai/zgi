'use client';

import React, {
  useEffect,
  useMemo,
  forwardRef,
  useImperativeHandle,
  useRef,
  useCallback,
  useState,
} from 'react';
import { cn } from '@/lib/utils';
import { useWorkflowStore } from '@/components/workflow/store';
import type { UpstreamExportItem } from '@/components/workflow/store/helpers/graph';
import type { WorkflowVariable } from '@/components/workflow/store/type';
import type { StructuredTypeField } from '@/components/workflow/types/input-var';

// tiptap
import { EditorContent, useEditor } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import { Extension, type Editor as TiptapEditor } from '@tiptap/core';
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

// Custom extension to fix backspace behavior near atom nodes
// When cursor is right after an atom node, default backspace may fail to delete previous character
const FixAtomBackspace = Extension.create({
  name: 'fixAtomBackspace',
  addKeyboardShortcuts() {
    return {
      Backspace: ({ editor }) => {
        const { state, view } = editor;
        const { selection } = state;
        const { $from, empty } = selection;

        // Only handle when cursor is collapsed (no selection)
        if (!empty) return false;

        const pos = $from.pos;
        if (pos <= 1) return false;

        // Check if node immediately before cursor is an atom
        const nodeBefore = $from.nodeBefore;

        // Debug logging
        const parent = $from.parent;
        const indexInParent = $from.index();

        // If nodeBefore is a text node, default backspace should work
        // isAtom returns true for text (leaf nodes), so we check isText explicitly
        if (nodeBefore?.isText) {
          // Default backspace should delete text character - but if it's stuck,
          // force delete from current position
          const deleteFrom = pos - 1;
          const deleteTo = pos;
          const tr = state.tr.delete(deleteFrom, deleteTo);
          view.dispatch(tr);
          return true;
        }

        // If nodeBefore is a non-text atom (like variableToken), let default handle it
        if (nodeBefore?.type.isAtom) {
          return false;
        }

        // Look for pattern: [text][atom] with cursor after atom
        // In this case, indexInParent points to position after last child
        if (parent.childCount >= 2 && indexInParent >= 1) {
          // Check if previous child is a non-text atom
          const prevChild = parent.child(indexInParent - 1);
          if (prevChild.type.isAtom && !prevChild.isText) {
            // We're positioned right after an atom
            // Look for text node before the atom
            if (indexInParent >= 2) {
              const textChild = parent.child(indexInParent - 2);
              if (textChild.isText && textChild.text) {
                // Calculate absolute position of the text node's last character
                let textEndPos = $from.start();
                for (let i = 0; i < indexInParent - 2; i++) {
                  textEndPos += parent.child(i).nodeSize;
                }
                textEndPos += textChild.nodeSize;
                const tr = state.tr.delete(textEndPos - 1, textEndPos);
                view.dispatch(tr);
                return true;
              }
            }
          }
        }

        // Let default handler proceed
        return false;
      },
    };
  },
});
import { valueToDoc, docToValue } from './utils/value-transform';
import VariableToken from './nodes/variable-token';
import TemplateSlot, { findTemplateSlotRangeAround } from './nodes/template-slot';
import TemplateSlotPlaceholder, {
  findTemplateSlotPlaceholderAround,
} from './nodes/template-slot-placeholder';
import VariableSuggestPanel from './variable-suggest-panel';
import { useT } from '@/i18n';

// Stable plugin key for our Suggestion plugin to avoid duplicate-key collisions
const SUGGEST_PLUGIN_KEY = 'wf-variable-suggest';
// Create a real ProseMirror PluginKey instance (strings won't have getState and will break PM)
const SUGGEST_PM_PLUGIN_KEY = new PluginKey(SUGGEST_PLUGIN_KEY);

// Token pattern utils are now imported from ./utils/value-transform

const EMPTY_BLOCK_PLACEHOLDER_KEY = new PluginKey('wf-empty-block-placeholder');

const EmptyBlockPlaceholder = Extension.create<{
  emptyBlockPlaceholder: string;
}>({
  name: 'emptyBlockPlaceholder',

  addOptions() {
    return {
      emptyBlockPlaceholder: '',
    };
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: EMPTY_BLOCK_PLACEHOLDER_KEY,
        props: {
          decorations: state => {
            const decorations: Decoration[] = [];
            const { doc, selection } = state;

            if (selection.empty) {
              const { $from } = selection;
              const placeholder = this.options.emptyBlockPlaceholder;
              if (
                placeholder &&
                $from.parent.type.name === 'paragraph' &&
                $from.parent.content.size === 0
              ) {
                decorations.push(
                  Decoration.widget(
                    $from.pos,
                    () => {
                      const span = document.createElement('span');
                      span.textContent = placeholder;
                      span.contentEditable = 'false';
                      span.className =
                        'pointer-events-none select-none whitespace-normal break-words text-muted-foreground/70';
                      return span;
                    },
                    {
                      ignoreSelection: true,
                      key: `empty-paragraph-placeholder-${$from.pos}-${placeholder}`,
                      side: 0,
                    }
                  )
                );
              }
            }

            return decorations.length > 0
              ? DecorationSet.create(doc, decorations)
              : DecorationSet.empty;
          },
        },
      }),
    ];
  },
});

export interface WorkflowValueEditorHandle {
  insertToken: (sourceId: string, name: string, label?: string) => void;
  replaceValue: (value: string) => void;
  focus: () => void;
  openVariableSelector: () => void;
}

export interface WorkflowValueEditorProps {
  className?: string;
  editorClassName?: string;
  value: string;
  onChange: (value: string) => void;
  readOnly?: boolean;
  placeholder?: string;
  emptyBlockPlaceholder?: string;
  nodeId?: string; // current node id to load upstream variables
  // Whether variable suggestion (slash-trigger) is enabled. Default: true
  suggestEnabled?: boolean;
  // Whether "/" should open the variable suggestion list while typing. Default: true
  slashTriggerEnabled?: boolean;
  // Extra suggest items appended after upstream groups
  extraSuggestItems?: VarOption[];
  // Extra suggest groups appended after upstream groups.
  extraSuggestGroups?: Array<{ id: string; title: string; items: VarOption[] }>;
  // Title for extra group
  extraGroupTitle?: string;
  // Notify parent when the TipTap editor gains focus
  onFocus?: () => void;
  // Optional portal root for rendering the variable suggest panel
  portalRoot?: React.ComponentProps<typeof VariableSuggestPanel>['portalRoot'];
  // Optional character counter. It is display-only; validation remains with the caller.
  showCharacterCount?: boolean;
  maxLength?: number;
  characterCount?: number;
  characterCountWarningThreshold?: number;
  characterCountFormatter?: (count: number, maxLength: number) => React.ReactNode;
  // Enables ZGI prompt template blocks such as <zgi:slot>, <zgi:knowledge>, and <zgi:skill>.
  templateBlocksEnabled?: boolean;
}

// VariableToken node and value transform functions are imported from local modules to keep this file focused on editor wiring.

// Upstream variable option
export interface VarOption {
  sourceId: string;
  sourceTitle: string;
  key: string;
  insertKey?: string;
  type: WorkflowVariable['type'];
  description?: string;
  hasChildren?: boolean;
  label?: string;
  displayKey?: string;
  showType?: boolean;
  invalid?: boolean;
}

const WorkflowValueEditor = forwardRef<WorkflowValueEditorHandle, WorkflowValueEditorProps>(
  (
    {
      className,
      editorClassName,
      value,
      onChange,
      readOnly = false,
      placeholder = `Enter '/' to insert variable`,
      emptyBlockPlaceholder,
      nodeId,
      suggestEnabled = true,
      slashTriggerEnabled = true,
      extraSuggestItems,
      extraSuggestGroups,
      extraGroupTitle,
      onFocus,
      portalRoot,
      showCharacterCount = false,
      maxLength,
      characterCount: characterCountOverride,
      characterCountWarningThreshold,
      characterCountFormatter,
      templateBlocksEnabled = false,
    },
    ref
  ) => {
    // Access store safely to get upstream variables for suggestions
    const getUpstreamVariables = useWorkflowStore.use.getUpstreamVariables();
    const getAncestors = useWorkflowStore.use.getAncestors();
    // Use graphVersion as a stable trigger for structural updates.
    // We avoid subscribing to nodeIdToTitle directly to prevent re-renders when nodes move.
    const graphVersion = useWorkflowStore.use.graphVersion();
    const t = useT();
    const shouldShowCharacterCount = showCharacterCount && typeof maxLength === 'number';
    const characterCount = shouldShowCharacterCount
      ? characterCountOverride ?? Array.from(value || '').length
      : 0;
    const isCharacterCountExceeded =
      shouldShowCharacterCount && characterCount > (maxLength as number);
    const isCharacterCountWarning =
      shouldShowCharacterCount &&
      !isCharacterCountExceeded &&
      typeof characterCountWarningThreshold === 'number' &&
      characterCount > characterCountWarningThreshold;

    // Helper to resolve description from descriptionKey
    const resolveDescription = useCallback(
      (descriptionKey?: string, description?: string): string | undefined => {
        if (descriptionKey) {
          return t(`nodes.${descriptionKey}` as Parameters<typeof t>[0], {
            innerType: description || 'string',
          });
        }
        return description;
      },
      [t]
    );

    // Recursive flattener for nested fields
    const flattenVariable = useCallback(
      (
        sid: string,
        st: string,
        key: string,
        type: WorkflowVariable['type'],
        description?: string,
        children?: StructuredTypeField[],
        depth = 0
      ): VarOption[] => {
        const result: VarOption[] = [
          {
            sourceId: sid,
            sourceTitle: st,
            key,
            type,
            description,
            hasChildren: children && children.length > 0,
          },
        ];

        // StructuredTypeField currently doesn't have nested fields in its interface,
        // but if it ever does or if we use getStructuredTypeFields recursively, we'd handle it here.
        if (depth < 5 && children && children.length > 0) {
          for (const f of children) {
            result.push(
              ...flattenVariable(
                sid,
                st,
                `${key}.${f.key}`,
                f.type as WorkflowVariable['type'],
                resolveDescription(f.descriptionKey),
                f.children,
                depth + 1
              )
            );
          }
        }
        return result;
      },
      [resolveDescription]
    );

    const [isFocused, setIsFocused] = useState(false);

    // Tracks current drill-down path in the suggestion list (e.g. ['nodeId', 'varKey'])
    const suggestSessionPathRef = useRef<string[]>([]);

    // Integrated suggestion state for stable React-based rendering
    interface SuggestItem {
      sourceId: string;
      key: string;
      insertKey?: string;
      sourceTitle?: string;
      type?: string;
      description?: string;
      hasChildren?: boolean;
      displayKey?: string;
      label?: string;
      showType?: boolean;
    }

    const [suggestState, setSuggestState] = useState<{
      open: boolean;
      x: number;
      y: number;
      query: string;
      trigger: 'slash' | 'manual';
      editor: TiptapEditor;
      command: (item: SuggestItem) => void;
      path: string[];
      activeGroupIndex: number;
      activeItemIndex: number;
    } | null>(null);

    // Refs need to be declared before callbacks that use them
    const suggestStateRef = useRef(suggestState);
    useEffect(() => {
      suggestStateRef.current = suggestState;
    }, [suggestState]);

    const closeSuggest = useCallback(() => {
      setSuggestState(null);
      suggestSessionPathRef.current = [];
    }, []);

    const suggestionEnabled = suggestEnabled && !readOnly;

    const handleSuggestSelect = useCallback(
      (it: SuggestItem) => {
        if (!suggestStateRef.current) return;
        try {
          suggestStateRef.current.editor.chain().focus();
          suggestStateRef.current.command(it);
          closeSuggest();
        } catch (err) {
          console.error('Failed to apply suggestion:', err);
        }
      },
      [closeSuggest]
    );

    const handleSuggestExpand = useCallback((it: { sourceId: string; key: string }) => {
      setSuggestState(prev => {
        if (!prev) return null;
        let nextPath: string[];
        if (prev.path.length === 0) {
          nextPath = [it.sourceId, it.key];
        } else {
          const parts = it.key.split('.');
          nextPath = [...prev.path, parts[parts.length - 1]];
        }
        suggestSessionPathRef.current = nextPath;
        return { ...prev, path: nextPath, activeGroupIndex: 0, activeItemIndex: 0 };
      });
    }, []);

    const handleSuggestBack = useCallback(() => {
      setSuggestState(prev => {
        if (!prev) return null;
        let nextPath: string[];
        if (prev.path.length <= 2) {
          nextPath = [];
        } else {
          nextPath = prev.path.slice(0, -1);
        }
        suggestSessionPathRef.current = nextPath;
        return { ...prev, path: nextPath, activeGroupIndex: 0, activeItemIndex: 0 };
      });
    }, []);

    const handleSuggestHover = useCallback((gi: number, ii: number) => {
      setSuggestState(prev =>
        prev ? { ...prev, activeGroupIndex: gi, activeItemIndex: ii } : null
      );
    }, []);

    const groups = useMemo(() => {
      void graphVersion;
      const list: Array<{ id: string; title: string; items: VarOption[] }> = [];
      // Delay expensive group and variable flattening until the editor is actually interacted with.
      if (!isFocused && !suggestState) return list;

      const upstreams: UpstreamExportItem[] = nodeId ? getUpstreamVariables(nodeId) || [] : [];

      // Build a dedicated sys group from upstream variables (deduped by key)
      const sysSeen = new Set<string>();
      const sysItems: VarOption[] = [];
      for (const src of upstreams) {
        const vars = src.variables || [];
        for (const v of vars) {
          if (typeof v.key === 'string' && v.key.startsWith('sys.')) {
            const trimmed = v.key.slice(4);
            if (!sysSeen.has(trimmed)) {
              sysSeen.add(trimmed);
              sysItems.push({
                sourceId: 'sys',
                sourceTitle: t('agents.workflow.systemVariables.title'),
                key: trimmed,
                type: v.type as WorkflowVariable['type'],
                description: resolveDescription(v.descriptionKey, v.description),
              });
            }
          }
        }
      }

      // Build normal groups excluding sys variables, with recursive flattening
      // Environment and conversation variables are included from upstreams
      const normalGroups = upstreams.map(src => {
        // Apply i18n for special groups (environment, conversation)
        let groupTitle = src.nodeTitle || src.nodeId;
        if (src.nodeId === 'environment') {
          groupTitle = t('agents.workflow.environmentVariables.title');
        } else if (src.nodeId === 'conversation') {
          groupTitle = t('agents.workflow.conversationVariables.title');
        }

        const flattenedItems = (src.variables || [])
          .filter(v => !(typeof v.key === 'string' && v.key.startsWith('sys.')))
          .flatMap(v =>
            flattenVariable(
              src.nodeId,
              groupTitle,
              v.key,
              v.type as WorkflowVariable['type'],
              resolveDescription(v.descriptionKey, v.description),
              v.children as StructuredTypeField[]
            )
          );

        return {
          id: src.nodeId,
          title: groupTitle,
          items: flattenedItems,
        };
      });

      if (Array.isArray(extraSuggestGroups) && extraSuggestGroups.length > 0) {
        extraSuggestGroups.forEach(group => {
          if (group.items.length > 0) {
            list.push(group);
          }
        });
      } else if (Array.isArray(extraSuggestItems) && extraSuggestItems.length > 0) {
        list.push({
          id: '__extra__',
          title: extraGroupTitle || '上下文',
          items: extraSuggestItems,
        });
      }
      if (sysItems.length > 0) {
        list.push({
          id: '__sys__',
          title: t('agents.workflow.systemVariables.title'),
          items: sysItems,
        });
      }
      list.push(...normalGroups);
      return list;
    }, [
      getUpstreamVariables,
      nodeId,
      extraSuggestItems,
      extraSuggestGroups,
      extraGroupTitle,
      flattenVariable,
      resolveDescription,
      isFocused,
      suggestState,
      graphVersion,
      t,
    ]);

    const groupsRef = useRef<Array<{ id: string; title: string; items: VarOption[] }>>(groups);
    useEffect(() => {
      groupsRef.current = groups;
    }, [groups]);

    // Map sourceId to sourceTitle for quick lookup when rendering tokens
    const idToTitle = useMemo(() => {
      void graphVersion;
      const state = useWorkflowStore.getState();
      // In history mode, compute titles from snapshot nodes
      let baseMap: Map<string, string>;
      if (state.mode === 'history' && state.selectedRunId) {
        const snap = state.historySnapshots[state.selectedRunId];
        if (snap && Array.isArray(snap.nodes)) {
          baseMap = new Map<string, string>();
          for (const n of snap.nodes) {
            const title = n.data?.title;
            if (title) baseMap.set(n.id, title);
          }
        } else {
          baseMap = new Map<string, string>(state.nodeIdToTitle);
        }
      } else {
        baseMap = new Map<string, string>(state.nodeIdToTitle);
      }
      // Override special groups with i18n labels
      baseMap.set('sys', t('agents.workflow.systemVariables.title'));
      baseMap.set('environment', t('agents.workflow.environmentVariables.title'));
      baseMap.set('conversation', t('agents.workflow.conversationVariables.title'));
      if (Array.isArray(extraSuggestItems)) {
        extraSuggestItems.forEach(item => {
          if (item.sourceId && item.sourceTitle) {
            baseMap.set(item.sourceId, item.sourceTitle);
          }
        });
      }
      if (Array.isArray(extraSuggestGroups)) {
        extraSuggestGroups.forEach(group => {
          group.items.forEach(item => {
            if (item.sourceId && item.sourceTitle) {
              baseMap.set(item.sourceId, item.sourceTitle);
            }
          });
        });
      }
      return baseMap;
    }, [extraSuggestItems, extraSuggestGroups, graphVersion, t]);

    // Keep latest title mapping in a ref to avoid re-registering Suggestion plugin on title updates
    const idToTitleRef = useRef<Map<string, string>>(idToTitle);
    useEffect(() => {
      idToTitleRef.current = idToTitle;
    }, [idToTitle]);

    const extraTokenLabels = useMemo(() => {
      const map = new Map<string, { label: string; title: string; invalid?: boolean }>();
      if (Array.isArray(extraSuggestItems)) {
        for (const item of extraSuggestItems) {
          map.set(`${item.sourceId}\0${item.key}`, {
            label: item.label || item.key || item.sourceTitle,
            title: item.sourceTitle || item.sourceId,
            invalid: item.invalid,
          });
          if (item.insertKey) {
            map.set(`${item.sourceId}\0${item.insertKey}`, {
              label: item.label || item.insertKey || item.sourceTitle,
              title: item.sourceTitle || item.sourceId,
              invalid: item.invalid,
            });
          }
        }
      }
      if (Array.isArray(extraSuggestGroups)) {
        for (const group of extraSuggestGroups) {
          for (const item of group.items) {
            map.set(`${item.sourceId}\0${item.key}`, {
              label: item.label || item.key || item.sourceTitle,
              title: item.sourceTitle || item.sourceId,
              invalid: item.invalid,
            });
            if (item.insertKey) {
              map.set(`${item.sourceId}\0${item.insertKey}`, {
                label: item.label || item.insertKey || item.sourceTitle,
                title: item.sourceTitle || item.sourceId,
                invalid: item.invalid,
              });
            }
          }
        }
      }
      return map;
    }, [extraSuggestItems, extraSuggestGroups]);

    // Labels for suggestion panel - stored in ref to avoid re-registering plugin on t change
    const suggestLabelsRef = useRef({ empty: '' });
    useEffect(() => {
      suggestLabelsRef.current = { empty: t('nodes.common.noVariables') };
    }, [t]);

    const wrapperRef = useRef<HTMLDivElement>(null);
    const closeSuggestRef = useRef<(() => void) | null>(null);
    const lastEmittedValueRef = useRef<string>(value);
    const lastEmitTimeRef = useRef<number>(0);
    const lastSeenPropRef = useRef<string>(value);

    useEffect(() => {
      closeSuggestRef.current = closeSuggest;
    }, [closeSuggest]);

    // If suggestions are being disabled dynamically, ensure any open dropdown is closed
    useEffect(() => {
      if (!suggestionEnabled) {
        closeSuggest();
      }
    }, [suggestionEnabled, closeSuggest]);

    const handlersRef = useRef({
      handleSuggestSelect,
      handleSuggestExpand,
      handleSuggestBack,
      closeSuggest,
    });
    useEffect(() => {
      handlersRef.current = {
        handleSuggestSelect,
        handleSuggestExpand,
        handleSuggestBack,
        closeSuggest,
      };
    }, [handleSuggestSelect, handleSuggestExpand, handleSuggestBack, closeSuggest]);

    const suggestPath = useMemo(() => {
      if (!suggestState) return [];
      const { path } = suggestState;
      if (path.length === 0) return [];
      const resolved = [idToTitle.get(path[0]) || path[0]];
      let currentKey = '';
      for (const segment of path.slice(1)) {
        currentKey = currentKey ? `${currentKey}.${segment}` : segment;
        const match = groups
          .find(group => group.id === path[0])
          ?.items.find(item => item.key === currentKey);
        resolved.push(match?.label || match?.displayKey || segment);
      }
      return resolved;
    }, [suggestState, idToTitle, groups]);

    const editorRef = useRef<TiptapEditor | null>(null);

    const buildVariableTokenAttrs = useCallback(
      (item: {
        sourceId: string;
        key: string;
        insertKey?: string;
        label?: string;
        displayKey?: string;
      }) => ({
        sourceId: item.sourceId,
        key: item.insertKey || item.key,
        title: idToTitleRef.current.get(item.sourceId) || item.sourceId,
        label: item.label || item.displayKey || item.key,
        syntax:
          templateBlocksEnabled &&
          (item.sourceId === 'knowledge' ||
            item.sourceId === 'skill' ||
            item.sourceId === 'database' ||
            item.sourceId === 'table' ||
            item.sourceId === 'workflow')
            ? 'zgi'
            : '',
      }),
      [templateBlocksEnabled]
    );

    const insertVariableToken = useCallback(
      (
        item: {
        sourceId: string;
        key: string;
        insertKey?: string;
        label?: string;
        displayKey?: string;
        },
        replaceRange?: { from: number; to: number }
      ) => {
        const currentEditor = editorRef.current;
        if (!currentEditor || !item.sourceId) return;

        const { state, view } = currentEditor;
        const tokenType = state.schema.nodes.variableToken;
        if (!tokenType) return;

        const selection = state.selection;
        const from = replaceRange?.from ?? selection.from;
        const to = replaceRange?.to ?? selection.to;
        const tokenNode = tokenType.create(buildVariableTokenAttrs(item));
        const placeholderRange =
          findTemplateSlotPlaceholderAround(state.doc, from) ||
          findTemplateSlotPlaceholderAround(state.doc, to);
        const slotRange =
          findTemplateSlotRangeAround(state.doc, from) ||
          findTemplateSlotRangeAround(state.doc, to);

        let tr = state.tr;
        let insertPos = from;

        if (placeholderRange) {
          tr = tr.delete(placeholderRange.from, placeholderRange.to);
          insertPos = tr.mapping.map(placeholderRange.from);
        } else if (slotRange) {
          const slotMarkType = state.schema.marks.templateSlot;
          tr = tr.removeMark(slotRange.from, slotRange.to, slotMarkType);
          const mappedFrom = tr.mapping.map(from);
          const mappedTo = tr.mapping.map(to);
          if (mappedFrom !== mappedTo) {
            tr = tr.delete(mappedFrom, mappedTo);
          }
          insertPos = tr.mapping.map(mappedFrom);
        } else {
          if (from !== to) {
            tr = tr.delete(from, to);
          }
          insertPos = tr.mapping.map(from);
        }

        tr = tr.insert(insertPos, tokenNode);
        tr = tr.setSelection(
          TextSelection.create(tr.doc, Math.min(insertPos + tokenNode.nodeSize, tr.doc.content.size))
        );
        tr = tr.setStoredMarks([]);
        view.dispatch(tr.scrollIntoView());
        view.focus();
      },
      [buildVariableTokenAttrs]
    );

    const buildSuggestCommand = useCallback((replaceRange?: { from: number; to: number }) => {
      return (item: SuggestItem) => {
        insertVariableToken(item, replaceRange);
      };
    }, [insertVariableToken]);

    const openManualSuggest = useCallback(() => {
      if (!suggestEnabled || !editorRef.current) return;

      const currentEditor = editorRef.current;
      currentEditor.chain().focus().run();

      const { selection } = currentEditor.state;
      const coords = currentEditor.view.coordsAtPos(selection.from);
      const rootRect =
        portalRoot instanceof HTMLElement ? portalRoot.getBoundingClientRect() : null;
      suggestSessionPathRef.current = [];

      setSuggestState({
        open: true,
        x: rootRect ? coords.left - rootRect.left : coords.left,
        y: rootRect ? coords.bottom - rootRect.top + 4 : coords.bottom + 4,
        query: '',
        trigger: 'manual',
        editor: currentEditor,
        command: buildSuggestCommand(),
        path: [],
        activeGroupIndex: 0,
        activeItemIndex: 0,
      });
    }, [buildSuggestCommand, portalRoot, suggestEnabled]);

    const extensions = useMemo(() => {
      return [
        FixAtomBackspace,
        VariableToken,
        TemplateSlotPlaceholder,
        TemplateSlot,
        // Custom variable trigger extension that allows detection anywhere (e.g. 123/|)
        Extension.create({
          name: 'variable-trigger',
          priority: 200,
          addProseMirrorPlugins() {
            return [
              new Plugin({
                key: SUGGEST_PM_PLUGIN_KEY,
                view() {
                  return {
                    update: view => {
                      const { state } = view;
                      const { selection, doc } = state;
                      const { $from, empty } = selection;

                      if (!empty || !suggestionEnabled) {
                        if (suggestStateRef.current?.open) handlersRef.current?.closeSuggest();
                        return;
                      }

                      if (!slashTriggerEnabled) {
                        if (suggestStateRef.current?.trigger === 'slash') {
                          handlersRef.current?.closeSuggest();
                        }
                        return;
                      }

                      // Look back for '/' trigger
                      const textBefore = $from.parent.textBetween(
                        Math.max(0, $from.parentOffset - 50),
                        $from.parentOffset,
                        '\n',
                        '\n'
                      );

                      const match = textBefore.match(/(^|\s)\/([^\s/]*)$/);
                      if (match) {
                        const prefix = match[1] ?? '';
                        const query = match[2] ?? '';
                        const start = $from.pos - query.length - 1;
                        const end = $from.pos;

                        const charAtStart = doc.textBetween(start, start + 1);
                        if (charAtStart !== '/' || (prefix && !/\s/.test(prefix))) {
                          if (suggestStateRef.current?.open) handlersRef.current?.closeSuggest();
                          return;
                        }

                        const coords = view.coordsAtPos(start);
                        const rootRect =
                          portalRoot instanceof HTMLElement
                            ? portalRoot.getBoundingClientRect()
                            : null;
                        const currentEditor = editorRef.current;
                        if (!currentEditor) return;
                        const nextState = {
                          open: true,
                          x: rootRect ? coords.left - rootRect.left : coords.left,
                          y: rootRect ? coords.bottom - rootRect.top + 4 : coords.bottom + 4,
                          query,
                          trigger: 'slash' as const,
                          editor: currentEditor,
                          command: buildSuggestCommand({ from: start, to: end }),
                          path: suggestSessionPathRef.current,
                          activeGroupIndex:
                            suggestStateRef.current?.query !== query
                              ? 0
                              : (suggestStateRef.current?.activeGroupIndex ?? 0),
                          activeItemIndex:
                            suggestStateRef.current?.query !== query
                              ? 0
                              : (suggestStateRef.current?.activeItemIndex ?? 0),
                        };

                        const prev = suggestStateRef.current;
                        if (
                          !prev ||
                          prev.query !== nextState.query ||
                          Math.abs(prev.x - nextState.x) > 1 ||
                          Math.abs(prev.y - nextState.y) > 1
                        ) {
                          setSuggestState(nextState);
                        }
                      } else {
                        if (suggestStateRef.current?.trigger === 'slash') {
                          handlersRef.current?.closeSuggest();
                        }
                      }
                    },
                  };
                },
              }),
            ];
          },
          addKeyboardShortcuts() {
            return {
              ArrowUp: () => {
                const s = suggestStateRef.current;
                if (!s?.open) return false;
                setSuggestState(prev => {
                  if (!prev) return null;
                  const currentGroups = enrichedGroupsRef.current;
                  if (!currentGroups.length) return prev;
                  let gi = prev.activeGroupIndex;
                  let ii = prev.activeItemIndex - 1;
                  if (ii < 0) {
                    gi = (gi - 1 + currentGroups.length) % currentGroups.length;
                    ii = (currentGroups[gi]?.items?.length || 1) - 1;
                  }
                  return { ...prev, activeGroupIndex: gi, activeItemIndex: ii };
                });
                return true;
              },
              ArrowDown: () => {
                const s = suggestStateRef.current;
                if (!s?.open) return false;
                setSuggestState(prev => {
                  if (!prev) return null;
                  const currentGroups = enrichedGroupsRef.current;
                  if (!currentGroups.length) return prev;
                  let gi = prev.activeGroupIndex;
                  let ii = prev.activeItemIndex + 1;
                  if (ii >= (currentGroups[gi]?.items?.length || 0)) {
                    gi = (gi + 1) % currentGroups.length;
                    ii = 0;
                  }
                  return { ...prev, activeGroupIndex: gi, activeItemIndex: ii };
                });
                return true;
              },
              ArrowRight: () => {
                const s = suggestStateRef.current;
                if (!s?.open) return false;
                const item =
                  enrichedGroupsRef.current[s.activeGroupIndex]?.items?.[s.activeItemIndex];
                if (item?.hasChildren) {
                  handlersRef.current?.handleSuggestExpand(item);
                  return true;
                }
                return false;
              },
              ArrowLeft: () => {
                const s = suggestStateRef.current;
                if (!s?.open || s.path.length === 0) return false;
                handlersRef.current?.handleSuggestBack();
                return true;
              },
              Enter: () => {
                const s = suggestStateRef.current;
                if (!s?.open) return false;
                const item =
                  enrichedGroupsRef.current[s.activeGroupIndex]?.items?.[s.activeItemIndex];
                if (item && item.key) {
                  s.command(item);
                  return true;
                }
                return true; // prevent newline even if no selection
              },
              Tab: () => {
                const s = suggestStateRef.current;
                if (!s?.open) return false;
                const item =
                  enrichedGroupsRef.current[s.activeGroupIndex]?.items?.[s.activeItemIndex];
                if (item && item.key) {
                  s.command(item);
                  return true;
                }
                return false;
              },
              Escape: () => {
                if (!suggestStateRef.current?.open) return false;
                handlersRef.current?.closeSuggest();
                return true;
              },
              Backspace: () => {
                const s = suggestStateRef.current;
                if (s?.open && !s.query && s.path.length > 0) {
                  handlersRef.current?.handleSuggestBack();
                  return true;
                }
                return false;
              },
            };
          },
        }),
        StarterKit.configure({
          heading: false,
          blockquote: false,
          code: false,
        }),
        EmptyBlockPlaceholder.configure({
          emptyBlockPlaceholder: emptyBlockPlaceholder || placeholder,
        }),
      ];
    }, [
      buildSuggestCommand,
      emptyBlockPlaceholder,
      placeholder,
      portalRoot,
      slashTriggerEnabled,
      suggestionEnabled,
    ]);

    const editor = useEditor({
      extensions,
      content: valueToDoc(value, { templateBlocksEnabled }),
      editable: !readOnly,
      editorProps: {
        attributes: {
          class:
            'ProseMirror min-w-0 max-w-full whitespace-pre-wrap break-all outline-none grow [&_p]:m-0',
        },
      },
      onUpdate: ({ editor }) => {
        if (readOnly) return;
        const next = docToValue(editor.getJSON());
        if (next !== lastEmittedValueRef.current && next !== lastSeenPropRef.current) {
          lastEmitTimeRef.current = Date.now();
          lastEmittedValueRef.current = next;
          onChange(next);
        }
      },
      immediatelyRender: false,
    });

    useEffect(() => {
      editorRef.current = editor;
    }, [editor]);

    useEffect(() => {
      if (!editor) return;
      editor.setEditable(!readOnly);
      if (readOnly) {
        closeSuggest();
      }
    }, [editor, readOnly, closeSuggest]);

    // Notify parent when editor gains focus
    useEffect(() => {
      if (!editor) return;
      const onFocusCb = () => {
        setIsFocused(true);
        if (typeof onFocus === 'function') onFocus();
      };
      const onBlurCb = () => setIsFocused(false);
      editor.on('focus', onFocusCb);
      editor.on('blur', onBlurCb);
      return () => {
        editor.off('focus', onFocusCb);
        editor.off('blur', onBlurCb);
      };
    }, [editor, onFocus]);

    // Update variable token titles (invalid/title)
    const updateTokenTitles = useCallback(() => {
      if (!editor) return;
      try {
        const { state, view } = editor;
        const { doc, schema } = state;
        const varType = schema.nodes.variableToken;
        if (!varType) return;
        const allowedSpecial = new Set<string>(['sys', 'conversation', 'environment']);
        if (Array.isArray(extraSuggestItems)) {
          extraSuggestItems.forEach(item => {
            if (item.sourceId) {
              allowedSpecial.add(item.sourceId);
            }
          });
        }
        if (Array.isArray(extraSuggestGroups)) {
          extraSuggestGroups.forEach(group => {
            group.items.forEach(item => {
              if (item.sourceId) {
                allowedSpecial.add(item.sourceId);
              }
            });
          });
        }
        const upstreamSet = nodeId ? new Set<string>(getAncestors(nodeId) || []) : null;
        const updates: Array<{ pos: number; attrs: Record<string, unknown> }> = [];
        doc.descendants((node, pos) => {
          if (node.type === varType) {
            const sid = String((node.attrs as { sourceId?: string }).sourceId || '');
            const key = String((node.attrs as { key?: string }).key || '');
            const extraLabel = extraTokenLabels.get(`${sid}\0${key}`);
            const desiredTitle = extraLabel?.title || idToTitleRef.current.get(sid) || sid;
            const desiredLabel = extraLabel?.label || '';
            const currentTitle = String((node.attrs as { title?: string }).title || '');
            const currentLabel = String((node.attrs as { label?: string }).label || '');
            const isExtraSource = Boolean(
              (Array.isArray(extraSuggestItems) &&
                extraSuggestItems.some(item => item.sourceId === sid)) ||
                (Array.isArray(extraSuggestGroups) &&
                  extraSuggestGroups.some(group =>
                    group.items.some(item => item.sourceId === sid)
                  ))
            );
            const shouldMarkInvalid = Boolean(
              sid &&
                (isExtraSource
                  ? !extraLabel || extraLabel.invalid
                  : !allowedSpecial.has(sid) && upstreamSet
                    ? !upstreamSet.has(sid)
                    : false)
            );
            if (desiredTitle && desiredTitle !== currentTitle) {
              updates.push({
                pos,
                attrs: { ...node.attrs, title: desiredTitle, label: desiredLabel },
              });
            } else if (desiredLabel !== currentLabel) {
              updates.push({ pos, attrs: { ...node.attrs, label: desiredLabel } });
            }
            if (shouldMarkInvalid !== Boolean((node.attrs as { invalid?: boolean }).invalid)) {
              updates.push({ pos, attrs: { ...node.attrs, invalid: shouldMarkInvalid } });
            }
          }
          return true;
        });
        if (updates.length > 0) {
          let tr = state.tr;
          for (const u of updates) {
            tr = tr.setNodeMarkup(u.pos, varType, u.attrs);
          }
          view.dispatch(tr);
        }
      } catch (e) {
        console.error('Error updating token titles:', e);
      }
    }, [editor, extraSuggestItems, extraSuggestGroups, extraTokenLabels, nodeId, getAncestors]);

    // Compute UI groups for the suggestion panel based on current path and query
    const enrichedGroups = useMemo(() => {
      if (!suggestState) return [];
      const lowQ = suggestState.query.toLowerCase();
      const segments = suggestState.path;

      return groups
        .map(g => {
          if (segments.length > 0) {
            const [sid, ...rest] = segments;
            if (g.id !== sid && sid !== '__sys__' && sid !== '__extra__') {
              return { title: g.title, items: [] };
            }
            const prefix = rest.length > 0 ? rest.join('.') + '.' : '';
            const items = g.items
              .filter(i => {
                if (prefix && !i.key.startsWith(prefix)) return false;
                const relativeKey = prefix ? i.key.slice(prefix.length) : i.key;
                return !relativeKey.includes('.');
              })
              .filter(i => {
                if (!lowQ) return true;
                return i.key.toLowerCase().includes(lowQ);
              })
              .map(i => {
                const relativeKey = prefix ? i.key.slice(prefix.length) : i.key;
                return {
                  ...i,
                  displayKey: i.displayKey || i.label || relativeKey || i.sourceTitle,
                  type: String(i.type),
                };
              });
            return { title: g.title, items };
          }
          const items = g.items
            .filter(i => {
              if (g.id !== '__extra__' && i.key.includes('.')) return false;
              if (!lowQ) return true;
              return (
                i.key.toLowerCase().includes(lowQ) ||
                i.sourceTitle.toLowerCase().includes(lowQ) ||
                (i.label ?? '').toLowerCase().includes(lowQ)
              );
            })
            .map(i => ({
              ...i,
              displayKey: i.displayKey || i.label || i.key || i.sourceTitle,
              type: String(i.type),
            }));
          return { title: g.title, items };
        })
        .filter(g => (g.items?.length ?? 0) > 0);
    }, [suggestState, groups]);

    const enrichedGroupsRef = useRef(enrichedGroups);
    useEffect(() => {
      enrichedGroupsRef.current = enrichedGroups;
    }, [enrichedGroups]);

    // Sync editor content with external value
    useEffect(() => {
      if (!editor) return;

      const isPropChange = value !== lastSeenPropRef.current;
      lastSeenPropRef.current = value;

      if (!isPropChange || value === lastEmittedValueRef.current) return;

      const now = Date.now();
      const isRecentlyTyped = editor.isFocused && now - lastEmitTimeRef.current < 1000;

      const current = docToValue(editor.getJSON());
      if (current !== value) {
        if (isRecentlyTyped) return;

        lastEmittedValueRef.current = value;
        editor.commands.setContent(valueToDoc(value, { templateBlocksEnabled }), {
          emitUpdate: false,
        });
        updateTokenTitles();
      }
    }, [value, editor, updateTokenTitles, templateBlocksEnabled]);

    // Keep token titles in sync with node changes
    useEffect(() => {
      updateTokenTitles();
    }, [updateTokenTitles, idToTitle]);

    // Suggestion plugin is now correctly managed via extensions array for proper priority

    useImperativeHandle(
      ref,
      () => ({
        insertToken: (sourceId: string, name: string, label?: string) => {
          if (readOnly) return;
          insertVariableToken({
            sourceId,
            key: name,
            label: label || name,
          });
        },
        replaceValue: (nextValue: string) => {
          if (readOnly || !editor) return;
          lastEmittedValueRef.current = nextValue;
          lastSeenPropRef.current = nextValue;
          editor.commands.setContent(valueToDoc(nextValue, { templateBlocksEnabled }), {
            emitUpdate: false,
          });
          updateTokenTitles();
          onChange(nextValue);
        },
        focus: () => {
          editor?.view?.focus();
        },
        openVariableSelector: () => {
          openManualSuggest();
        },
      }),
      [
        editor,
        insertVariableToken,
        onChange,
        openManualSuggest,
        readOnly,
        templateBlocksEnabled,
        updateTokenTitles,
      ]
    );

    return (
      <div className={cn('space-y-0.5', className)}>
        <div className="relative h-full min-h-0">
          <div
            ref={wrapperRef}
            className={cn(
              'relative w-full min-w-0 overflow-x-hidden overflow-y-auto rounded-sm border bg-background px-2.5 py-1.5 text-sm ring-offset-background focus-within:outline-none flex flex-col',
              readOnly ? 'cursor-default' : 'cursor-text',
              shouldShowCharacterCount && 'pb-6',
              editorClassName,
              isCharacterCountExceeded &&
                'border-destructive/70 focus-within:border-destructive focus-within:ring-1 focus-within:ring-destructive/20'
            )}
            onClick={() => {
              if (!readOnly) {
                editor?.view?.focus();
              }
            }}
            onBlur={() => {
              const root = wrapperRef.current;
              setTimeout(() => {
                const next = document.activeElement as HTMLElement | null;
                const suggest = document.querySelector('div[data-wf-suggest="open"]');
                const inSuggest = !!(suggest && next && suggest.contains(next));
                if (!inSuggest && root && (!next || !root.contains(next))) {
                  closeSuggest();
                }
              }, 100);
            }}
          >
            <EditorContent editor={editor} className="min-h-[1.5em] min-w-0" />
          </div>

          {shouldShowCharacterCount ? (
            <div
              className={cn(
                'pointer-events-none absolute bottom-1.5 right-2.5 rounded-sm bg-background/90 px-1 text-[11px] leading-4 text-muted-foreground shadow-sm',
                isCharacterCountWarning && 'text-amber-600',
                isCharacterCountExceeded && 'text-destructive'
              )}
            >
              {characterCountFormatter
                ? characterCountFormatter(characterCount, maxLength as number)
                : `${characterCount}/${maxLength}`}
            </div>
          ) : null}
        </div>

        {suggestState && (
          <VariableSuggestPanel
            open={suggestState.open}
            x={suggestState.x}
            y={suggestState.y}
            groups={enrichedGroups}
            activeGroupIndex={suggestState.activeGroupIndex}
            activeItemIndex={suggestState.activeItemIndex}
            suggestPath={suggestPath}
            onHover={handleSuggestHover}
            onSelect={handleSuggestSelect}
            onExpand={handleSuggestExpand}
            onBack={handleSuggestBack}
            portalRoot={portalRoot}
            labels={suggestLabelsRef.current}
            onOpenChange={open => !open && closeSuggest()}
          />
        )}
      </div>
    );
  }
);

WorkflowValueEditor.displayName = 'WorkflowValueEditor';

export default WorkflowValueEditor;
