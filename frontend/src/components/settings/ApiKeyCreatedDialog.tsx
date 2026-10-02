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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { CreateAPIKeyResponse } from '@/types/apikey'

interface ApiKeyCreatedDialogProps {
  open: boolean
  onClose: () => void
  response: CreateAPIKeyResponse
}

function CopyButton({ text, label }: { text: string; label?: string }) {
  const { t } = useTranslation('settings')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState(false)

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setError(false)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      setError(true)
      setTimeout(() => setError(false), 3000)
    }
  }

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={handleCopy}
      aria-label={label ?? t('apiKeys.created.copy')}
    >
      {error ? t('apiKeys.created.failed') : copied ? t('apiKeys.created.copied') : t('apiKeys.created.copy')}
    </Button>
  )
}

function CodeBlock({ children, copyText }: { children: string; copyText?: string }) {
  const { t } = useTranslation('settings')
  return (
    <div className="relative">
      <pre className="overflow-x-auto rounded-md bg-muted p-3 text-xs font-mono whitespace-pre-wrap break-all">
        {children}
      </pre>
      <div className="absolute right-2 top-2">
        <CopyButton text={copyText ?? children} label={t('apiKeys.created.copySnippet')} />
      </div>
    </div>
  )
}

function getMcpConfig(response: CreateAPIKeyResponse): string {
  return JSON.stringify(
    {
      mcpServers: {
        voxis: {
          url: response.mcp_url,
          headers: {
            Authorization: `Bearer ${response.key}`,
          },
        },
      },
    },
    null,
    2
  )
}

function getCurlExample(response: CreateAPIKeyResponse): string {
  return `curl ${response.api_url}/media \\
  -H "Authorization: Bearer ${response.key}"`
}

export function ApiKeyCreatedDialog({
  open,
  onClose,
  response,
}: ApiKeyCreatedDialogProps) {
  const { t } = useTranslation('settings')
  return (
    <Dialog
      open={open}
      onOpenChange={(isOpen) => {
        if (!isOpen) onClose()
      }}
    >
      <DialogContent
        className="max-w-lg"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('apiKeys.created.title')}</DialogTitle>
          <DialogDescription>
            {t('apiKeys.created.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="rounded-md border border-warning/40 bg-warning/10 p-3">
            <p className="text-sm font-medium text-warning">
              {t('apiKeys.created.warning')}
            </p>
          </div>

          <div className="flex items-center gap-2 rounded-md bg-muted p-3">
            <code className="flex-1 break-all text-sm font-mono">{response.key}</code>
            <CopyButton text={response.key} label={t('apiKeys.created.copyApiKey')} />
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium">{t('apiKeys.created.snippets')}</p>
            <Tabs defaultValue="claude">
              <TabsList className="flex-wrap h-auto">
                <TabsTrigger value="claude">{t('apiKeys.created.tabs.claude')}</TabsTrigger>
                <TabsTrigger value="codex">{t('apiKeys.created.tabs.codex')}</TabsTrigger>
                <TabsTrigger value="adk">{t('apiKeys.created.tabs.adk')}</TabsTrigger>
                <TabsTrigger value="cursor">{t('apiKeys.created.tabs.cursor')}</TabsTrigger>
                <TabsTrigger value="rest">{t('apiKeys.created.tabs.rest')}</TabsTrigger>
                <TabsTrigger value="openapi">{t('apiKeys.created.tabs.openapi')}</TabsTrigger>
              </TabsList>

              <TabsContent value="claude">
                <p className="mb-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.claude')}
                </p>
                <CodeBlock>{getMcpConfig(response)}</CodeBlock>
              </TabsContent>

              <TabsContent value="codex">
                <p className="mb-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.codex')}
                </p>
                <CodeBlock>{getMcpConfig(response)}</CodeBlock>
              </TabsContent>

              <TabsContent value="adk">
                <p className="mb-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.adk')}
                </p>
                <CodeBlock>{getMcpConfig(response)}</CodeBlock>
              </TabsContent>

              <TabsContent value="cursor">
                <p className="mb-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.cursor')}
                </p>
                <CodeBlock>{getMcpConfig(response)}</CodeBlock>
              </TabsContent>

              <TabsContent value="rest">
                <p className="mb-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.rest')}
                </p>
                <CodeBlock>{getCurlExample(response)}</CodeBlock>
              </TabsContent>

              <TabsContent value="openapi">
                <p className="mb-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.openapi')}
                </p>
                <CodeBlock>{`${response.api_url}/openapi.json`}</CodeBlock>
                <p className="mt-2 text-xs text-muted-foreground">
                  {t('apiKeys.created.instructions.openapiNote')}
                </p>
              </TabsContent>
            </Tabs>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={onClose}>{t('apiKeys.created.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
