import { useState } from "react"
import { toast } from "sonner"
import { RotateCcw, Loader2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { deployments as deploymentsApi } from "@/lib/api"
import type { Deployment } from "@/types/api"

interface DeploymentListProps {
  siteId: string
  items: Deployment[]
  activeVersion: number | null
  onRollback: () => void
}

export function DeploymentList({ siteId, items, activeVersion, onRollback }: DeploymentListProps) {
  const [rollingBack, setRollingBack] = useState<number | null>(null)

  async function handleRollback(version: number) {
    setRollingBack(version)
    try {
      await deploymentsApi.rollback(siteId, version)
      toast.success(`Rolled back to v${version}`)
      onRollback()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Rollback failed")
    } finally {
      setRollingBack(null)
    }
  }

  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-8 text-center">
        No deployments yet. Upload a zip to deploy.
      </p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Version</TableHead>
          <TableHead>Hash</TableHead>
          <TableHead>Deployed</TableHead>
          <TableHead className="w-32" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((dep) => (
          <TableRow key={dep.id}>
            <TableCell className="font-mono">v{dep.version}</TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">
              {dep.file_hash.substring(0, 12)}
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">
              {new Date(dep.uploaded_at).toLocaleString()}
            </TableCell>
            <TableCell className="text-right">
              {dep.version === activeVersion ? (
                <Badge variant="default" className="bg-emerald-600 text-white">
                  active
                </Badge>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 text-xs"
                  disabled={rollingBack !== null}
                  onClick={() => handleRollback(dep.version)}
                >
                  {rollingBack === dep.version ? (
                    <Loader2 className="size-3 animate-spin mr-1" />
                  ) : (
                    <RotateCcw className="size-3 mr-1" />
                  )}
                  Rollback
                </Button>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
