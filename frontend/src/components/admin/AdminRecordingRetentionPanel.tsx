import { useEffect, useMemo, useState } from 'react'
import {
  type RecordingRetentionPreview,
  useAdminRecordingRetentionPolicy,
  useAdminRecordingRetentionPreview,
  useUpdateAdminRecordingRetentionPolicy,
} from '@/hooks/useAdminRecordingRetention'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/lib/toast'

function retentionRequest(enabled: boolean, daysInput: string, applyToExisting: boolean) {
  const days = Number.parseInt(daysInput, 10)
  return {
    enabled,
    days: Number.isFinite(days) ? Math.min(30, Math.max(1, days)) : 1,
    apply_to_existing: applyToExisting,
  }
}

function formatDate(value?: string): string {
  if (!value) return ''
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(new Date(value))
}

export function AdminRecordingRetentionPanel() {
  const { data: policy, isLoading, isError } = useAdminRecordingRetentionPolicy()
  const previewMutation = useAdminRecordingRetentionPreview()
  const updateMutation = useUpdateAdminRecordingRetentionPolicy()
  const previewRetention = previewMutation.mutateAsync
  const [enabled, setEnabled] = useState(false)
  const [daysInput, setDaysInput] = useState('7')
  const [applyToExisting, setApplyToExisting] = useState(false)
  const [confirmCount, setConfirmCount] = useState('')
  const [preview, setPreview] = useState<RecordingRetentionPreview | null>(null)
  const request = useMemo(
    () => retentionRequest(enabled, daysInput, applyToExisting),
    [enabled, daysInput, applyToExisting],
  )

  useEffect(() => {
    if (!policy) return
    setEnabled(policy.enabled)
    setDaysInput(String(policy.days))
    setApplyToExisting(policy.apply_to_existing)
  }, [policy])

  useEffect(() => {
    if (!policy) return
    let cancelled = false
    previewRetention(request)
      .then((nextPreview) => {
        if (!cancelled) setPreview(nextPreview)
      })
      .catch(() => {
        if (!cancelled) setPreview(null)
      })
    return () => {
      cancelled = true
    }
  }, [policy, request, previewRetention])

  if (isError) {
    return <p className="text-sm text-destructive">Unable to load recording retention policy.</p>
  }

  if (isLoading || !policy) {
    return (
      <Card>
        <CardContent className="space-y-4 p-4">
          <Skeleton className="h-6 w-64" />
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    )
  }

  const immediateCount = preview?.immediate_delete_count ?? 0
  const confirmRequired = enabled && immediateCount > 0
  const confirmationMatches = Number.parseInt(confirmCount, 10) === immediateCount

  const handleSave = async () => {
    if (confirmRequired && !confirmationMatches) {
      toast.error('Enter the current immediate delete count before saving.')
      return
    }
    try {
      await updateMutation.mutateAsync({
        ...request,
        confirmed_immediate_delete_count: confirmRequired ? immediateCount : 0,
      })
      toast.success('Recording retention policy saved.')
      setConfirmCount('')
    } catch {
      toast.error('Unable to save recording retention policy.')
    }
  }

  return (
    <Card>
      <CardContent className="space-y-6 p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-0.5">
            <Label htmlFor="admin-recording-retention-enabled">Auto-delete live recording audio</Label>
            <p className="text-xs text-muted-foreground">Applies to live recordings for every user.</p>
          </div>
          <Switch
            id="admin-recording-retention-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label="Auto-delete live recording audio"
          />
        </div>

        <div className="grid gap-2 sm:max-w-44">
          <Label htmlFor="admin-recording-retention-days">Delete after</Label>
          <Input
            id="admin-recording-retention-days"
            type="number"
            min={1}
            max={30}
            value={daysInput}
            onChange={(event) => setDaysInput(event.target.value)}
            onBlur={() => setDaysInput(String(request.days))}
            disabled={!enabled}
          />
          <p className="text-xs text-muted-foreground">Choose 1 to 30 days after completion.</p>
        </div>

        <div className="flex items-start gap-3">
          <Checkbox
            id="admin-recording-retention-existing"
            checked={applyToExisting}
            onCheckedChange={(checked) => setApplyToExisting(checked === true)}
            disabled={!enabled}
            aria-label="Apply to existing live recordings"
          />
          <div className="grid gap-1.5 leading-none">
            <Label htmlFor="admin-recording-retention-existing">Apply to existing live recordings</Label>
            <p className="text-xs text-muted-foreground">When off, only recordings completed after this policy is enabled are eligible.</p>
          </div>
        </div>

        {enabled && (
          <div className="rounded-md border bg-muted/30 p-3 text-sm">
            <p className="font-medium">
              {immediateCount} live {immediateCount === 1 ? 'recording has' : 'recordings have'} audio eligible for immediate deletion.
            </p>
            {preview?.oldest_completed_at && (
              <p className="mt-1 text-xs text-muted-foreground">
                Oldest eligible recording completed on {formatDate(preview.oldest_completed_at)}.
              </p>
            )}
          </div>
        )}

        {confirmRequired && (
          <div className="grid gap-2 sm:max-w-xs">
            <Label htmlFor="admin-recording-retention-confirm">Confirm immediate delete count</Label>
            <Input
              id="admin-recording-retention-confirm"
              type="number"
              min={0}
              value={confirmCount}
              onChange={(event) => setConfirmCount(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">Enter {immediateCount} to confirm irreversible audio deletion.</p>
          </div>
        )}

        <Button type="button" onClick={handleSave} disabled={updateMutation.isPending}>
          {updateMutation.isPending ? 'Saving...' : 'Save retention policy'}
        </Button>
      </CardContent>
    </Card>
  )
}
