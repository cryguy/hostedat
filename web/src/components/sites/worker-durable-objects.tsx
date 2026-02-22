import { useEffect, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Trash2 } from "lucide-react"
import { workers } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { DurableObjectNamespace } from "@/types/api"

interface WorkerDurableObjectsProps {
  siteId: string
}

export function WorkerDurableObjects({ siteId }: WorkerDurableObjectsProps) {
  const [namespaces, setNamespaces] = useState<DurableObjectNamespace[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState("")
  const [creating, setCreating] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const load = async () => {
    try {
      const data = await workers.listDurableObjects(siteId)
      setNamespaces(data.items)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load Durable Object namespaces")
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
      await workers.createDurableObject(siteId, name.trim())
      toast.success("Durable Object namespace created")
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
      await workers.deleteDurableObject(siteId, id)
      toast.success("Durable Object namespace deleted")
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
                <TableHead>Namespace ID</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {namespaces.map((ns) => (
                <TableRow key={ns.id}>
                  <TableCell className="font-mono text-sm">{ns.name}</TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">{ns.namespace_id}</TableCell>
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
          <Label htmlFor="do-name">Namespace Name</Label>
          <Input
            id="do-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="MY_DURABLE_OBJECT"
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
          title="Delete Durable Object namespace?"
          description="This will delete the namespace and all stored entries. This action cannot be undone."
          onConfirm={() => handleDelete(deleteId)}
          destructive
        />
      )}
    </div>
  )
}
