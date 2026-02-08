import { useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"
import { sites } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface CreateSiteDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

export function CreateSiteDialog({ open, onOpenChange, onCreated }: CreateSiteDialogProps) {
  const [name, setName] = useState("")
  const [slug, setSlug] = useState("")
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    try {
      await sites.create(name, slug || undefined)
      toast.success("Site created")
      setName("")
      setSlug("")
      onOpenChange(false)
      onCreated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create site")
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New site</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="site-name">Name</Label>
            <Input
              id="site-name"
              placeholder="My Portfolio"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-slug">Subdomain (optional)</Label>
            <div className="flex items-center gap-2">
              <Input
                id="site-slug"
                placeholder="my-portfolio"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                className="font-mono text-sm"
              />
              <span className="text-sm text-muted-foreground whitespace-nowrap">.hostedat.ditto.moe</span>
            </div>
            <p className="text-xs text-muted-foreground">
              Leave blank to auto-generate from name
            </p>
          </div>
          <Button
            type="submit"
            className="w-full bg-emerald-600 text-white hover:bg-emerald-500"
            disabled={loading}
          >
            {loading ? <Loader2 className="size-4 animate-spin" /> : "Create site"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}
