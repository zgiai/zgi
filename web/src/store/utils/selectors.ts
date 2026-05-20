/**
 * Selector utility for Zustand stores
 * Provides a type-safe way to create selectors for stores
 */
import type { StoreApi, UseBoundStore } from 'zustand';

type WithSelectors<S> = S extends { getState: () => infer T }
  ? S & { use: { [K in keyof T]: () => T[K] } }
  : never;

/**
 * Creates selectors for a Zustand store
 * Allows using individual state slices with automatic memoization
 *
 * @example
 * const useCounterStore = createSelectors(create<CounterState>()((set) => ({
 *   count: 0,
 *   increment: () => set((state) => ({ count: state.count + 1 })),
 * })));
 *
 * // Later in a component:
 * const count = useCounterStore.use.count();
 * const increment = useCounterStore.use.increment();
 */
export function createSelectors<S extends UseBoundStore<StoreApi<object>>>(_store: S) {
  const store = _store as WithSelectors<typeof _store>;
  store.use = {};

  // Get the store's state shape from the store's initial state
  type State = ReturnType<(typeof _store)['getState']>;

  // Create a selector for each property in the store
  const keys = Object.keys(store.getState()) as Array<keyof State>;

  // For each key in the store, create a selector
  for (const key of keys) {
    // @ts-expect-error - We know that the key exists in the store
    store.use[key] = () => store(state => state[key]);
  }

  return store;
}
