import { useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"
import { admin } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface CreateInviteDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

export function CreateInviteDialog({ open, onOpenChange, onCreated }: CreateInviteDialogProps) {
  const [maxUses, setMaxUses] = useState("")
  const [expiresIn, setExpiresIn] = useState("")
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)

    const data: { max_uses?: number; expires_at?: string } = {}
    if (maxUses) data.max_uses = parseInt(maxUses, 10)
    if (expiresIn) {
      const hours = parseInt(expiresIn, 10)
      const exp = new Date(Date.now() + hours * 3600 * 1000)
      data.expires_at = exp.toISOString()
    }

    try {
      await admin.createInvite(data)
      toast.success("Invite created")
      setMaxUses("")
      setExpiresIn("")
      onOpenChange(false)
      onCreated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create invite")
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New invite</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="max-uses">Max uses (optional)</Label>
            <Input
              id="max-uses"
              type="number"
              min="1"
              placeholder="Unlimited"
              value={maxUses}
              onChange={(e) => setMaxUses(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="expires">Expires in hours (optional)</Label>
            <Input
              id="expires"
              type="number"
              min="1"
              placeholder="Never"
              value={expiresIn}
              onChange={(e) => setExpiresIn(e.target.value)}
            />
          </div>
          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? <Loader2 className="size-4 animate-spin" /> : "Create invite"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}
