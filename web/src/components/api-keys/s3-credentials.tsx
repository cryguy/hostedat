import { toast } from "sonner"
import { Trash2 } from "lucide-react"
import { useState } from "react"
import { s3Credentials } from "@/lib/api"
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
import type { S3Credential } from "@/types/api"

interface S3CredentialListProps {
  items: S3Credential[]
  onDeleted: () => void
}

export function S3CredentialList({ items, onDeleted }: S3CredentialListProps) {
  const [deleteId, setDeleteId] = useState<string | null>(null)

  async function handleDelete() {
    if (!deleteId) return
    try {
      await s3Credentials.delete(deleteId)
      toast.success("S3 credential deleted")
      setDeleteId(null)
      onDeleted()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete")
    }
  }

  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-8 text-center">
        No S3 credentials yet.
      </p>
    )
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Access Key ID</TableHead>
            <TableHead>Created</TableHead>
            <TableHead>Last used</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((cred) => (
            <TableRow key={cred.id}>
              <TableCell className="font-medium">{cred.name}</TableCell>
              <TableCell className="font-mono text-sm text-muted-foreground">{cred.access_key_id}</TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {new Date(cred.created_at).toLocaleDateString()}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {cred.last_used_at
                  ? new Date(cred.last_used_at).toLocaleDateString()
                  : "Never"}
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => setDeleteId(cred.id)}
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
        title="Delete S3 credential"
        description="This credential will be permanently revoked. Any integrations using it will stop working."
        confirmLabel="Delete"
        onConfirm={handleDelete}
        destructive
      />
    </>
  )
}
