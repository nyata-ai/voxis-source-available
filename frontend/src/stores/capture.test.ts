import { beforeEach, describe, expect, it } from 'vitest'
import { useCaptureStore } from './capture'

describe('Voxis-OSS capture store', () => {
  beforeEach(() => {
    useCaptureStore.setState({ open: false, tab: 'upload' })
  })

  it('opens only upload and standard recording tabs', () => {
    useCaptureStore.getState().openCapture('record')
    expect(useCaptureStore.getState()).toMatchObject({ open: true, tab: 'record' })

    useCaptureStore.getState().setTab('upload')
    expect(useCaptureStore.getState().tab).toBe('upload')
  })
})
