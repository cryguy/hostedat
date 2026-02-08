import { useEffect, useState } from "react"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"
import { admin } from "@/lib/api"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import type { InstanceSettings } from "@/types/api"

export function SettingsPanel() {
  const [settings, setSettings] = useState<InstanceSettings | null>(null)
  const [loading, setLoading] = useState(true)

  async function load() {
    try {
      const data = await admin.getSettings()
      setSettings(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load settings")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  async function toggle(key: keyof InstanceSettings, value: boolean) {
    try {
      const updated = await admin.updateSettings({ [key]: value })
      setSettings(updated)
      toast.success("Settings updated")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update")
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!settings) return null

  return (
    <div className="space-y-6 max-w-lg">
      <div className="flex items-center justify-between">
        <div>
          <Label htmlFor="reg-enabled">Registration</Label>
          <p className="text-xs text-muted-foreground mt-0.5">
            Allow new users to create accounts
          </p>
        </div>
        <Switch
          id="reg-enabled"
          checked={settings.registration_enabled}
          onCheckedChange={(v) => toggle("registration_enabled", v)}
        />
      </div>

      <div className={`flex items-center justify-between pl-4 border-l-2 border-border ${!settings.registration_enabled ? "opacity-50 pointer-events-none" : ""}`}>
        <div>
          <Label htmlFor="invite-req">Require invite</Label>
          <p className="text-xs text-muted-foreground mt-0.5">
            New users must provide an invite code
          </p>
        </div>
        <Switch
          id="invite-req"
          checked={settings.invite_required}
          onCheckedChange={(v) => toggle("invite_required", v)}
          disabled={!settings.registration_enabled}
        />
      </div>
    </div>
  )
}
