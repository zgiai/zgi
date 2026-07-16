import { readFileSync } from 'node:fs';

const source = readFileSync(
  'src/components/workflow/ui/features-panel/opening-statement-dialog.tsx',
  'utf8'
);

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const initializationStart = source.indexOf('const isOpening = open && !wasOpenRef.current;');
const initializationEnd = source.indexOf('}, [open, value]);', initializationStart);
const initializationEffect = source.slice(initializationStart, initializationEnd);

assert(initializationStart !== -1, 'Dialog draft should detect the closed-to-open edge.');
assert(initializationEnd !== -1, 'Dialog draft initialization effect should remain explicit.');
assert(
  initializationEffect.includes('wasOpenRef.current = open;'),
  'Dialog should remember the previous open state.'
);
assert(
  initializationEffect.includes('if (!isOpening) return;'),
  'Parent rerenders while open must not reinitialize the dialog draft.'
);
assert(
  initializationEffect.indexOf('if (!isOpening) return;') <
    initializationEffect.indexOf('setDraft(nextValue);'),
  'Draft initialization must happen after the opening-edge guard.'
);

const generationStart = source.indexOf('const handleGenerateSuggestedQuestions = useCallback');
const generationEnd = source.indexOf('const questions = normalizeQuestions', generationStart);
const generationHandler = source.slice(generationStart, generationEnd);

assert(generationStart !== -1 && generationEnd !== -1, 'Suggestion generation handler is missing.');
assert(
  generationHandler.includes('setDraft(prev => ({') &&
    generationHandler.includes('...prev,') &&
    generationHandler.includes(
      'suggestedQuestions: mergeGeneratedQuestions(prev.suggestedQuestions, result.questions)'
    ),
  'Generated questions should merge into the latest draft without replacing edited title or message.'
);

console.log('Opening statement draft preservation checks passed.');
