import { useEffect, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Trash2 } from "lucide-react"
import { workers } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { KVNamespace } from "@/types/api"

interface WorkerKVProps {
  siteId: string
}

export function WorkerKV({ siteId }: WorkerKVProps) {
  const [namespaces, setNamespaces] = useState<KVNamespace[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState("")
  const [creating, setCreating] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const load = async () => {
    try {
      const data = await workers.listKV(siteId)
      setNamespaces(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load KV namespaces")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [siteId])

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    setCreating(true)
    try {
      await workers.createKV(siteId, name.trim())
      toast.success("KV namespace created")
      setName("")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create namespace")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await workers.deleteKV(siteId, id)
      toast.success("KV namespace deleted")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete namespace")
    } finally {
      setDeleteId(null)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-8">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {namespaces.length > 0 && (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {namespaces.map((ns) => (
                <TableRow key={ns.id}>
                  <TableCell className="font-mono text-sm">{ns.name}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {new Date(ns.created_at).toLocaleString()}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setDeleteId(ns.id)}
                      className="text-red-600 hover:text-red-700 hover:bg-red-50"
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <form onSubmit={handleCreate} className="space-y-4 border rounded-lg p-4">
        <div className="space-y-2">
          <Label htmlFor="kv-name">Namespace Name</Label>
          <Input
            id="kv-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="MY_KV"
            className="font-mono"
          />
        </div>
        <Button type="submit" disabled={creating || !name.trim()}>
          {creating && <Loader2 className="size-4 animate-spin mr-2" />}
          Create Namespace
        </Button>
      </form>

      {deleteId && (
        <ConfirmDialog
          open={!!deleteId}
          onOpenChange={(open) => !open && setDeleteId(null)}
          title="Delete KV namespace?"
          description="This will delete the namespace and all stored data. This action cannot be undone."
          onConfirm={() => handleDelete(deleteId)}
          destructive
        />
      )}
    </div>
  )
}
