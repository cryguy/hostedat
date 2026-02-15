import { useEffect, useState } from "react"
import { toast } from "sonner"
import { Loader2, RefreshCw } from "lucide-react"
import { workers } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import type { WorkerLog } from "@/types/api"

interface WorkerLogsProps {
  siteId: string
}

export function WorkerLogs({ siteId }: WorkerLogsProps) {
  const [logs, setLogs] = useState<WorkerLog[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  const load = async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const data = await workers.getLogs(siteId)
      setLogs(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load worker logs")
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    load()
  }, [siteId])

  const handleRefresh = () => {
    load(true)
  }

  const getLevelVariant = (level: string) => {
    switch (level) {
      case "error":
        return "destructive"
      case "warn":
        return "outline"
      default:
        return "secondary"
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
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <p className="text-sm text-muted-foreground">
          Showing last {logs.length} log entries
        </p>
        <Button
          variant="outline"
          size="sm"
          onClick={handleRefresh}
          disabled={refreshing}
        >
          <RefreshCw className={`size-4 mr-2 ${refreshing ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {logs.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <p>No logs yet</p>
          <p className="text-xs mt-1">Logs will appear here when your worker runs</p>
        </div>
      ) : (
        <div className="space-y-2 border rounded-lg p-4 bg-muted/30 font-mono text-xs max-h-[500px] overflow-y-auto">
          {logs.map((log) => (
            <div key={log.id} className="flex gap-3 py-1">
              <span className="text-muted-foreground shrink-0">
                {new Date(log.created_at).toLocaleTimeString()}
              </span>
              <Badge
                variant={getLevelVariant(log.level)}
                className={`shrink-0 h-5 ${log.level === "warn" ? "bg-yellow-100 text-yellow-800 border-yellow-300" : ""}`}
              >
                {log.level}
              </Badge>
              <span className="break-words flex-1">{log.message}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
