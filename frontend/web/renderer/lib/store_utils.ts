import { type StateCreator, create } from 'zustand'
import { subscribeWithSelector } from 'zustand/middleware'

/**
 * add subscribe store
 * ```tsx
const useUserSelectStore = createSubsStore<UserSelectStore>((set, get) => ({})
useUserSelectStore.subscribe(
  (state) => [state.var1, state.var2, state.var3],
  ([var1, var2, var3]) => {}
)
 * ```
 */
export const createSubsStore = <S = any>(stateCreatorFn: StateCreator<S>) => {
  return create<S>()(subscribeWithSelector(stateCreatorFn))
}

type ExtractProperties<T> = {
  [K in keyof T]?: T[K]
}

export type GroupScopeStore<S, A = Partial<ExtractProperties<S>>> = (
  set: (
    partial: S | Partial<S> | ((state: S) => S | Partial<S>),
    replace?: boolean | undefined,
  ) => void,
  get: () => S,
) => A
