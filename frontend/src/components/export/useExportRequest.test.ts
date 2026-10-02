import { describe, it, expect } from 'vitest'
import { EXPORT_FORMATS, resolveExport } from './useExportRequest'

describe('resolveExport', () => {
  it('offers exactly the three formats the backend renders', () => {
    expect([...EXPORT_FORMATS]).toEqual(['pdf', 'docx', 'json'])
  })

  it('builds the transcription endpoint and drops the source extension', () => {
    expect(
      resolveExport({
        entityType: 'transcription',
        entityId: 'tx-1',
        mediaFilename: 'board-meeting.mp3',
        choice: 'docx',
      }),
    ).toEqual({
      endpoint: '/transcriptions/tx-1/export?format=docx',
      filename: 'board-meeting.docx',
      isBap: false,
    })
  })

  it('builds the summary endpoint', () => {
    expect(
      resolveExport({
        entityType: 'summary',
        entityId: 'sum-9',
        mediaFilename: 'board-meeting.mp3',
        choice: 'pdf',
      }),
    ).toEqual({
      endpoint: '/summaries/sum-9/export?format=pdf',
      filename: 'board-meeting.pdf',
      isBap: false,
    })
  })

  it('sends the BAP draft to the transcription template', () => {
    const resolved = resolveExport({
      entityType: 'transcription',
      entityId: 'tx-1',
      mediaFilename: 'board-meeting.mp3',
      choice: 'bap',
    })
    expect(resolved.endpoint).toBe('/transcriptions/tx-1/export?format=docx&template=bap')
    expect(resolved.filename).toBe('board-meeting-BAP-draf.docx')
    expect(resolved.isBap).toBe(true)
  })

  // The template only exists on the transcription endpoint; sending the
  // parameter to a summary would be rejected.
  it('degrades a BAP choice on a summary to a plain DOCX', () => {
    const resolved = resolveExport({
      entityType: 'summary',
      entityId: 'sum-9',
      mediaFilename: 'board-meeting.mp3',
      choice: 'bap',
    })
    expect(resolved.endpoint).toBe('/summaries/sum-9/export?format=docx')
    expect(resolved.isBap).toBe(false)
  })

  it('falls back to the entity type when there is no filename to work from', () => {
    expect(
      resolveExport({ entityType: 'summary', entityId: 'sum-9', choice: 'json' }).filename,
    ).toBe('summary.json')
    expect(
      resolveExport({
        entityType: 'transcription',
        entityId: 'tx-1',
        mediaFilename: '',
        choice: 'json',
      }).filename,
    ).toBe('transcription.json')
  })

  // Real ids are UUIDs, but the id arrives from a route param and an API
  // payload, and it is spliced straight into a path.
  it('encodes the id rather than splicing it into the path raw', () => {
    expect(
      resolveExport({
        entityType: 'transcription',
        entityId: '../../admin/secrets',
        mediaFilename: 'x.mp3',
        choice: 'pdf',
      }).endpoint,
    ).toBe('/transcriptions/..%2F..%2Fadmin%2Fsecrets/export?format=pdf')

    expect(
      resolveExport({
        entityType: 'summary',
        entityId: 'sum 9?format=docx&template=bap',
        mediaFilename: 'x.mp3',
        choice: 'pdf',
      }).endpoint,
    ).toBe('/summaries/sum%209%3Fformat%3Ddocx%26template%3Dbap/export?format=pdf')
  })
})
