import { Navigate, Outlet } from "react-router-dom"
import { useAuth } from "@/hooks/use-auth"

export function AdminRoute() {
  const { isAdmin, isLoading } = useAuth()

  if (isLoading) return null
  if (!isAdmin) return <Navigate to="/" replace />

  return <Outlet />
}
