import { useEffect, useState, useCallback } from "react"
import { toast } from "sonner"
import { Plus, Globe, Loader2 } from "lucide-react"
import { sites as sitesApi } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/shared/page-header"
import { EmptyState } from "@/components/shared/empty-state"
import { SiteCard } from "@/components/sites/site-card"
import { CreateSiteDialog } from "@/components/sites/create-site-dialog"
import type { Site } from "@/types/api"

const DOMAIN = "hostedat.ditto.moe"

export default function DashboardPage() {
  const [sites, setSites] = useState<Site[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)

  const load = useCallback(async () => {
    try {
      const data = await sitesApi.list()
      setSites(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load sites")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  if (loading) {
    return (
      <div className="flex justify-center py-24">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <>
      <PageHeader
        title="Sites"
        description={`${sites.length} site${sites.length !== 1 ? "s" : ""}`}
        action={
          <Button onClick={() => setCreateOpen(true)} className="bg-emerald-600 text-white hover:bg-emerald-500">
            <Plus className="size-4" />
            New site
          </Button>
        }
      />

      {sites.length === 0 ? (
        <EmptyState
          icon={<Globe className="size-10" />}
          title="No sites yet"
          description="Create your first site to get started with static hosting."
          action={
            <Button onClick={() => setCreateOpen(true)} className="bg-emerald-600 text-white hover:bg-emerald-500">
              <Plus className="size-4" />
              New site
            </Button>
          }
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {sites.map((site) => (
            <SiteCard key={site.id} site={site} domain={DOMAIN} />
          ))}
        </div>
      )}

      <CreateSiteDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={load}
      />
    </>
  )
}
