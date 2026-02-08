import { Link, useLocation } from "react-router-dom"
import { Globe, Key, Users, Settings, Ticket, LogOut } from "lucide-react"
import { cn } from "@/lib/utils"
import { useAuth } from "@/hooks/use-auth"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"

const navItems = [
  { to: "/", label: "Sites", icon: Globe },
  { to: "/keys", label: "API Keys", icon: Key },
]

const adminItems = [
  { to: "/admin/users", label: "Users", icon: Users },
  { to: "/admin/settings", label: "Settings", icon: Settings },
  { to: "/admin/invites", label: "Invites", icon: Ticket },
]

export function Sidebar() {
  const location = useLocation()
  const { user, isAdmin, logout } = useAuth()

  return (
    <aside className="flex flex-col w-60 border-r border-border bg-card h-screen sticky top-0">
      <div className="p-4">
        <Link to="/" className="text-lg font-bold tracking-tight">
          hostedat
        </Link>
      </div>

      <nav className="flex-1 px-2 space-y-1">
        {navItems.map((item) => {
          const active = item.to === "/"
            ? location.pathname === "/"
            : location.pathname.startsWith(item.to)
          return (
            <Link
              key={item.to}
              to={item.to}
              className={cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                active
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
              )}
            >
              <item.icon className="size-4" />
              {item.label}
            </Link>
          )
        })}

        {isAdmin && (
          <>
            <Separator className="my-3" />
            <p className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              Admin
            </p>
            {adminItems.map((item) => {
              const active = location.pathname.startsWith(item.to)
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  className={cn(
                    "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    active
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                  )}
                >
                  <item.icon className="size-4" />
                  {item.label}
                </Link>
              )
            })}
          </>
        )}
      </nav>

      <div className="p-4 border-t border-border">
        <div className="flex items-center justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium truncate">{user?.email}</p>
            <p className="text-xs text-muted-foreground capitalize">{user?.role}</p>
          </div>
          <Button variant="ghost" size="icon" onClick={logout} title="Log out">
            <LogOut className="size-4" />
          </Button>
        </div>
      </div>
    </aside>
  )
}
