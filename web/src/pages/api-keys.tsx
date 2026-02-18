import { useEffect, useState, useCallback } from "react"
import { toast } from "sonner"
import { Plus, Loader2, Key } from "lucide-react"
import { apiKeys as keysApi, s3Credentials } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { PageHeader } from "@/components/shared/page-header"
import { EmptyState } from "@/components/shared/empty-state"
import { KeyList } from "@/components/api-keys/key-list"
import { S3CredentialList } from "@/components/api-keys/s3-credentials"
import { CreateKeyDialog } from "@/components/api-keys/create-key-dialog"
import { CreateS3CredentialDialog } from "@/components/api-keys/create-s3-credential-dialog"
import type { APIKey, S3Credential } from "@/types/api"

export default function APIKeysPage() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [credentials, setCredentials] = useState<S3Credential[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [createS3Open, setCreateS3Open] = useState(false)

  const load = useCallback(async () => {
    try {
      const data = await keysApi.list()
      setKeys(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load keys")
    } finally {
      setLoading(false)
    }
  }, [])

  const loadCredentials = useCallback(async () => {
    try {
      const data = await s3Credentials.list()
      setCredentials(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load S3 credentials")
    }
  }, [])

  useEffect(() => { load() }, [load])
  useEffect(() => { loadCredentials() }, [loadCredentials])

  if (loading) {
    return (
      <div className="flex justify-center py-24">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <>
      <PageHeader
        title="API Keys"
        description="Manage keys for CLI and CI/CD integrations"
        action={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            New key
          </Button>
        }
      />

      {keys.length === 0 ? (
        <EmptyState
          icon={<Key className="size-10" />}
          title="No API keys"
          description="Create an API key to use the CLI or automate deployments."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              New key
            </Button>
          }
        />
      ) : (
        <KeyList items={keys} onDeleted={load} />
      )}

      <Separator className="my-8" />

      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-2xl font-semibold">S3 Credentials</h2>
            <p className="text-sm text-muted-foreground">
              Manage S3-compatible storage credentials for object storage access
            </p>
          </div>
          <Button variant="outline" onClick={() => setCreateS3Open(true)}>
            <Plus className="size-4" />
            New credential
          </Button>
        </div>
        <S3CredentialList items={credentials} onDeleted={loadCredentials} />
      </div>

      <CreateKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={load}
      />

      <CreateS3CredentialDialog
        open={createS3Open}
        onOpenChange={setCreateS3Open}
        onCreated={loadCredentials}
      />
    </>
  )
}
