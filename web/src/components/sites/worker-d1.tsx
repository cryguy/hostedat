import { useEffect, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Trash2 } from "lucide-react"
import { workers } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { D1Database } from "@/types/api"

interface WorkerD1Props {
  siteId: string
}

export function WorkerD1({ siteId }: WorkerD1Props) {
  const [databases, setDatabases] = useState<D1Database[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState("")
  const [creating, setCreating] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const load = async () => {
    try {
      const data = await workers.listD1(siteId)
      setDatabases(data.items)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load D1 databases")
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
      await workers.createD1(siteId, name.trim())
      toast.success("D1 database created")
      setName("")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create database")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await workers.deleteD1(siteId, id)
      toast.success("D1 database deleted")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete database")
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
      {databases.length > 0 && (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Database ID</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {databases.map((db) => (
                <TableRow key={db.id}>
                  <TableCell className="font-mono text-sm">{db.name}</TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">{db.database_id}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {new Date(db.created_at).toLocaleString()}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setDeleteId(db.id)}
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
          <Label htmlFor="d1-name">Database Name</Label>
          <Input
            id="d1-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="MY_DB"
            className="font-mono"
          />
        </div>
        <Button type="submit" disabled={creating || !name.trim()}>
          {creating && <Loader2 className="size-4 animate-spin mr-2" />}
          Create Database
        </Button>
      </form>

      {deleteId && (
        <ConfirmDialog
          open={!!deleteId}
          onOpenChange={(open) => !open && setDeleteId(null)}
          title="Delete D1 database?"
          description="This will delete the database and all stored data. This action cannot be undone."
          onConfirm={() => handleDelete(deleteId)}
          destructive
        />
      )}
    </div>
  )
}
