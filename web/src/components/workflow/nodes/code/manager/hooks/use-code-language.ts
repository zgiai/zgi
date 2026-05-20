import React, { useCallback, useEffect } from 'react';
import type { CodeNodeData } from '../../../../store/type';
import { useDebouncedCommit } from '../../../../hooks/use-debounced-commit';
import { useLocalNodeData } from '../../../../hooks/use-local-node-data';
import { useNodeData } from '../../../../hooks/use-node-data';
import { useNodeDataUpdate } from '../../../../hooks/use-node-data-update';
import { useLocale } from '@/hooks/use-locale';
import { pickLocale } from '@/utils/tool-helpers';
import type { LocalizedString } from '@/services/types/system-settings';

export type CodeLanguageType = 'javascript' | 'python3';
export type EditorLanguageType = 'js' | 'python3';

/**
 * Code template definition with i18n support
 */
export interface CodeTemplate {
  language: CodeLanguageType;
  editorLanguage: EditorLanguageType;
  /** Localized code template (comments are in different languages) */
  template: LocalizedString;
}

/**
 * Default code templates with i18n support for comments
 */
export const DEFAULT_CODE_TEMPLATES: CodeTemplate[] = [
  {
    language: 'javascript',
    editorLanguage: 'js',
    template: {
      en_US: `// Entry function. Return an object with your outputs
function main({var1, var2}) {
  return { result: var1 + var2 };
}
`,
      zh_Hans: `// 入口函数，请返回包含输出字段的对象
function main({var1, var2}) {
  return { result: var1 + var2 };
}
`,
    },
  },
  {
    language: 'python3',
    editorLanguage: 'python3',
    template: {
      en_US: `# Entry function. Return a dict with your outputs
def main(var1, var2):
    return { 'result': var1 + var2 }
`,
      zh_Hans: `# 入口函数，请返回包含输出字段的字典
def main(var1, var2):
    return { 'result': var1 + var2 }
`,
    },
  },
];

export interface UseCodeLanguageOptions {
  /**
   * Custom code templates to override defaults.
   * If not provided, uses DEFAULT_CODE_TEMPLATES.
   */
  codeTemplates?: CodeTemplate[];
  /**
   * Callback when user tries to switch language with non-default code.
   * Should return a Promise that resolves to true if user confirms overwrite,
   * or false to keep current code.
   * If not provided, will auto-replace when code is empty or matches previous default.
   */
  onLanguageChangeConfirm?: (
    fromLang: EditorLanguageType,
    toLang: EditorLanguageType,
    currentCode: string,
    newDefaultCode: string
  ) => Promise<boolean>;
}

export interface UseCodeLanguageResult {
  codeValue: string;
  setCodeLocal: (code: string) => void;
  editorLanguage: EditorLanguageType;
  handleLangChange: (lang: EditorLanguageType) => void;
}

/**
 * Get code template for a language, with locale-aware comments
 */
export function getCodeTemplate(
  lang: CodeLanguageType,
  locale: string,
  templates: CodeTemplate[] = DEFAULT_CODE_TEMPLATES
): string {
  const template = templates.find(t => t.language === lang);
  if (!template) {
    // Fallback to first template or empty string
    return templates[0] ? pickLocale(templates[0].template, locale as 'en-US' | 'zh-Hans', '') : '';
  }
  return pickLocale(template.template, locale as 'en-US' | 'zh-Hans', '');
}

/**
 * Store-aware hook for managing code language in a Code node.
 * Automatically reads from and writes to the workflow store.
 */
export function useCodeLanguage(
  nodeId: string,
  isReadOnly: boolean,
  options: UseCodeLanguageOptions = {}
): UseCodeLanguageResult {
  const { locale } = useLocale();
  const templates = options.codeTemplates ?? DEFAULT_CODE_TEMPLATES;

  // Store-aware: read data directly from store
  const nodeData = useNodeData<CodeNodeData>(nodeId);
  const updateNodeData = useNodeDataUpdate<CodeNodeData>(nodeId);

  const {
    value: codeValue,
    setValue: setCodeValue,
    flush: flushCode,
  } = useDebouncedCommit<string>(nodeData?.code || '', {
    delay: 500,
    onCommit: (v: string) => updateNodeData({ code: v }),
    isEqual: (a: string, b: string) => a === b,
    flushOnUnmount: true,
  });

  const externalLangData = React.useMemo(
    () => ({ code_language: nodeData?.code_language ?? 'python3' }),
    [nodeData?.code_language]
  );

  const {
    localData: formData,
    setLocalData,
    flush: flushLang,
  } = useLocalNodeData<Pick<CodeNodeData, 'code_language'>>(externalLangData, {
    delay: 400,
    onCommit: (data: Partial<Pick<CodeNodeData, 'code_language'>>) => updateNodeData(data),
    isEqual: (a: Pick<CodeNodeData, 'code_language'>, b: Pick<CodeNodeData, 'code_language'>) =>
      a.code_language === b.code_language,
  });

  // Use refs to track the LATEST values for race condition prevention
  const codeValueRef = React.useRef(codeValue);
  const langRef = React.useRef(formData.code_language);
  const isChangingRef = React.useRef(false);

  // Keep refs in sync (these run AFTER render, so we also update synchronously below)
  codeValueRef.current = codeValue;
  langRef.current = formData.code_language;

  // Get default code for a language using current locale
  const getDefaultCode = useCallback(
    (lang: CodeLanguageType): string => {
      return getCodeTemplate(lang, locale, templates);
    },
    [locale, templates]
  );

  // Inject default code when empty
  useEffect(() => {
    if (isReadOnly) return;
    const current = (codeValue || '').trim();
    if (current.length === 0) {
      const lang = formData.code_language === 'javascript' ? 'javascript' : 'python3';
      const def = getDefaultCode(lang);
      setCodeValue(def);
      codeValueRef.current = def; // Sync ref immediately
      flushCode();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formData.code_language, isReadOnly]);

  const setCodeLocal = useCallback(
    (code: string) => {
      codeValueRef.current = code; // Sync ref immediately
      setCodeValue(code);
    },
    [setCodeValue]
  );

  const handleLangChange = useCallback(
    async (lang: EditorLanguageType) => {
      // Prevent concurrent language changes
      if (isChangingRef.current) return;
      isChangingRef.current = true;

      try {
        const nextLang: CodeLanguageType = lang === 'js' ? 'javascript' : 'python3';
        // Use ref to get the LATEST code value (synchronized above)
        const currentCode = (codeValueRef.current || '').trim();
        const prevLang: CodeLanguageType =
          langRef.current === 'javascript' ? 'javascript' : 'python3';

        // Don't switch to the same language
        if (nextLang === prevLang) return;

        // Get next default for the target language
        const nextDefault = getDefaultCode(nextLang);

        // Find the CURRENT language's template and collect ALL locale variants
        const prevTemplate = templates.find(t => t.language === prevLang);
        const currentLangDefaults: string[] = prevTemplate
          ? Object.values(prevTemplate.template)
              .filter((t): t is string => !!t)
              .map(t => t.trim())
          : [];

        // Auto-replace if:
        // 1. Code is empty, OR
        // 2. Code matches the CURRENT language's template (any locale variant)
        // All other cases (custom code, other language's template) should prompt
        const isAutoReplace =
          currentCode.length === 0 || currentLangDefaults.some(d => currentCode === d);

        if (isAutoReplace) {
          // Update refs IMMEDIATELY before state updates
          codeValueRef.current = nextDefault;
          langRef.current = nextLang;
          // BYPASS debounce: directly update both code and language in ONE call
          // This ensures atomic update without race conditions
          updateNodeData({ code: nextDefault, code_language: nextLang });
          // Also update local state for immediate UI feedback
          setCodeValue(nextDefault);
          setLocalData({ code_language: nextLang });
          return;
        }

        // If custom confirm callback is provided, ask user
        if (options.onLanguageChangeConfirm) {
          const fromEditorLang: EditorLanguageType = prevLang === 'javascript' ? 'js' : 'python3';
          const confirmed = await options.onLanguageChangeConfirm(
            fromEditorLang,
            lang,
            currentCode,
            nextDefault
          );
          // Update refs
          langRef.current = nextLang;
          if (confirmed) {
            codeValueRef.current = nextDefault;
            // BYPASS debounce: directly update both in ONE call
            updateNodeData({ code: nextDefault, code_language: nextLang });
            setCodeValue(nextDefault);
            setLocalData({ code_language: nextLang });
          } else {
            // Only update language, keep code
            updateNodeData({ code_language: nextLang });
            setLocalData({ code_language: nextLang });
          }
        } else {
          // No confirm callback, just switch language without replacing code
          langRef.current = nextLang;
          updateNodeData({ code_language: nextLang });
          setLocalData({ code_language: nextLang });
        }
      } finally {
        isChangingRef.current = false;
      }
    },
    [setLocalData, getDefaultCode, setCodeValue, updateNodeData, options, templates]
  );

  return {
    codeValue: codeValue || '',
    setCodeLocal,
    editorLanguage: formData.code_language === 'javascript' ? 'js' : 'python3',
    handleLangChange,
  };
}
