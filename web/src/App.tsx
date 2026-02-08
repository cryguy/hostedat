import { createBrowserRouter, RouterProvider } from "react-router-dom"
import { AuthProvider } from "@/contexts/auth-context"
import { Toaster } from "@/components/ui/sonner"
import { AppLayout } from "@/components/layout/app-layout"
import { ProtectedRoute } from "@/components/shared/protected-route"
import { AdminRoute } from "@/components/shared/admin-route"
import LoginPage from "@/pages/login"
import RegisterPage from "@/pages/register"
import DashboardPage from "@/pages/dashboard"
import SiteDetailPage from "@/pages/site-detail"
import APIKeysPage from "@/pages/api-keys"
import AdminUsersPage from "@/pages/admin/users"
import AdminSettingsPage from "@/pages/admin/settings"
import AdminInvitesPage from "@/pages/admin/invites"
import NotFoundPage from "@/pages/not-found"

const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  { path: "/register", element: <RegisterPage /> },
  {
    element: <ProtectedRoute />,
    children: [
      {
        element: <AppLayout />,
        children: [
          { path: "/", element: <DashboardPage /> },
          { path: "/sites/:id", element: <SiteDetailPage /> },
          { path: "/keys", element: <APIKeysPage /> },
          {
            element: <AdminRoute />,
            children: [
              { path: "/admin/users", element: <AdminUsersPage /> },
              { path: "/admin/settings", element: <AdminSettingsPage /> },
              { path: "/admin/invites", element: <AdminInvitesPage /> },
            ],
          },
        ],
      },
    ],
  },
  { path: "*", element: <NotFoundPage /> },
])

export default function App() {
  return (
    <AuthProvider>
      <RouterProvider router={router} />
      <Toaster />
    </AuthProvider>
  )
}
