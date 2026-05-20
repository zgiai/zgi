// Type declaration for MDX files imported as React components
declare module '*.mdx' {
  import type { ComponentType } from 'react';
  import type { MDXComponents } from 'mdx/types';

  interface MDXProps {
    components?: MDXComponents;
    apibase?: string;
  }

  const MDXComponent: ComponentType<MDXProps>;
  export default MDXComponent;
}
