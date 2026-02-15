import { useEffect, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Trash2 } from "lucide-react"
import { workers } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { CronSchedule } from "@/types/api"

interface WorkerCronsProps {
  siteId: string
}

export function WorkerCrons({ siteId }: WorkerCronsProps) {
  const [crons, setCrons] = useState<CronSchedule[]>([])
  const [loading, setLoading] = useState(true)
  const [expression, setExpression] = useState("")
  const [enabled, setEnabled] = useState(true)
  const [creating, setCreating] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const load = async () => {
    try {
      const data = await workers.listCrons(siteId)
      setCrons(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load cron schedules")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [siteId])

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!expression.trim()) return

    setCreating(true)
    try {
      await workers.createCron(siteId, { cron: expression.trim(), enabled })
      toast.success("Cron schedule created")
      setExpression("")
      setEnabled(true)
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create cron schedule")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await workers.deleteCron(siteId, id)
      toast.success("Cron schedule deleted")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete cron schedule")
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
      {crons.length > 0 && (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Expression</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Last Run</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {crons.map((cron) => (
                <TableRow key={cron.id}>
                  <TableCell className="font-mono text-sm">{cron.cron}</TableCell>
                  <TableCell>
                    <Badge variant={cron.enabled ? "default" : "secondary"}>
                      {cron.enabled ? "Enabled" : "Disabled"}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {cron.last_run_at
                      ? new Date(cron.last_run_at).toLocaleString()
                      : "Never"}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setDeleteId(cron.id)}
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
          <Label htmlFor="cron-expr">Cron Expression</Label>
          <Input
            id="cron-expr"
            value={expression}
            onChange={(e) => setExpression(e.target.value)}
            placeholder="0 * * * *"
            className="font-mono"
          />
          <p className="text-xs text-muted-foreground">
            Format: minute hour day month weekday (e.g., "0 * * * *" = every hour)
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="cron-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
          />
          <Label htmlFor="cron-enabled" className="cursor-pointer">
            Enabled
          </Label>
        </div>
        <Button type="submit" disabled={creating || !expression.trim()}>
          {creating && <Loader2 className="size-4 animate-spin mr-2" />}
          Create Schedule
        </Button>
      </form>

      {deleteId && (
        <ConfirmDialog
          open={!!deleteId}
          onOpenChange={(open) => !open && setDeleteId(null)}
          title="Delete cron schedule?"
          description="This action cannot be undone."
          onConfirm={() => handleDelete(deleteId)}
          destructive
        />
      )}
    </div>
  )
}
