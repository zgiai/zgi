// Render the default console experience directly. Avoiding a redirect here keeps
// the root entry stable during auth/context hydration and prevents redirect loops.
export { default } from './work/chat/page';
