import { useEffect, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Trash2, Eye, EyeOff } from "lucide-react"
import { workers } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { WorkerEnvVar } from "@/types/api"

interface WorkerEnvVarsProps {
  siteId: string
}

export function WorkerEnvVars({ siteId }: WorkerEnvVarsProps) {
  const [vars, setVars] = useState<WorkerEnvVar[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState("")
  const [value, setValue] = useState("")
  const [secret, setSecret] = useState(false)
  const [adding, setAdding] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [revealed, setRevealed] = useState<Set<string>>(new Set())

  const load = async () => {
    try {
      const data = await workers.listEnv(siteId)
      setVars(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load environment variables")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [siteId])

  const handleAdd = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !value.trim()) return

    setAdding(true)
    try {
      await workers.setEnv(siteId, { name: name.trim(), value: value.trim(), secret })
      toast.success("Environment variable added")
      setName("")
      setValue("")
      setSecret(false)
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add variable")
    } finally {
      setAdding(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await workers.deleteEnv(siteId, id)
      toast.success("Environment variable deleted")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete variable")
    } finally {
      setDeleteId(null)
    }
  }

  const toggleReveal = (id: string) => {
    setRevealed(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
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
      {vars.length > 0 && (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Value</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {vars.map((v) => (
                <TableRow key={v.id}>
                  <TableCell className="font-mono text-sm">{v.name}</TableCell>
                  <TableCell className="font-mono text-sm">
                    <div className="flex items-center gap-2">
                      {v.secret && !revealed.has(v.id) ? (
                        <span className="text-muted-foreground">••••••••</span>
                      ) : (
                        <span>{v.value}</span>
                      )}
                      {v.secret && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => toggleReveal(v.id)}
                          className="h-6 w-6 p-0"
                        >
                          {revealed.has(v.id) ? (
                            <EyeOff className="size-3.5" />
                          ) : (
                            <Eye className="size-3.5" />
                          )}
                        </Button>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setDeleteId(v.id)}
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

      <form onSubmit={handleAdd} className="space-y-4 border rounded-lg p-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="var-name">Name</Label>
            <Input
              id="var-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="API_KEY"
              className="font-mono"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="var-value">Value</Label>
            <Input
              id="var-value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="sk_..."
              className="font-mono"
            />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="var-secret"
            checked={secret}
            onCheckedChange={setSecret}
          />
          <Label htmlFor="var-secret" className="cursor-pointer">
            Mark as secret
          </Label>
        </div>
        <Button type="submit" disabled={adding || !name.trim() || !value.trim()}>
          {adding && <Loader2 className="size-4 animate-spin mr-2" />}
          Add Variable
        </Button>
      </form>

      {deleteId && (
        <ConfirmDialog
          open={!!deleteId}
          onOpenChange={(open) => !open && setDeleteId(null)}
          title="Delete environment variable?"
          description="This action cannot be undone."
          onConfirm={() => handleDelete(deleteId)}
          destructive
        />
      )}
    </div>
  )
}
