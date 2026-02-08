import { useEffect, useState, useCallback } from "react"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"
import { admin } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/shared/page-header"
import { UsersTable } from "@/components/admin/users-table"
import type { User } from "@/types/api"

export default function AdminUsersPage() {
  const [users, setUsers] = useState<User[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      const data = await admin.listUsers(page)
      setUsers(data.users)
      setTotal(data.total)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load users")
    } finally {
      setLoading(false)
    }
  }, [page])

  useEffect(() => { load() }, [load])

  const totalPages = Math.ceil(total / 20)

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
        title="Users"
        description={`${total} user${total !== 1 ? "s" : ""}`}
      />

      <UsersTable users={users} onChanged={load} />

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2 mt-4">
          <Button
            variant="outline"
            size="sm"
            disabled={page === 1}
            onClick={() => setPage((p) => p - 1)}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </>
  )
}
