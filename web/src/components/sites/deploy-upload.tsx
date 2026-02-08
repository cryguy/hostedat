import { useState, useCallback, type DragEvent } from "react"
import { toast } from "sonner"
import { Upload, Loader2, FileArchive } from "lucide-react"
import { deployments } from "@/lib/api"
import { Button } from "@/components/ui/button"

interface DeployUploadProps {
  siteId: string
  onDeployed: () => void
}

export function DeployUpload({ siteId, onDeployed }: DeployUploadProps) {
  const [loading, setLoading] = useState(false)
  const [dragOver, setDragOver] = useState(false)

  const handleFile = useCallback(async (file: File) => {
    if (!file.name.endsWith(".zip")) {
      toast.error("Please upload a .zip file")
      return
    }
    setLoading(true)
    try {
      const dep = await deployments.deploy(siteId, file)
      toast.success(`Deployed v${dep.version}`)
      onDeployed()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Deploy failed")
    } finally {
      setLoading(false)
    }
  }, [siteId, onDeployed])

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }

  function handleFileInput(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) handleFile(file)
    e.target.value = ""
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border p-8">
        <Loader2 className="size-8 animate-spin text-muted-foreground" />
        <p className="mt-3 text-sm text-muted-foreground">Uploading & deploying...</p>
      </div>
    )
  }

  return (
    <div
      onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
      onDragLeave={() => setDragOver(false)}
      onDrop={handleDrop}
      className={`flex flex-col items-center justify-center rounded-lg border border-dashed p-8 transition-colors ${
        dragOver ? "border-emerald-500 bg-emerald-500/5" : "border-border"
      }`}
    >
      <FileArchive className="size-8 text-muted-foreground" />
      <p className="mt-3 text-sm text-muted-foreground">
        Drag & drop a <span className="font-mono">.zip</span> file, or
      </p>
      <Button variant="outline" size="sm" className="mt-3 relative">
        <Upload className="size-4" />
        Browse files
        <input
          type="file"
          accept=".zip"
          onChange={handleFileInput}
          className="absolute inset-0 opacity-0 cursor-pointer"
        />
      </Button>
    </div>
  )
}
