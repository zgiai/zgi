# AGENTS.md

## Scope

These instructions apply to the Next.js frontend under `web/`.

## Frontend Commands

Run commands from `web/`:

```bash
pnpm install
pnpm dev
pnpm lint
pnpm type-check
pnpm build
```

The project uses `pnpm@10.12.1`.

## Architecture

- `src/app/` contains Next.js App Router routes.
- `src/components/` contains reusable UI and feature components.
- `src/hooks/` contains React hooks for data access and feature logic.
- `src/services/` contains API clients and service-layer types.
- `src/store/` contains client-side global state.
- `src/i18n/` contains localization modules.
- `src/customer/` contains customer overlay support prepared by scripts.

## TypeScript and React

- Keep TypeScript strict. Avoid `any` unless the boundary is genuinely dynamic and document the reason in code.
- Prefer existing service, hook, and component patterns over introducing new state or data-fetching layers.
- Use React Query for server state and Zustand only for shared client state.
- Keep components focused. Move reusable behavior into hooks and reusable presentation into components.
- Keep server/client component boundaries explicit with `use client` only when required.

## UI Guidelines

- Reuse existing components from `src/components/ui` and established feature components.
- Keep layouts responsive and avoid text overflow in buttons, tabs, cards, and dense panels.
- Use the existing icon and design language already present in the feature area.
- Do not introduce unrelated visual redesigns while making functional changes.

## API and Data

- Keep request/response types close to the service layer when they are API-specific.
- Preserve existing error handling and loading states.
- Keep auth, workspace, organization, and quota behavior consistent across feature areas.

## Internationalization

- Add or update translations when user-facing text changes.
- Do not hardcode new visible strings in only one locale when the surrounding feature is localized.

## Validation

- Run `pnpm lint` and `pnpm type-check` for frontend changes when practical.
- Run `pnpm build` for routing, configuration, or production bundling changes.
