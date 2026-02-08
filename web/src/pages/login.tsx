import { useState, type FormEvent } from "react"
import { Link, Navigate, useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"
import { useAuth } from "@/hooks/use-auth"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export default function LoginPage() {
  const { user, login } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [loading, setLoading] = useState(false)

  if (user) return <Navigate to="/" replace />

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    try {
      await login(email, password)
      navigate("/")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Login failed")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative min-h-screen flex items-center justify-center bg-background overflow-hidden">
      {/* Dot grid background */}
      <div
        className="absolute inset-0 opacity-[0.03]"
        style={{
          backgroundImage: "radial-gradient(circle, hsl(var(--foreground)) 1px, transparent 1px)",
          backgroundSize: "24px 24px",
        }}
      />

      <div className="relative w-full max-w-sm px-4">
        {/* Logo */}
        <div className="mb-8 text-center">
          <h1 className="text-xl font-mono font-bold tracking-tight text-foreground">
            <span className="text-muted-foreground">//</span>hostedat
          </h1>
          <p className="mt-2 text-xs text-muted-foreground tracking-wide uppercase">
            Static site hosting
          </p>
        </div>

        {/* Login card */}
        <div className="rounded-lg border border-border bg-card p-6">
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
                autoFocus
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoComplete="current-password"
              />
            </div>

            <Button
              type="submit"
              className="w-full bg-emerald-600 text-white hover:bg-emerald-500"
              disabled={loading}
            >
              {loading ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Signing in...
                </>
              ) : (
                "Sign in"
              )}
            </Button>
          </form>

          <div className="mt-4 text-center">
            <span className="text-sm text-muted-foreground">
              No account?{" "}
              <Link
                to="/register"
                className="text-foreground underline underline-offset-4 hover:text-emerald-400 transition-colors"
              >
                Register
              </Link>
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
