import { create } from 'zustand'
import type { CaptureTab } from '@/config/routes'
import { useUploadStore } from '@/stores/upload'

interface CaptureState {
  open: boolean
  tab: CaptureTab
  openCapture: (tab: CaptureTab) => void
  closeCapture: () => void
  setTab: (tab: CaptureTab) => void
  setOpen: (open: boolean) => void
}

// Single source of truth for the unified capture dialog. Every entry point —
// header quick actions, the dashboard ActionTriad, and the `?capture=` deep
// link — drives this one store, so there is exactly one live modal tree with
// one set of form state instead of one per trigger.
//
// `tab` survives close on purpose: reopening returns the user to the channel
// they last used, and the deep-link/trigger paths always pass an explicit tab.
export const useCaptureStore = create<CaptureState>((set) => ({
  open: false,
  tab: 'upload',
  openCapture: (tab) => set({ open: true, tab }),
  closeCapture: () => set({ open: false }),
  setTab: (tab) => set({ tab }),
  setOpen: (open) => set({ open }),
}))

// Dismiss the dialog, clearing the upload queue only once every item has
// settled. Closing does not cancel the XHRs behind the queue, and clearing it
// mid-flight would throw away the only record of them: their progress rows,
// their eventual errors, and the media ids they resolve to. Uploads that
// outlive the dialog keep running, reappear on the next open, and surface in
// the activity band as they complete. A queued row is busy too: it is either
// waiting for its turn (the dropzone sends a bounded number at once) or
// parked while the server is out of upload capacity, and clearing it would
// silently drop a file the user handed us. Shared by the dialog's own dismiss
// path and the upload tab's navigate-to-file paths, which must not diverge.
export function dismissCapture() {
  const uploads = useUploadStore.getState()
  const busy = uploads.queue.some(
    (item) => item.status === 'queued' || item.status === 'uploading' || item.status === 'processing'
  )
  if (!busy) uploads.reset()
  useCaptureStore.getState().closeCapture()
}
