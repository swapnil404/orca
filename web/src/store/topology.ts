import { create } from 'zustand'
import type { ProjectStateSnapshot } from '../types/resources'

interface TopologyStore {
  snapshot: ProjectStateSnapshot | null
  connected: boolean
  replaceSnapshot: (snapshot: ProjectStateSnapshot) => void
  setConnected: (connected: boolean) => void
  reset: () => void
}

export const useTopologyStore = create<TopologyStore>((set) => ({
  snapshot: null,
  connected: false,
  replaceSnapshot: (snapshot) => set({ snapshot }),
  setConnected: (connected) => set({ connected }),
  reset: () => set({ snapshot: null, connected: false }),
}))
