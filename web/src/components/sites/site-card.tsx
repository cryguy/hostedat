import { Link } from "react-router-dom"
import { ExternalLink } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import type { Site } from "@/types/api"

interface SiteCardProps {
  site: Site
  domain: string
}

export function SiteCard({ site, domain }: SiteCardProps) {
  const hasDeployment = site.active_version !== null

  return (
    <Link
      to={`/sites/${site.id}`}
      className="group block rounded-lg border border-border bg-card p-4 transition-colors hover:border-muted-foreground/30"
    >
      <div className="flex items-start justify-between">
        <div className="min-w-0">
          <h3 className="font-medium truncate">{site.name}</h3>
          <div className="flex items-center gap-1.5 mt-1">
            <span className="text-xs text-muted-foreground font-mono truncate">
              {site.subdomain_slug}.{domain}
            </span>
            {hasDeployment && (
              <ExternalLink className="size-3 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
            )}
          </div>
        </div>
        <Badge variant={hasDeployment ? "default" : "secondary"} className="ml-2 shrink-0">
          {hasDeployment ? `v${site.active_version}` : "no deploys"}
        </Badge>
      </div>
      <p className="text-xs text-muted-foreground mt-3">
        {new Date(site.created_at).toLocaleDateString()}
      </p>
    </Link>
  )
}
