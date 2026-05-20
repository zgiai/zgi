# AI Agent Guide

> This document is designed for AI coding assistants (Cursor, Windsurf, GitHub Copilot, Claude Code, etc.) to understand and work with this codebase effectively.

## Project Overview

**ZGI Web Platform** - A production-ready AI workflow platform built with Next.js, enabling users to create, manage, and execute AI-powered workflows with real-time streaming capabilities.

| Tech | Version | Purpose |
|------|---------|---------|
| Next.js | 16.1.6 | App Router, RSC, API Routes |
| React | 19.2.1 | UI Library |
| TypeScript | 5.x | Type Safety |
| Tailwind CSS | 3.4.1 | Styling |
| Radix UI | Latest | UI Primitives |
| Zustand | 5.0.2 | Global State Management |
| React Query | 5.80.7 | Server State Management |
| next-intl | 4.1.2 | Internationalization |
| @xyflow/react | 12.8.4 | Workflow Visual Editor |
| Monaco Editor | 0.53.0 | Code Editing |
| Tiptap | 3.4.4 | Rich Text Editing |
| @dnd-kit | Latest | Drag and Drop |

## Directory Structure

```
src/
├── app/                    # Next.js App Router
│   ├── (auth)/            # Public auth route group (login, register)
│   ├── api/               # API Route Handlers
│   ├── console/           # Admin console routes
│   ├── dashboard/         # Dashboard routes
│   ├── profile/           # User profile pages
│   └── webapp/            # Web application routes
├── components/            # Feature-based components
│   ├── ui/               # Radix UI primitives (DO NOT MODIFY)
│   ├── agents/           # Agent management components
│   ├── workflow/         # Workflow editor and execution UI
│   ├── chat/             # Chat interface components
│   ├── datasets/         # Dataset management UI
│   └── common/           # Shared components
├── config/               # Application configuration
├── constants/            # Application constants and enums
├── hooks/                # Custom React hooks (domain-organized)
│   ├── agent/           # Agent-related hooks
│   ├── workflow/        # Workflow hooks (streaming, execution)
│   ├── dataset/         # Dataset operations
│   ├── auth/            # Authentication hooks
│   └── [domain]/        # Other domain-specific hooks
├── i18n/                 # Internationalization
│   └── modules/         # Per-module translations (en-US, zh-Hans)
├── lib/                  # Third-party library configurations
│   ├── http/            # HTTP client setup (Axios wrapper)
│   └── mocks/           # Mock data for development
├── providers/            # React context providers
├── services/             # API service layer (one file per domain)
├── store/                # Zustand stores (global state)
├── styles/               # Global styles and theme system
│   └── themes/          # Theme variant files (6 themes)
├── types/                # TypeScript type definitions
└── utils/                # Pure utility functions
```

## Architecture Patterns

### 1. Layered Architecture

The project follows a strict **Service-Hook-Component** pattern:

```
UI Components (React)
        ↓
Custom Hooks (React Query + Domain Logic)
        ↓
Services Layer (API Communication)
        ↓
HTTP Client (Axios wrapper)
        ↓
Backend API
```

**Critical Rules:**
- **Components**: Never call services directly. Always use hooks.
- **Hooks**: Encapsulate React Query logic and business rules.
- **Services**: Pure functions, no React dependencies, stateless.
- **Utils**: Pure functions, no side effects, no React/API calls.

### AI Model Selection

- Never hardcode AI model names in runtime frontend code as hidden defaults.
- If a feature lets users pick a model, send only that explicit user selection.
- If no model is selected, omit the model field and let the backend resolve the organization/workspace default model.
- Literal model names are acceptable only in tests, static examples, documentation, or seed/template content where they are clearly not runtime fallbacks.

### 2. Feature-First Organization

Code is organized by **domain/feature**, not by technical type:

```
✅ CORRECT:
hooks/
  └── workflow/
      ├── use-run-workflow.ts
      ├── use-workflow-nodes.ts
      └── use-run-workflow-draft-stream.ts

❌ WRONG:
hooks/
  ├── use-run-workflow.ts
  ├── use-workflow-nodes.ts
  └── use-run-workflow-draft-stream.ts
```

### 3. Workflow System Architecture

The workflow system is the **core feature** of this platform:

#### Visual Workflow Editor
- Built with `@xyflow/react` (node-based visual editor)
- Real-time node manipulation with drag-and-drop
- Custom node types for different AI operations
- Connection validation and data flow management

#### Workflow Execution
- **Draft Mode**: Test execution without saving
- **Production Mode**: Saved workflow execution
- **Streaming Execution**: Real-time SSE (Server-Sent Events) for live updates
- **HTTP Nodes**: REST API integration within workflows

#### Key Files:
- `src/components/workflow/` - All workflow UI components
- `src/hooks/workflow/use-run-workflow-draft-stream.ts` - Streaming execution
- `src/services/workflow.service.ts` - Workflow API service
- `src/components/workflow/ui/workflow-run-panel/` - Execution panel

### 4. State Management Strategy

We use a **hybrid state management** approach:

| State Type | Tool | Use Case | Example |
|------------|------|----------|---------|
| **Global State** | Zustand | App-wide concerns | `auth-store.ts`, `workspace-store.ts` |
| **Server State** | React Query | API data caching | All hooks in `src/hooks/` |
| **Local State** | React Hooks | Component-specific | Form inputs, modals |
| **UI State** | Zustand | Cross-component UI | `ui-store.ts` (sidebar, modals) |

**State Management Rules:**
- Use **React Query** for all server data (GET, POST, PUT, DELETE).
- Use **Zustand** only for global client state (auth, workspace context, UI state).
- Never duplicate server data in Zustand stores.
- Keep stores "dumb" - no async logic in stores, use hooks instead.

### 5. Internationalization (i18n)

The project uses a sophisticated i18n system built on `next-intl` with **custom wrapper functions** for a unified translation interface.

#### Architecture Overview:
- **Built on**: `next-intl` library with custom `useT` and `getT` wrappers
- **27 translation modules**: Organized by domain/feature
- **Lazy loading**: Dynamic imports for optimal performance
- **Type-safe**: Full TypeScript autocomplete and compile-time validation
- **Dual interface**: Supports both dot notation and namespace-based access

#### Supported Languages:
- `en-US` - English
- `zh-Hans` - Simplified Chinese (default)

#### File Structure:
```
i18n/
├── config.ts              # Locale configuration and detection
├── loader.ts              # Dynamic module loading registry
├── translations.ts        # Custom useT and getT implementations
└── modules/               # Translation modules by domain
    ├── common/
    │   ├── en-US.ts
    │   └── zh-Hans.ts
    ├── agents/
    │   ├── en-US.ts
    │   └── zh-Hans.ts
    ├── workflow/
    │   ├── en-US.ts
    │   └── zh-Hans.ts
    └── [27 total modules]
```

#### Available Modules:
The project has 27 translation modules defined in `AVAILABLE_MODULES`:
- **Core**: common, navigation, auth, ui, home
- **Features**: agents, workflow, nodes, datasets, dbs, files, webapp
- **Admin**: dashboard, settings, users, workspace, enterprise
- **Integrations**: aiProviders, models, channels, apikeys, market
- **User**: profile

#### Pattern Selection Guide:

**Choose your translation pattern based on usage density:**

| Calls from Same Module | Recommended Pattern | Example |
|------------------------|---------------------|---------|
| **5+ calls** | **Scoped mode** | `const t = useT('nodes')` then `t('httpRequest.method')` |
| **1-4 calls** or **multi-module** | **Dot notation** | `const t = useT()` then `t('nodes.httpRequest.method')` |
| **Existing code only** | **Namespace (deprecated)** | `t.nodes('httpRequest.method')` ⚠️ |

**Why this matters:**
- **Scoped mode** (5+ calls): Cleaner code, better performance, clear context
- **Dot notation** (1-4 calls): Full key visibility, easy to understand
- **Namespace pattern**: Backward compatible but NOT recommended for new code

#### Usage Patterns:

**Pattern 1: Scoped Mode (5+ calls from same module)**

Best for components with dense translation usage from a single module.

```tsx
import { useT } from '@/i18n/translations';

function HttpRequestManager() {
  // ✅ RECOMMENDED: Scoped to 'nodes' module
  const t = useT('nodes');

  return (
    <div>
      <h3>{t('httpRequest.section.method')}</h3>
      <Input placeholder={t('httpRequest.placeholders.url')} />
      <Button>{t('httpRequest.actions.send')}</Button>
      <Label>{t('httpRequest.fields.headers')}</Label>
      {/* 30+ more t() calls - all from 'nodes' module */}
    </div>
  );
}
```

**Pattern 2: Dot Notation (1-4 calls or multi-module)**

Best for sparse usage or when accessing multiple modules.

```tsx
import { useT } from '@/i18n/translations';

function WorkflowCard() {
  // ✅ RECOMMENDED: Unified access for multi-module usage
  const t = useT();

  return (
    <div>
      <h3>{t('workflow.title')}</h3>
      <Badge>{t('common.status.active')}</Badge>
      <Button>{t('agents.actions.run')}</Button>
    </div>
  );
}
```

**Pattern 3: Namespace (Deprecated - backward compatible only)**

```tsx
import { useT } from '@/i18n/translations';

function LegacyComponent() {
  const t = useT();

  // ⚠️ DEPRECATED: Don't use in new code
  return <button>{t.nodes('httpRequest.method')}</button>;

  // ✅ INSTEAD: Use scoped mode if 5+ calls, or dot notation if 1-4 calls
}
```

**Server Components:**
```tsx
import { getT } from '@/i18n/translations';

export default async function Page() {
  // Same patterns apply
  const t = await getT('workflow');  // Scoped mode
  // OR
  const t = await getT();            // Dot notation

  return <h1>{t('editor.title')}</h1>;
}
```

#### Migration Examples:

**Example 1: HTTP Request Manager (36 calls from 'nodes' module)**

❌ **BEFORE** (Namespace pattern - inefficient for dense usage):
```tsx
const t = useT();

<h3>{t.nodes('httpRequest.section.method')}</h3>
<Input placeholder={t.nodes('httpRequest.placeholders.url')} />
<Button>{t.nodes('httpRequest.actions.send')}</Button>
<Label>{t.nodes('httpRequest.fields.headers')}</Label>
<Select placeholder={t.nodes('httpRequest.fields.method')} />
// ... 31 more t.nodes() calls
```

✅ **AFTER** (Scoped mode - cleaner and more efficient):
```tsx
const t = useT('nodes');  // Single scoped hook

<h3>{t('httpRequest.section.method')}</h3>
<Input placeholder={t('httpRequest.placeholders.url')} />
<Button>{t('httpRequest.actions.send')}</Button>
<Label>{t('httpRequest.fields.headers')}</Label>
<Select placeholder={t('httpRequest.fields.method')} />
// ... 31 more t() calls - no module prefix needed
```

**Benefits**: ~252 characters saved (15% reduction), clearer intent, better performance

**Example 2: Workflow Card (3 calls from different modules)**

✅ **CORRECT** (Dot notation - perfect for multi-module usage):
```tsx
const t = useT();

<h3>{t('workflow.title')}</h3>
<Badge>{t('common.status.active')}</Badge>
<Button>{t('agents.actions.run')}</Button>
```

❌ **DON'T DO** (Scoped mode with multiple modules - defeats the purpose):
```tsx
const tWorkflow = useT('workflow');
const tCommon = useT('common');
const tAgents = useT('agents');

<h3>{tWorkflow('title')}</h3>
<Badge>{tCommon('status.active')}</Badge>
<Button>{tAgents('actions.run')}</Button>
```

#### Type Safety Features:

The i18n system provides comprehensive TypeScript support:

- **`AllTranslationKeys`**: Union type of all valid dot-notation keys
- **`UnifiedTranslations`**: Hybrid type supporting both dot notation and namespace access
- **Per-namespace types**: `NodesSuffix`, `AgentsSuffix`, `WorkflowSuffix`, etc.
- **Full autocomplete**: IDE autocomplete for all translation keys
- **Compile-time validation**: Invalid keys cause TypeScript errors

#### Key Files:
- `src/i18n/translations.ts` - Custom `useT` and `getT` implementations
- `src/i18n/loader.ts` - Module registry and dynamic loading functions
- `src/i18n/config.ts` - Locale configuration and detection logic
- `src/i18n/modules/index.ts` - TypeScript type definitions

**Critical i18n Rules:**
- **Zero hardcoded strings**: All user-facing text MUST use `useT()` or `getT()`
- **Import from correct location**: Use `@/i18n/translations`, NOT `next-intl` directly
- **Add to both languages**: When adding a key, update both `en-US.ts` and `zh-Hans.ts`
- **Domain organization**: Add keys to the appropriate module (workflow keys → `workflow/`)
- **Pattern selection** (NEW):
  - ✅ Use **scoped mode** (`useT('moduleName')`) for components with **5+ calls** from same module
  - ✅ Use **dot notation** (`t('module.key')`) for **1-4 calls** or multi-module usage
  - ⚠️ **Namespace pattern deprecated** (`t.module('key')`): Backward compatible but NOT for new code
- **Type safety**: Invalid keys will cause TypeScript errors at compile time

### 6. Editor Integrations

#### Monaco Editor (Code Editing)
Used for:
- API request/response editing in HTTP workflow nodes
- JSON/YAML configuration editing
- Code snippets in agent configurations

**Implementation Pattern:**
```tsx
import { Editor } from '@monaco-editor/react';

function CodeEditor({ value, onChange }: CodeEditorProps) {
  return (
    <Editor
      height="400px"
      defaultLanguage="json"
      value={value}
      onChange={onChange}
      theme="vs-dark"
      options={{
        minimap: { enabled: false },
        fontSize: 14,
      }}
    />
  );
}
```

#### Tiptap (Rich Text Editing)
Used for:
- Chat interfaces
- Agent descriptions
- Markdown content editing

**Performance Rule:**
- Always use `dynamic` import for editors (they are large bundles):
```tsx
import dynamic from 'next/dynamic';

const MonacoEditor = dynamic(() => import('./monaco-editor'), {
  ssr: false,
  loading: () => <Skeleton className="h-64" />,
});
```

### 7. Multi-Tenant Architecture

The platform supports **organization-based multi-tenancy**:

- **Organization**: Top-level isolation
- **Workspace**: Namespace within organization
- **Permissions**: Role-based access control

**Key Stores:**
- `organization-store.ts` - Current organization context
- `workspace-store.ts` - Current workspace context
- `auth-store.ts` - User authentication and permissions

**Pattern:**
- Always include organization/workspace context in API calls
- Use `useOrganization()` and `useWorkspace()` hooks to access context
- Validate permissions before rendering sensitive UI

## Code Conventions

### Must Follow

- **Package manager**: `pnpm` only
- **Comments**: English only
- **File naming**: `kebab-case.tsx` for components, `use-*.ts` for hooks
- **Hot reload**: Do NOT restart dev server (auto-updates with Turbopack)
- **No emoji**: Only use emoji if user explicitly requests it

### Import Order

```typescript
// 1. React/Next imports
import { useState } from 'react';
import Link from 'next/link';
import dynamic from 'next/dynamic';

// 2. Third-party libraries
import { useQuery } from '@tanstack/react-query';
import { useTranslations } from 'next-intl';

// 3. Internal imports (absolute paths with @/ alias)
import { Button } from '@/components/ui/button';
import { useWorkflow } from '@/hooks/workflow/use-workflow';
import { workflowService } from '@/services/workflow.service';
```

### Component Standards

#### Atomic Component Contract

All components MUST follow these rules:

- **Named Exports**: Always use named exports. Do NOT use `export default`.
  ```tsx
  // ✅ Correct
  export function WorkflowNode({ ... }: WorkflowNodeProps) { ... }

  // ❌ Wrong
  export default function WorkflowNode({ ... }: WorkflowNodeProps) { ... }
  ```

- **Strict Typing**: Always define an interface for props named `[ComponentName]Props`.
  ```tsx
  interface WorkflowNodeProps {
    id: string;
    type: string;
    data: NodeData;
  }

  export function WorkflowNode({ id, type, data }: WorkflowNodeProps) { ... }
  ```

- **RSC First Strategy**:
  - Components are **Server Components** by default.
  - If a component needs interactivity (hooks or events), add `'use client'` directive.
  - Extract interactive logic into **small, leaf-level Client Components**.
  - Avoid turning large feature components into Client Components.

- **Zero Hardcoded Strings**: All user-facing text must use `useTranslations`.

- **Icon Consistency**: Use `lucide-react`. Standardize size with `size-4` (16px) or `size-5` (20px).

#### Component Annotation Standard

All components MUST include a JSDoc header:

```typescript
/**
 * @component WorkflowExecutionPanel
 * @category Feature
 * @status Stable
 * @description Real-time workflow execution panel with streaming logs and result display
 * @usage Use in workflow run pages to show execution progress
 * @example
 * <WorkflowExecutionPanel workflowId={id} runId={runId} />
 */
export function WorkflowExecutionPanel({ workflowId, runId }: WorkflowExecutionPanelProps) {
  // ...
}
```

#### Performance Optimization Rules

Following React best practices for optimal performance:

- **Dynamic Imports**: Use `next/dynamic` for components > 50KB (editors, charts, heavy visualizations):
  ```tsx
  const WorkflowEditor = dynamic(() => import('./workflow-editor'), {
    loading: () => <Skeleton className="h-96" />,
  });
  ```

- **React.memo**: Use for expensive child components with stable props:
  ```tsx
  export const WorkflowNodeList = React.memo(function WorkflowNodeList({ nodes }: Props) {
    // Heavy rendering logic
  });
  ```

- **Conditional Rendering**: Use ternary operators, not `&&`:
  ```tsx
  // ✅ Correct
  {isVisible ? <Component /> : null}

  // ❌ Avoid (may render '0' or 'false')
  {isVisible && <Component />}
  ```

- **RSC Caching**: Use `React.cache()` for per-request deduplication in Server Components:
  ```tsx
  import { cache } from 'react';

  export const getWorkflow = cache(async (id: string) => {
    return await workflowService.getById(id);
  });
  ```

## Service-Hook-Type Pattern

All data handling MUST follow this strict layered architecture.

### 1. Define Types (`src/types/*.ts`)

All data structures must be strictly typed.

```typescript
// src/types/workflow.ts
export interface Workflow {
  id: string;
  name: string;
  description: string;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  status: 'draft' | 'published';
}

export interface CreateWorkflowRequest {
  name: string;
  description?: string;
}

export interface UpdateWorkflowRequest {
  name?: string;
  description?: string;
  nodes?: WorkflowNode[];
  edges?: WorkflowEdge[];
}
```

### 2. Implement Stateless Service (`src/services/*.ts`)

Services are pure functional objects:
- **Stateless**: No state, no hooks
- **HTTP Client**: Use the configured HTTP client from `src/lib/http/`
- **Simple wrappers**: Just API call wrappers with type safety

```typescript
// src/services/workflow.service.ts
import { httpClient } from '@/lib/http';
import type { Workflow, CreateWorkflowRequest, UpdateWorkflowRequest } from '@/types/workflow';

export const workflowService = {
  getAll: () => httpClient.get<Workflow[]>('/workflows'),

  getById: (id: string) => httpClient.get<Workflow>(`/workflows/${id}`),

  create: (data: CreateWorkflowRequest) => httpClient.post<Workflow>('/workflows', data),

  update: (id: string, data: UpdateWorkflowRequest) =>
    httpClient.put<Workflow>(`/workflows/${id}`, data),

  delete: (id: string) => httpClient.delete(`/workflows/${id}`),
};
```

### 3. Implement Encapsulated Hooks (`src/hooks/*.ts`)

Hooks manage React Query state and side effects.

#### Query Key Factory Pattern

Always define query keys using a factory:

```typescript
// src/hooks/workflow/query-keys.ts
export const workflowKeys = {
  all: ['workflows'] as const,
  lists: () => [...workflowKeys.all, 'list'] as const,
  list: (filters: WorkflowFilters) => [...workflowKeys.lists(), filters] as const,
  details: () => [...workflowKeys.all, 'detail'] as const,
  detail: (id: string) => [...workflowKeys.details(), id] as const,
  runs: (workflowId: string) => [...workflowKeys.detail(workflowId), 'runs'] as const,
};
```

#### Query Hooks

```typescript
// src/hooks/workflow/use-workflow.ts
import { useQuery } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import { workflowKeys } from './query-keys';

export function useWorkflow(id: string) {
  return useQuery({
    queryKey: workflowKeys.detail(id),
    queryFn: () => workflowService.getById(id),
    enabled: !!id,
  });
}

export function useWorkflows(filters?: WorkflowFilters) {
  return useQuery({
    queryKey: workflowKeys.list(filters || {}),
    queryFn: () => workflowService.getAll(filters),
  });
}
```

#### Mutation Hooks with Optimistic Updates

```typescript
// src/hooks/workflow/use-update-workflow.ts
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { workflowService } from '@/services/workflow.service';
import { workflowKeys } from './query-keys';

export function useUpdateWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateWorkflowRequest }) =>
      workflowService.update(id, data),

    // Step 1: Optimistic update
    onMutate: async ({ id, data }) => {
      // Cancel outgoing refetches
      await queryClient.cancelQueries({ queryKey: workflowKeys.detail(id) });

      // Snapshot previous value
      const previous = queryClient.getQueryData(workflowKeys.detail(id));

      // Optimistically update
      if (previous) {
        queryClient.setQueryData(workflowKeys.detail(id), {
          ...(previous as Workflow),
          ...data,
        });
      }

      return { previous };
    },

    // Step 2: Rollback on error via invalidation
    onError: (err, { id }, context) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.detail(id) });
      toast.error(err.message);
    },

    // Step 3: Final synchronization
    onSettled: (data, err, { id }) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: workflowKeys.lists() });
    },

    onSuccess: () => {
      toast.success('Workflow updated successfully');
    },
  });
}
```

**Key Strategies:**
- **Cancel outgoing refetches** in `onMutate` to prevent race conditions
- **Rollback via Invalidation**: Simpler and more robust than manual restoration
- **Always Sync**: Final invalidation in `onSettled` ensures perfect alignment with server
- **User Feedback**: Always show toast notifications for mutations

## Theme System

The project uses a sophisticated **OKLCH-based theme system** with 6 built-in themes.

### Available Themes:
1. **Light** - Default light theme
2. **Dark** - Dark mode theme
3. **Ocean Blue** - Professional blue theme
4. **Nature Green** - Fresh green theme
5. **Royal Purple** - Creative purple theme
6. **High Contrast** - Accessibility-focused theme

### Theme Architecture:

```
src/styles/
├── index.css              # Main entry point
└── themes/                # Individual theme files
    ├── light.css
    ├── dark.css
    ├── blue.css
    ├── green.css
    ├── purple.css
    └── high-contrast.css
```

### Usage in Components:

Always use **semantic color classes**, never raw color values:

```tsx
// ✅ Correct - Semantic tokens
<div className="bg-background text-foreground border-border">
  <h1 className="text-primary">Heading</h1>
  <p className="text-muted-foreground">Description</p>
</div>

// ❌ Wrong - Hardcoded colors
<div className="bg-white text-black border-gray-300">
  <h1 className="text-blue-600">Heading</h1>
</div>
```

### Semantic Color Tokens:

| Token | Purpose |
|-------|---------|
| `background` | Main canvas background |
| `foreground` | Main text color |
| `primary` | Primary brand color |
| `secondary` | Secondary elements |
| `muted` | Muted backgrounds |
| `muted-foreground` | Muted text |
| `accent` | Accent backgrounds |
| `accent-foreground` | Accent text |
| `destructive` | Destructive actions (delete, error) |
| `border` | Border colors |
| `input` | Input borders |
| `ring` | Focus rings |

## Tooling & Utility Standards (ARW)

### The "Search First" Rule

Before writing a new utility function or React hook, you **MUST**:

1. **Check `src/utils/`**: Scan existing utilities by domain
2. **Check `src/hooks/[domain]/`**: Browse existing hooks in the relevant domain folder
3. **Check Approved Libraries**: Verify if functionality exists in:
   - `date-fns`: For all date manipulations
   - `lodash-es`: For complex object/array operations (use sparingly, prefer native)
   - `zod`: For schema validation

### Implementation Priority

Follow this order:
1. **Native Web APIs**: `Intl`, `URL`, `Crypto`, `Blob`, `File`, etc.
2. **Existing Project Utils/Hooks**: Reuse from `src/utils` or `src/hooks`
3. **Approved Third-Party Libraries**: Use existing dependencies from `package.json`
4. **Custom Implementation**: Only if above options are exhausted

### Utility Discovery Map

Common utilities already available:

| Need | Check First | File |
|------|-------------|------|
| Array operations | `src/utils/array.ts` | `chunk`, `unique`, `groupBy` |
| Object operations | `src/utils/object.ts` | `pick`, `omit`, `deepMerge` |
| String formatting | `src/utils/string.ts` | `capitalize`, `truncate`, `slugify` |
| Number formatting | `src/utils/number.ts` | `formatCurrency`, `formatPercent` |
| Date operations | `src/utils/date.ts` | `formatDate`, `parseDate`, `relativeTime` |
| File operations | `src/utils/file-helpers.ts` | `downloadFile`, `getFileExtension` |
| Debouncing | `src/utils/debounce.ts` | `debounce`, `throttle` |
| Validation | `src/utils/validation.ts` | Email, URL, phone validators |
| DOM utilities | `src/utils/dom.ts` | `copyToClipboard`, `downloadBlob` |
| JWT operations | `src/utils/jwt.ts` | `decodeToken`, `isTokenExpired` |

### Hook Discovery Map

| Need | Check First | Directory |
|------|-------------|-----------|
| Workflow operations | `src/hooks/workflow/` | Execution, nodes, streaming |
| Agent operations | `src/hooks/agent/` | CRUD, marketplace |
| Dataset operations | `src/hooks/dataset/` | CRUD, folders |
| Authentication | `src/hooks/auth/` | Login, logout, session |
| File operations | `src/hooks/use-upload.ts` | Upload, download |
| Mobile detection | `src/hooks/use-mobile.ts` | Responsive detection |
| Debounced values | `src/hooks/use-debounced-value.ts` | Input debouncing |
| Infinite scroll | `src/hooks/use-infinite-observer.ts` | Pagination |
| Localization | `src/hooks/use-locale.ts` | Language switching |

### Contract for New Additions

**Utils:**
- Must be pure functions in `src/utils/[category].ts`
- Must have JSDoc comments with `@util` tag
- Must be exported via `index.ts` if intended for public use
- Must have unit tests (if adding test infrastructure)

**Hooks:**
- Must be in `src/hooks/[domain]/use-[purpose].ts`
- Must follow React Hook rules (naming, dependencies)
- Must have JSDoc comments with `@hook` tag
- Must encapsulate React Query logic (never expose query/mutation objects directly)

## Common Tasks

### Adding a New Workflow Node Type

1. Define the node type in `src/types/workflow.ts`:
   ```typescript
   export type WorkflowNodeType = 'http' | 'llm' | 'condition' | 'transform' | 'new-type';
   ```

2. Create the node component in `src/components/workflow/nodes/`:
   ```tsx
   // src/components/workflow/nodes/new-type-node.tsx
   export function NewTypeNode({ id, data }: NewTypeNodeProps) {
     // Node implementation
   }
   ```

3. Register the node in the workflow editor configuration

4. Add translations for the node in `src/i18n/modules/workflow/`:
   ```typescript
   // en-US.ts
   export default {
     nodes: {
       newType: {
         title: 'New Type Node',
         description: 'Description of the new node type',
       },
     },
   };
   ```

### Adding a New API Endpoint Integration

1. Define types in `src/types/[domain].ts`
2. Add service methods in `src/services/[domain].service.ts`
3. Create hooks in `src/hooks/[domain]/`
4. Update query keys in `src/hooks/[domain]/query-keys.ts`

### Adding a New Page

1. Create file in `src/app/[route]/page.tsx`
2. Add route constants to `src/constants/routes.ts` (if exists)
3. Add translations to `src/i18n/modules/[module]/en-US.ts` and `zh-Hans.ts`
4. Add navigation links in appropriate components

### Adding a New UI Component

1. **Radix UI primitives** → Use shadcn/ui CLI: `npx shadcn@latest add [component]`
   - These always go in `src/components/ui/`
   - **DO NOT modify** these files after installation

2. **Feature components** → Create in `src/components/[domain]/`
   - Organize by functional domain (e.g., `src/components/workflow/`)
   - Use named exports
   - Add JSDoc header

3. **Common components** → Generic components in `src/components/common/`
   - Reusable across multiple domains
   - No business logic

### Implementing Real-Time Streaming

For workflow execution or chat streaming:

1. Use Server-Sent Events (SSE) pattern
2. Create a streaming hook (see `use-run-workflow-draft-stream.ts` as reference)
3. Handle connection lifecycle (open, message, error, close)
4. Implement proper cleanup in `useEffect` return

Example pattern:
```typescript
export function useStreamingExecution(workflowId: string) {
  const [status, setStatus] = useState<'idle' | 'streaming' | 'complete' | 'error'>('idle');
  const [messages, setMessages] = useState<StreamMessage[]>([]);

  useEffect(() => {
    if (!workflowId) return;

    const eventSource = new EventSource(`/api/workflows/${workflowId}/stream`);

    eventSource.onmessage = (event) => {
      const message = JSON.parse(event.data);
      setMessages((prev) => [...prev, message]);
    };

    eventSource.onerror = () => {
      setStatus('error');
      eventSource.close();
    };

    return () => {
      eventSource.close();
    };
  }, [workflowId]);

  return { status, messages };
}
```

## Environment Variables

The project uses environment variables for configuration.

### Common Variables:
- `NEXT_PUBLIC_API_URL` - Backend API base URL
- `NEXT_PUBLIC_APP_URL` - Frontend application URL
- `NEXT_PUBLIC_WS_URL` - WebSocket URL for real-time features

### Adding a New Environment Variable:

1. Add to `.env.example` with description
2. Add to `.env.local` with actual value
3. Document in README.md
4. Use via `process.env.NEXT_PUBLIC_*` in client code
5. Use via `process.env.*` in server code (API routes, Server Components)

**Security Rules:**
- **Client-accessible variables** MUST be prefixed with `NEXT_PUBLIC_`
- **Secrets** (API keys, tokens) must NEVER have `NEXT_PUBLIC_` prefix
- Never commit `.env.local` to version control

## API Error Handling

The project uses a standardized error handling pattern.

### Error Response Format:
```json
{
  "error": "Human-readable error message",
  "code": "ERROR_CODE",
  "details": {}
}
```

### Error Handling in Hooks:

Always handle errors in mutation hooks with user feedback:

```typescript
export function useCreateWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: workflowService.create,
    onError: (error) => {
      // Show error toast
      toast.error(error.message || 'Failed to create workflow');

      // Log to error tracking (Sentry)
      console.error('Workflow creation failed:', error);
    },
    onSuccess: (data) => {
      toast.success('Workflow created successfully');
      queryClient.invalidateQueries({ queryKey: workflowKeys.lists() });
    },
  });
}
```

### Global Error Boundary:

For unexpected errors, the app should have an error boundary. Check `src/app/error.tsx` or `src/components/error-boundary.tsx`.

## Performance Best Practices

### 1. Bundle Size Optimization

**Heavy Components - Always Dynamic Import:**
- Workflow editor (`@xyflow/react`)
- Monaco editor
- Tiptap editor
- Chart libraries (`recharts`)
- Large data visualizations

```tsx
// ✅ Correct
const WorkflowEditor = dynamic(
  () => import('@/components/workflow/workflow-editor'),
  { ssr: false, loading: () => <Skeleton className="h-96" /> }
);

// ❌ Wrong - Will bloat initial bundle
import { WorkflowEditor } from '@/components/workflow/workflow-editor';
```

### 2. Image Optimization

Always use Next.js `Image` component:

```tsx
import Image from 'next/image';

<Image
  src="/avatar.png"
  alt="User avatar"
  width={48}
  height={48}
  className="rounded-full"
/>
```

### 3. Data Fetching Optimization

- Use React Query's `staleTime` to reduce refetches
- Implement pagination for large lists
- Use `useInfiniteQuery` for infinite scroll
- Prefetch data on hover for better UX

```typescript
export function useWorkflows() {
  return useQuery({
    queryKey: workflowKeys.lists(),
    queryFn: workflowService.getAll,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}
```

### 4. Rendering Optimization

- Memoize expensive computations with `useMemo`
- Memoize callbacks with `useCallback`
- Use `React.memo` for pure components with heavy rendering
- Avoid inline object/array creation in JSX props

## Testing Guidelines

(To be implemented)

The project currently lacks a comprehensive test suite. When adding tests:

- Place tests in `src/__tests__/` or co-locate with components
- Use Jest + React Testing Library
- Test user behavior, not implementation details
- Mock API calls with MSW (Mock Service Worker)

## Do NOT

- ❌ Modify files in `src/components/ui/` (shadcn/ui managed)
- ❌ Use hardcoded strings in UI (always use translations)
- ❌ Call services directly from components (use hooks)
- ❌ Add business logic to stores (keep stores simple)
- ❌ Use `export default` (use named exports)
- ❌ Import from `src/` (use `@/` alias)
- ❌ Add Chinese comments (English only)
- ❌ Generate `.sh` or `.md` files unless explicitly requested
- ❌ Use `&&` for conditional rendering (use ternary)
- ❌ Skip translations when adding new UI text
- ❌ Restart dev server (hot reload handles changes)

## Quick Reference

| Action | Command |
|--------|---------|
| Install deps | `pnpm install` |
| Dev server | `pnpm dev` |
| Build | `pnpm build` |
| Start production | `pnpm start` |
| Type check | `pnpm type-check` |
| Lint | `pnpm lint` |
| Lint fix | `pnpm lint:fix` |
| Format | `pnpm format` |
| Add UI component | `npx shadcn@latest add [name]` |

## Additional Resources

- **API Documentation**: Check `docs/builtin_tools_api.md` for built-in tools
- **Next.js Docs**: https://nextjs.org/docs
- **React Query Docs**: https://tanstack.com/query/latest
- **Radix UI Docs**: https://www.radix-ui.com/
- **XYFlow Docs**: https://reactflow.dev/

---

**Remember**: This is an AI-powered workflow platform. Prioritize real-time streaming, type safety, and exceptional user experience. When in doubt, check existing patterns in the codebase before implementing new solutions.
