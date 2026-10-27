import { LoginFrame } from "./login-frame";
import { Brand } from "@/components/brand";
import { useState, type FormEvent } from "react";
import { LogIn } from "lucide-react";
import { authAction, message, type AuthInfo } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
export function Login({
  auth,
  onSignedIn,
  onToken,
}: {
  auth: AuthInfo;
  onSignedIn: () => void;
  onToken: () => void;
}) {
  const directories = auth.providers.filter(
    (p) => p.type === "ldap" || p.type === "local",
  );
  const [provider, setProvider] = useState(directories[0]?.id ?? "");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(
    new URLSearchParams(
      typeof window === "undefined" ? "" : window.location.search,
    ).has("auth_error")
      ? "Sign-in could not be completed. Try again or contact your administrator."
      : "",
  );
  async function login(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await authAction(
        `/auth/${directories.find((p) => p.id === provider)?.type === "local" ? "local" : "ldap"}/${encodeURIComponent(provider)}/login`,
        auth.csrfToken ?? "",
        { username, password },
      );
      onSignedIn();
    } catch (e) {
      setError(message(e));
    } finally {
      setPassword("");
      setBusy(false);
    }
  }
  return (
    <main
      data-private
      data-replay-block
      className="flex min-h-svh items-center justify-center bg-background px-5 py-10"
    >
      <LoginFrame>
        <Card className="w-full gap-0 py-0">
          <div className="login-form-heading space-y-3 border-b p-6">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <Brand />
            </div>
            <h1 className="text-2xl font-semibold tracking-tight">Sign in</h1>
            <p className="text-sm text-muted-foreground">
              Use your account or company sign-in.
            </p>
          </div>
          <CardContent className="space-y-5 p-6">
            {error ? (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            ) : null}
            {auth.providers
              .filter((p) => p.type === "oidc")
              .map((p) => (
                <Button
                  key={p.id}
                  variant="outline"
                  className="w-full justify-start"
                  asChild
                >
                  <a href={`/auth/oidc/${encodeURIComponent(p.id)}/login`}>
                    <LogIn className="size-4" />
                    Continue with {p.name}
                  </a>
                </Button>
              ))}
            {directories.length ? (
              <form onSubmit={login} className="space-y-4">
                <p className="text-sm font-medium">
                  {auth.providers.some((p) => p.type === "oidc")
                    ? "Or use your directory account"
                    : "Username and password"}
                </p>
                {directories.length > 1 ? (
                  <div>
                    <Label htmlFor="login-provider">Directory</Label>
                    <select
                      id="login-provider"
                      className="mt-2 h-9 w-full rounded-md border bg-background px-3 text-sm"
                      value={provider}
                      onChange={(e) => setProvider(e.target.value)}
                    >
                      {directories.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                    </select>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    {directories[0].name}
                  </p>
                )}
                <div>
                  <Label htmlFor="login-username">Username</Label>
                  <Input
                    id="login-username"
                    autoComplete="username"
                    required
                    maxLength={256}
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    className="mt-2"
                  />
                </div>
                <div>
                  <Label htmlFor="login-password">Password</Label>
                  <Input
                    id="login-password"
                    type="password"
                    autoComplete="current-password"
                    required
                    maxLength={4096}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="mt-2"
                  />
                </div>
                <Button
                  type="submit"
                  className="w-full"
                  disabled={busy || !username || !password}
                >
                  {busy ? "Signing in…" : "Sign in"}
                </Button>
              </form>
            ) : null}
            <div className="border-t pt-4">
              <Button
                variant="ghost"
                size="sm"
                className="-ml-2 text-muted-foreground"
                onClick={onToken}
              >
                Use an API credential
              </Button>
            </div>
          </CardContent>
        </Card>
      </LoginFrame>
    </main>
  );
}
