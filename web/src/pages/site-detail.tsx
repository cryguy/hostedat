import { useEffect, useState, useCallback } from "react"
import { useParams, Link } from "react-router-dom"
import { toast } from "sonner"
import { ArrowLeft, ExternalLink, Loader2 } from "lucide-react"
import { sites as sitesApi, deployments as deploymentsApi } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { DeployUpload } from "@/components/sites/deploy-upload"
import { DeploymentList } from "@/components/sites/deployment-list"
import { SiteSettings } from "@/components/sites/site-settings"
import { WorkerPanel } from "@/components/sites/worker-panel"
import { getInstanceDomain } from "@/lib/config"
import type { Site, Deployment } from "@/types/api"

const DOMAIN = getInstanceDomain()

export default function SiteDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [site, setSite] = useState<Site | null>(null)
  const [deps, setDeps] = useState<Deployment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!id) return
    setError(null)
    try {
      const [s, d] = await Promise.all([
        sitesApi.get(id),
        deploymentsApi.list(id),
      ])
      setSite(s)
      setDeps(d)
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load site"
      setError(msg)
      toast.error(msg)
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => { load() }, [load])

  if (loading) {
    return (
      <div className="flex justify-center py-24">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !site) {
    return (
      <div className="flex flex-col items-center justify-center py-24 gap-4">
        <p className="text-sm text-muted-foreground">{error || "Site not found"}</p>
        <Link to="/" className="text-sm text-emerald-400 hover:underline">Back to sites</Link>
      </div>
    )
  }

  const siteUrl = `https://${site.subdomain_slug}.${DOMAIN}`

  return (
    <div>
      <div className="mb-6">
        <Link
          to="/"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors mb-4"
        >
          <ArrowLeft className="size-3.5" />
          Sites
        </Link>

        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold tracking-tight">{site.name}</h1>
            <div className="flex items-center gap-2 mt-1">
              <a
                href={siteUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm text-muted-foreground font-mono hover:text-emerald-400 transition-colors inline-flex items-center gap-1"
              >
                {site.subdomain_slug}.{DOMAIN}
                <ExternalLink className="size-3" />
              </a>
              {site.spa_mode && (
                <Badge variant="outline" className="text-xs">SPA</Badge>
              )}
            </div>
          </div>
          {site.active_version !== null && (
            <Badge className="bg-emerald-600 text-white">v{site.active_version}</Badge>
          )}
        </div>
      </div>

      <Tabs defaultValue="deploy" className="space-y-4">
        <TabsList>
          <TabsTrigger value="deploy">Deploy</TabsTrigger>
          <TabsTrigger value="deployments">Deployments</TabsTrigger>
          <TabsTrigger value="worker">Worker</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="deploy">
          <DeployUpload siteId={site.id} onDeployed={load} />
        </TabsContent>

        <TabsContent value="deployments">
          <DeploymentList siteId={site.id} items={deps} activeVersion={site.active_version} onRollback={load} />
        </TabsContent>

        <TabsContent value="worker">
          <WorkerPanel siteId={site.id} hasWorker={site.has_worker} />
        </TabsContent>

        <TabsContent value="settings">
          <SiteSettings site={site} onUpdated={load} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
