import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"
import { sites } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Separator } from "@/components/ui/separator"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { Site } from "@/types/api"

interface SiteSettingsProps {
  site: Site
  onUpdated: () => void
}

export function SiteSettings({ site, onUpdated }: SiteSettingsProps) {
  const navigate = useNavigate()
  const [name, setName] = useState(site.name)
  const [spaMode, setSpaMode] = useState(site.spa_mode)
  const [saving, setSaving] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      await sites.update(site.id, { name, spa_mode: spaMode })
      toast.success("Settings saved")
      onUpdated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save")
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    setDeleting(true)
    try {
      await sites.delete(site.id)
      toast.success("Site deleted")
      navigate("/")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete")
      setDeleting(false)
    }
  }

  return (
    <div className="space-y-6">
      <form onSubmit={handleSave} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="site-name">Site name</Label>
          <Input
            id="site-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>

        <div className="flex items-center justify-between">
          <div>
            <Label htmlFor="spa-mode">SPA mode</Label>
            <p className="text-xs text-muted-foreground mt-0.5">
              Serve index.html for all routes (single-page app)
            </p>
          </div>
          <Switch
            id="spa-mode"
            checked={spaMode}
            onCheckedChange={setSpaMode}
          />
        </div>

        <Button type="submit" disabled={saving}>
          {saving ? <Loader2 className="size-4 animate-spin" /> : "Save changes"}
        </Button>
      </form>

      <Separator />

      <div>
        <h3 className="text-sm font-medium text-destructive">Danger zone</h3>
        <p className="text-xs text-muted-foreground mt-1">
          Permanently delete this site and all its deployments.
        </p>
        <Button
          variant="destructive"
          size="sm"
          className="mt-3"
          onClick={() => setDeleteOpen(true)}
          disabled={deleting}
        >
          {deleting ? <Loader2 className="size-4 animate-spin" /> : "Delete site"}
        </Button>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete site"
        description={`This will permanently delete "${site.name}" and all its deployments. This action cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        destructive
      />
    </div>
  )
}
