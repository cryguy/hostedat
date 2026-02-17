import { Code } from "lucide-react"
import { Separator } from "@/components/ui/separator"
import { WorkerEnvVars } from "./worker-env-vars"
import { WorkerKV } from "./worker-kv"
import { WorkerStorage } from "./worker-storage"
import { WorkerCrons } from "./worker-crons"
import { WorkerLogs } from "./worker-logs"

interface WorkerPanelProps {
  siteId: string
  hasWorker: boolean
}

export function WorkerPanel({ siteId, hasWorker }: WorkerPanelProps) {
  if (!hasWorker) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-4 text-center">
        <div className="rounded-full bg-muted p-4 mb-4">
          <Code className="size-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-semibold mb-2">No worker detected</h3>
        <p className="text-sm text-muted-foreground max-w-md mb-4">
          Include a <code className="bg-muted px-1.5 py-0.5 rounded text-xs">_worker.js</code> file in your deployment to enable server-side JavaScript execution.
        </p>
        <p className="text-xs text-muted-foreground max-w-lg">
          Workers can intercept HTTP requests, access key-value storage, run scheduled tasks, and make outbound fetch requests — similar to Cloudflare Workers.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-8">
      <div>
        <h3 className="text-lg font-semibold mb-2">Environment Variables</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Set environment variables and secrets accessible via the <code className="bg-muted px-1.5 py-0.5 rounded text-xs">env</code> parameter in your worker.
        </p>
        <WorkerEnvVars siteId={siteId} />
      </div>

      <Separator />

      <div>
        <h3 className="text-lg font-semibold mb-2">KV Namespaces</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Persistent key-value storage accessible via <code className="bg-muted px-1.5 py-0.5 rounded text-xs">env.YOUR_NAMESPACE</code> in your worker.
        </p>
        <WorkerKV siteId={siteId} />
      </div>

      <Separator />

      <div>
        <h3 className="text-lg font-semibold mb-2">Storage Buckets</h3>
        <p className="text-sm text-muted-foreground mb-4">
          S3-compatible object storage accessible via <code className="bg-muted px-1.5 py-0.5 rounded text-xs">env.YOUR_BUCKET</code> in your worker.
        </p>
        <WorkerStorage siteId={siteId} />
      </div>

      <Separator />

      <div>
        <h3 className="text-lg font-semibold mb-2">Cron Schedules</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Trigger your worker's <code className="bg-muted px-1.5 py-0.5 rounded text-xs">scheduled()</code> handler on a schedule.
        </p>
        <WorkerCrons siteId={siteId} />
      </div>

      <Separator />

      <div>
        <h3 className="text-lg font-semibold mb-2">Logs</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Console output from your worker (last 100 entries, newest first).
        </p>
        <WorkerLogs siteId={siteId} />
      </div>
    </div>
  )
}
