import { useState } from "react"
import { toast } from "sonner"
import { Copy, Check, XCircle } from "lucide-react"
import { admin } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { Invite } from "@/types/api"

interface InviteListProps {
  items: Invite[]
  domain: string
  onRevoked: () => void
}

export function InviteList({ items, domain, onRevoked }: InviteListProps) {
  const [copiedId, setCopiedId] = useState<string | null>(null)

  function copyLink(invite: Invite) {
    const url = `https://${domain}/register?invite=${invite.code}`
    navigator.clipboard.writeText(url)
    setCopiedId(invite.id)
    setTimeout(() => setCopiedId(null), 2000)
  }

  async function handleRevoke(id: string) {
    try {
      await admin.revokeInvite(id)
      toast.success("Invite revoked")
      onRevoked()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to revoke")
    }
  }

  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-8 text-center">
        No invites yet.
      </p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Code</TableHead>
          <TableHead>Uses</TableHead>
          <TableHead>Expires</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="w-24" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((inv) => {
          const expired = inv.expires_at && new Date(inv.expires_at) < new Date()
          const maxed = inv.max_uses !== undefined && inv.max_uses !== null && inv.use_count >= inv.max_uses
          const isActive = inv.active && !expired && !maxed

          return (
            <TableRow key={inv.id}>
              <TableCell className="font-mono text-xs">{inv.code.substring(0, 16)}...</TableCell>
              <TableCell className="text-sm">
                {inv.use_count}{inv.max_uses !== undefined && inv.max_uses !== null ? ` / ${inv.max_uses}` : ""}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {inv.expires_at ? new Date(inv.expires_at).toLocaleDateString() : "Never"}
              </TableCell>
              <TableCell>
                <Badge variant={isActive ? "default" : "secondary"}>
                  {isActive ? "active" : expired ? "expired" : maxed ? "exhausted" : "revoked"}
                </Badge>
              </TableCell>
              <TableCell>
                <div className="flex gap-1">
                  {isActive && (
                    <>
                      <Button variant="ghost" size="icon-xs" onClick={() => copyLink(inv)} title="Copy invite link">
                        {copiedId === inv.id ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                      </Button>
                      <Button variant="ghost" size="icon-xs" onClick={() => handleRevoke(inv.id)} title="Revoke">
                        <XCircle className="size-3.5 text-muted-foreground" />
                      </Button>
                    </>
                  )}
                </div>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
