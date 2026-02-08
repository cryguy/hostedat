import { useEffect, useState, useCallback } from "react"
import { toast } from "sonner"
import { Plus, Loader2, Key } from "lucide-react"
import { apiKeys as keysApi } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/shared/page-header"
import { EmptyState } from "@/components/shared/empty-state"
import { KeyList } from "@/components/api-keys/key-list"
import { CreateKeyDialog } from "@/components/api-keys/create-key-dialog"
import type { APIKey } from "@/types/api"

export default function APIKeysPage() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)

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

  useEffect(() => { load() }, [load])

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

      <CreateKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={load}
      />
    </>
  )
}
