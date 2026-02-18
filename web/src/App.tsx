import { lazy, Suspense } from "react"
import { createBrowserRouter, RouterProvider } from "react-router-dom"
import { AuthProvider } from "@/contexts/auth-context"
import { Toaster } from "@/components/ui/sonner"
import { AppLayout } from "@/components/layout/app-layout"
import { ProtectedRoute } from "@/components/shared/protected-route"
import { AdminRoute } from "@/components/shared/admin-route"

const LoginPage = lazy(() => import("@/pages/login"))
const RegisterPage = lazy(() => import("@/pages/register"))
const DashboardPage = lazy(() => import("@/pages/dashboard"))
const SiteDetailPage = lazy(() => import("@/pages/site-detail"))
const APIKeysPage = lazy(() => import("@/pages/api-keys"))
const AdminUsersPage = lazy(() => import("@/pages/admin/users"))
const AdminSettingsPage = lazy(() => import("@/pages/admin/settings"))
const AdminInvitesPage = lazy(() => import("@/pages/admin/invites"))
const NotFoundPage = lazy(() => import("@/pages/not-found"))

function LazyPage({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={null}>{children}</Suspense>
}

const router = createBrowserRouter([
  { path: "/login", element: <LazyPage><LoginPage /></LazyPage> },
  { path: "/register", element: <LazyPage><RegisterPage /></LazyPage> },
  {
    element: <ProtectedRoute />,
    children: [
      {
        element: <AppLayout />,
        children: [
          { path: "/", element: <LazyPage><DashboardPage /></LazyPage> },
          { path: "/sites/:id", element: <LazyPage><SiteDetailPage /></LazyPage> },
          { path: "/keys", element: <LazyPage><APIKeysPage /></LazyPage> },
          {
            element: <AdminRoute />,
            children: [
              { path: "/admin/users", element: <LazyPage><AdminUsersPage /></LazyPage> },
              { path: "/admin/settings", element: <LazyPage><AdminSettingsPage /></LazyPage> },
              { path: "/admin/invites", element: <LazyPage><AdminInvitesPage /></LazyPage> },
            ],
          },
        ],
      },
    ],
  },
  { path: "*", element: <LazyPage><NotFoundPage /></LazyPage> },
])

export default function App() {
  return (
    <AuthProvider>
      <RouterProvider router={router} />
      <Toaster />
    </AuthProvider>
  )
}
