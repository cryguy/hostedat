import { toast } from "sonner"
import { Trash2 } from "lucide-react"
import { useState } from "react"
import { apiKeys } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { APIKey } from "@/types/api"

interface KeyListProps {
  items: APIKey[]
  onDeleted: () => void
}

export function KeyList({ items, onDeleted }: KeyListProps) {
  const [deleteId, setDeleteId] = useState<string | null>(null)

  async function handleDelete() {
    if (!deleteId) return
    try {
      await apiKeys.delete(deleteId)
      toast.success("API key deleted")
      setDeleteId(null)
      onDeleted()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete")
    }
  }

  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-8 text-center">
        No API keys yet.
      </p>
    )
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Created</TableHead>
            <TableHead>Last used</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((key) => (
            <TableRow key={key.id}>
              <TableCell className="font-medium">{key.name}</TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {new Date(key.created_at).toLocaleDateString()}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {key.last_used_at
                  ? new Date(key.last_used_at).toLocaleDateString()
                  : "Never"}
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => setDeleteId(key.id)}
                >
                  <Trash2 className="size-3.5 text-muted-foreground" />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <ConfirmDialog
        open={deleteId !== null}
        onOpenChange={(open) => { if (!open) setDeleteId(null) }}
        title="Delete API key"
        description="This key will be permanently revoked. Any integrations using it will stop working."
        confirmLabel="Delete"
        onConfirm={handleDelete}
        destructive
      />
    </>
  )
}
