import { useEffect, useState, useCallback } from "react"
import { toast } from "sonner"
import { Plus, Loader2, Ticket } from "lucide-react"
import { admin } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/shared/page-header"
import { EmptyState } from "@/components/shared/empty-state"
import { InviteList } from "@/components/admin/invite-list"
import { CreateInviteDialog } from "@/components/admin/create-invite-dialog"
import type { Invite } from "@/types/api"

const DOMAIN = "hostedat.ditto.moe"

export default function AdminInvitesPage() {
  const [invites, setInvites] = useState<Invite[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)

  const load = useCallback(async () => {
    try {
      const data = await admin.listInvites()
      setInvites(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load invites")
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
        title="Invites"
        description="Manage invite codes for new users"
        action={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            New invite
          </Button>
        }
      />

      {invites.length === 0 ? (
        <EmptyState
          icon={<Ticket className="size-10" />}
          title="No invites"
          description="Create invite codes to let new users register."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              New invite
            </Button>
          }
        />
      ) : (
        <InviteList items={invites} domain={DOMAIN} onRevoked={load} />
      )}

      <CreateInviteDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={load}
      />
    </>
  )
}
