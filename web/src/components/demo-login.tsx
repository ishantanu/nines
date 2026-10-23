import { Brand } from "@/components/brand";
import { useState, type FormEvent } from "react";
import { LogIn, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function DemoLogin({
  darkMode,
  onToggleTheme,
  onSignIn,
  replayConfigured,
  replayConsent,
}: {
  darkMode: boolean;
  onToggleTheme: () => void;
  onSignIn: (allowReplay: boolean) => void;
  replayConfigured: boolean;
  replayConsent: boolean;
}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin");
  const [allowReplay, setAllowReplay] = useState(replayConsent);
  const [error, setError] = useState("");

  function login(event: FormEvent) {
    event.preventDefault();
    if (username !== "admin" || password !== "admin") {
      setError("Invalid demo username or password.");
      return;
    }
    setError("");
    onSignIn(allowReplay);
  }

  return (
    <main
      data-replay-block
      data-private
      className="demo-login min-h-svh bg-background px-5 py-10 text-foreground"
    >
      <Button
        variant="ghost"
        size="icon"
        className="fixed right-4 top-4"
        onClick={onToggleTheme}
        aria-label={darkMode ? "Switch to light mode" : "Switch to dark mode"}
        aria-pressed={darkMode}
        title={darkMode ? "Switch to light mode" : "Switch to dark mode"}
      >
        {darkMode ? (
          <Sun className="size-4" aria-hidden="true" />
        ) : (
          <Moon className="size-4" aria-hidden="true" />
        )}
      </Button>
      <div className="demo-login-content mx-auto flex min-h-[calc(100svh-5rem)] w-full max-w-md items-center">
        <Card className="demo-login-card w-full gap-0 overflow-hidden py-0">
          <div className="demo-login-heading space-y-3 border-b p-6">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <Brand />
            </div>
            <h1 className="text-2xl font-semibold tracking-tight">Sign in</h1>
            <p className="text-sm text-muted-foreground">
              Sample SLOs and critical user journeys.
            </p>
          </div>
          <CardContent className="space-y-5 p-6">
            <div className="demo-credentials rounded-md border bg-muted/50 p-4 text-sm">
              <p className="font-medium">Demo credentials</p>
              <dl className="mt-3 grid grid-cols-[1fr_auto] gap-x-6 gap-y-2 text-muted-foreground">
                <dt>Username</dt>
                <dd className="font-mono text-foreground">admin</dd>
                <dt>Password</dt>
                <dd className="font-mono text-foreground">admin</dd>
              </dl>
            </div>
            <form onSubmit={login} className="space-y-4">
              {error ? (
                <p role="alert" className="text-sm text-destructive">
                  {error}
                </p>
              ) : null}
              <div>
                <Label htmlFor="demo-username">Username</Label>
                <Input
                  id="demo-username"
                  autoComplete="username"
                  required
                  maxLength={256}
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  className="mt-2"
                />
              </div>
              <div>
                <Label htmlFor="demo-password">Password</Label>
                <Input
                  id="demo-password"
                  type="password"
                  autoComplete="current-password"
                  required
                  maxLength={256}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="mt-2"
                />
              </div>
              {replayConfigured ? (
                <label className="flex items-start gap-2 text-sm leading-5 text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={allowReplay}
                    onChange={(event) => setAllowReplay(event.target.checked)}
                    className="mt-1 accent-primary"
                  />
                  <span>
                    Allow operator diagnostics through PostHog for this demo
                    session. All text and form inputs are hidden from
                    recordings.
                  </span>
                </label>
              ) : (
                <p className="text-xs leading-5 text-muted-foreground">
                  Record and view masked sessions from Session replay after
                  signing in.
                </p>
              )}
              <Button type="submit" className="w-full">
                <LogIn className="size-4" aria-hidden="true" />
                Sign in
              </Button>
            </form>
            <p className="border-t pt-4 text-xs leading-5 text-muted-foreground">
              Demo sign-in is for the sample workspace only and is not an
              access-control mechanism.
            </p>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
