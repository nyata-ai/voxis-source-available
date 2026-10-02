import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { AVAILABLE_SCOPES, type CreateAPIKeyRequest, type CreateAPIKeyResponse } from '@/types/apikey'
import { getApiErrorMessage } from '@/lib/api-errors'

interface CreateApiKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (req: CreateAPIKeyRequest) => Promise<CreateAPIKeyResponse>
  isPending: boolean
  error: Error | null
}

const EXPIRATION_OPTIONS = [
  { value: '30', labelKey: 'apiKeys.createDialog.expirationOptions.30' },
  { value: '60', labelKey: 'apiKeys.createDialog.expirationOptions.60' },
  { value: '90', labelKey: 'apiKeys.createDialog.expirationOptions.90' },
  { value: 'none', labelKey: 'apiKeys.createDialog.expirationOptions.none' },
]

export function CreateApiKeyDialog({
  open,
  onOpenChange,
  onSubmit,
  isPending,
  error,
}: CreateApiKeyDialogProps) {
  const { t } = useTranslation(['settings', 'common', 'errors'])
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState<string[]>(AVAILABLE_SCOPES.map((s) => s.value))
  const [expiration, setExpiration] = useState('90')

  function handleScopeToggle(scope: string, checked: boolean) {
    setScopes((prev) =>
      checked ? [...prev, scope] : prev.filter((s) => s !== scope)
    )
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return

    const req: CreateAPIKeyRequest = {
      name: name.trim(),
      scopes,
    }

    if (expiration !== 'none') {
      req.expires_in_days = Number(expiration)
    }

    await onSubmit(req)
    setName('')
    setScopes(AVAILABLE_SCOPES.map((s) => s.value))
    setExpiration('90')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('apiKeys.createDialog.title')}</DialogTitle>
          <DialogDescription>
            {t('apiKeys.createDialog.description')}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="key-name">{t('apiKeys.createDialog.keyName')}</Label>
            <Input
              id="key-name"
              placeholder={t('apiKeys.createDialog.keyNamePlaceholder')}
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <Label>{t('apiKeys.createDialog.scopes')}</Label>
            <div className="space-y-2 rounded-md border p-3">
              {AVAILABLE_SCOPES.map((scope) => (
                <div key={scope.value} className="flex items-center space-x-2">
                  <Checkbox
                    id={`scope-${scope.value}`}
                    checked={scopes.includes(scope.value)}
                    onCheckedChange={(checked) =>
                      handleScopeToggle(scope.value, checked === true)
                    }
                  />
                  <Label
                    htmlFor={`scope-${scope.value}`}
                    className="flex-1 cursor-pointer font-normal"
                  >
                    <span className="font-medium">{t(scope.labelKey)}</span>
                    <span className="ml-1 text-muted-foreground">
                      - {t(scope.descriptionKey)}
                    </span>
                  </Label>
                </div>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t('apiKeys.createDialog.expiration')}</Label>
            <RadioGroup value={expiration} onValueChange={setExpiration}>
              {EXPIRATION_OPTIONS.map((opt) => (
                <div key={opt.value} className="flex items-center space-x-2">
                  <RadioGroupItem value={opt.value} id={`exp-${opt.value}`} />
                  <Label htmlFor={`exp-${opt.value}`} className="cursor-pointer font-normal">
                    {t(opt.labelKey)}
                  </Label>
                </div>
              ))}
            </RadioGroup>
          </div>

          {error && (
            <p className="text-sm text-destructive">
              {getApiErrorMessage(error, t, t('apiKeys.createDialog.error'))}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t('common:actions.cancel')}
            </Button>
            <Button type="submit" disabled={isPending || !name.trim()}>
              {isPending ? t('apiKeys.createDialog.creating') : t('apiKeys.createDialog.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
