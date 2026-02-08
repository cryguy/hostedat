import { useState } from "react"
import { toast } from "sonner"
import { MoreHorizontal } from "lucide-react"
import { admin } from "@/lib/api"
import { useAuth } from "@/hooks/use-auth"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ConfirmDialog } from "@/components/shared/confirm-dialog"
import type { User } from "@/types/api"

const roleBadgeClass: Record<string, string> = {
  superadmin: "bg-purple-600/20 text-purple-400 border-purple-600/30",
  admin: "bg-blue-600/20 text-blue-400 border-blue-600/30",
  user: "",
}

interface UsersTableProps {
  users: User[]
  onChanged: () => void
}

export function UsersTable({ users, onChanged }: UsersTableProps) {
  const { user: currentUser } = useAuth()
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const deleteUser = users.find((u) => u.id === deleteId)

  async function handleRoleChange(userId: string, role: string) {
    try {
      await admin.updateUserRole(userId, role)
      toast.success(`Role updated to ${role}`)
      onChanged()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update role")
    }
  }

  async function handleDelete() {
    if (!deleteId) return
    try {
      await admin.deleteUser(deleteId)
      toast.success("User deleted")
      setDeleteId(null)
      onChanged()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete user")
    }
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Email</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Joined</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((u) => (
            <TableRow key={u.id}>
              <TableCell className="font-medium">{u.email}</TableCell>
              <TableCell>
                <Badge variant="outline" className={roleBadgeClass[u.role]}>
                  {u.role}
                </Badge>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {new Date(u.created_at).toLocaleDateString()}
              </TableCell>
              <TableCell>
                {u.role !== "superadmin" && u.id !== currentUser?.id && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-xs">
                        <MoreHorizontal className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {u.role === "user" && (
                        <DropdownMenuItem onClick={() => handleRoleChange(u.id, "admin")}>
                          Promote to admin
                        </DropdownMenuItem>
                      )}
                      {u.role === "admin" && (
                        <DropdownMenuItem onClick={() => handleRoleChange(u.id, "user")}>
                          Demote to user
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem
                        className="text-destructive"
                        onClick={() => setDeleteId(u.id)}
                      >
                        Delete user
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <ConfirmDialog
        open={deleteId !== null}
        onOpenChange={(open) => { if (!open) setDeleteId(null) }}
        title="Delete user"
        description={`Permanently delete ${deleteUser?.email ?? "this user"} and all their sites?`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        destructive
      />
    </>
  )
}
