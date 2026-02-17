import { useEffect, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Trash2 } from "lucide-react"
import { storage } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { StorageBucket } from "@/types/api"

interface WorkerStorageProps {
  siteId: string
}

export function WorkerStorage({ siteId }: WorkerStorageProps) {
  const [buckets, setBuckets] = useState<StorageBucket[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState("")
  const [bucketName, setBucketName] = useState("")
  const [creating, setCreating] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const load = async () => {
    try {
      const data = await storage.listBuckets(siteId)
      setBuckets(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load storage buckets")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [siteId])

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !bucketName.trim()) return

    setCreating(true)
    try {
      await storage.createBucket(siteId, { name: name.trim(), bucket_name: bucketName.trim() })
      toast.success("Storage bucket created")
      setName("")
      setBucketName("")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create bucket")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await storage.deleteBucket(siteId, id)
      toast.success("Storage bucket deleted")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete bucket")
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
      {buckets.length > 0 && (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Binding Name</TableHead>
                <TableHead>Bucket Name</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {buckets.map((bucket) => (
                <TableRow key={bucket.id}>
                  <TableCell className="font-mono text-sm">{bucket.name}</TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">{bucket.bucket_name}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {new Date(bucket.created_at).toLocaleString()}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setDeleteId(bucket.id)}
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
          <Label htmlFor="binding-name">Binding Name</Label>
          <Input
            id="binding-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="IMAGES"
            className="font-mono"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="bucket-name">Bucket Name</Label>
          <Input
            id="bucket-name"
            value={bucketName}
            onChange={(e) => setBucketName(e.target.value)}
            placeholder="my-images"
            className="font-mono"
          />
        </div>
        <Button type="submit" disabled={creating || !name.trim() || !bucketName.trim()}>
          {creating && <Loader2 className="size-4 animate-spin mr-2" />}
          Create Bucket
        </Button>
      </form>

      {deleteId && (
        <ConfirmDialog
          open={!!deleteId}
          onOpenChange={(open) => !open && setDeleteId(null)}
          title="Delete storage bucket?"
          description="This will delete the bucket binding and all stored data. This action cannot be undone."
          onConfirm={() => handleDelete(deleteId)}
          destructive
        />
      )}
    </div>
  )
}
