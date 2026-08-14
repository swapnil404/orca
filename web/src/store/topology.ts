import { create } from 'zustand'
import type { ProjectStateSnapshot } from '../types/resources'

interface TopologyStore {
  snapshot: ProjectStateSnapshot | null
  replaceSnapshot: (snapshot: ProjectStateSnapshot) => void
  reset: () => void
}

export const useTopologyStore = create<TopologyStore>((set) => ({
  snapshot: null,
  replaceSnapshot: (snapshot) => set({ snapshot }),
  reset: () => set({ snapshot: null }),
}))
