import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { Deployment } from "@/types/api"

interface DeploymentListProps {
  items: Deployment[]
  activeVersion: number | null
}

export function DeploymentList({ items, activeVersion }: DeploymentListProps) {
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
          <TableHead className="w-20" />
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
            <TableCell>
              {dep.version === activeVersion && (
                <Badge variant="default" className="bg-emerald-600 text-white">
                  active
                </Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
