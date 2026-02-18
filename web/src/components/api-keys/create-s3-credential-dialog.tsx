import { useState, type FormEvent } from "react"
import { toast } from "sonner"
import { Loader2, Copy, Check } from "lucide-react"
import { s3Credentials } from "@/lib/api"
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

interface CreateS3CredentialDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

export function CreateS3CredentialDialog({ open, onOpenChange, onCreated }: CreateS3CredentialDialogProps) {
  const [name, setName] = useState("")
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<{ access_key_id: string; secret_access_key: string } | null>(null)
  const [copiedField, setCopiedField] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    try {
      const res = await s3Credentials.create(name)
      setResult({ access_key_id: res.access_key_id, secret_access_key: res.secret_access_key })
      onCreated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create credential")
    } finally {
      setLoading(false)
    }
  }

  function handleCopy(value: string, field: string) {
    navigator.clipboard.writeText(value)
    setCopiedField(field)
    setTimeout(() => setCopiedField(null), 2000)
  }

  function handleClose(nextOpen: boolean) {
    if (!nextOpen) {
      setName("")
      setResult(null)
      setCopiedField(null)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{result ? "S3 credential created" : "New S3 credential"}</DialogTitle>
          {result && (
            <DialogDescription>
              Copy the secret access key now. You won't be able to see it again.
            </DialogDescription>
          )}
        </DialogHeader>

        {result ? (
          <div className="space-y-3">
            <div className="space-y-1">
              <Label className="text-xs text-muted-foreground">Access Key ID</Label>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded-md border border-border bg-muted px-3 py-2 font-mono text-xs break-all">
                  {result.access_key_id}
                </code>
                <Button variant="outline" size="icon" onClick={() => handleCopy(result.access_key_id, "access")}>
                  {copiedField === "access" ? <Check className="size-4 text-emerald-500" /> : <Copy className="size-4" />}
                </Button>
              </div>
            </div>
            <div className="space-y-1">
              <Label className="text-xs text-muted-foreground">Secret Access Key</Label>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded-md border border-border bg-muted px-3 py-2 font-mono text-xs break-all">
                  {result.secret_access_key}
                </code>
                <Button variant="outline" size="icon" onClick={() => handleCopy(result.secret_access_key, "secret")}>
                  {copiedField === "secret" ? <Check className="size-4 text-emerald-500" /> : <Copy className="size-4" />}
                </Button>
              </div>
            </div>
            <Button className="w-full" onClick={() => handleClose(false)}>
              Done
            </Button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="cred-name">Name</Label>
              <Input
                id="cred-name"
                placeholder="my-app"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                autoFocus
              />
              <p className="text-xs text-muted-foreground">
                Letters, digits, underscore, or hyphen (1-32 characters)
              </p>
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? <Loader2 className="size-4 animate-spin" /> : "Create credential"}
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
