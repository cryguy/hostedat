import { useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Copy, Check } from "lucide-react"
import { apiKeys } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"

interface CreateKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

export function CreateKeyDialog({ open, onOpenChange, onCreated }: CreateKeyDialogProps) {
  const [name, setName] = useState("")
  const [loading, setLoading] = useState(false)
  const [rawKey, setRawKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    try {
      const res = await apiKeys.create(name)
      setRawKey(res.key)
      onCreated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create key")
    } finally {
      setLoading(false)
    }
  }

  function handleCopy() {
    if (rawKey) {
      navigator.clipboard.writeText(rawKey)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  function handleClose(nextOpen: boolean) {
    if (!nextOpen) {
      setName("")
      setRawKey(null)
      setCopied(false)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{rawKey ? "API key created" : "New API key"}</DialogTitle>
          {rawKey && (
            <DialogDescription>
              Copy this key now. You won't be able to see it again.
            </DialogDescription>
          )}
        </DialogHeader>

        {rawKey ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <code className="flex-1 rounded-md border border-border bg-muted px-3 py-2 font-mono text-xs break-all">
                {rawKey}
              </code>
              <Button variant="outline" size="icon" onClick={handleCopy}>
                {copied ? <Check className="size-4 text-emerald-500" /> : <Copy className="size-4" />}
              </Button>
            </div>
            <Button className="w-full" onClick={() => handleClose(false)}>
              Done
            </Button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="key-name">Name</Label>
              <Input
                id="key-name"
                placeholder="CI/CD pipeline"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                autoFocus
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? <Loader2 className="size-4 animate-spin" /> : "Create key"}
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
