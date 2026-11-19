import { SignInPanel, SignInPassword } from "./ui/notchset/sign-in";
import { NativeSelect } from "@/components/ui/select";
import { useState, type FormEvent } from "react";
import { LogIn } from "lucide-react";
import { authAction, message, type AuthInfo } from "@/lib/api";
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
      className="flex min-h-svh items-center justify-center auth-stage px-5 pb-10 pt-16 sm:py-10"
    >
      <SignInPanel description="Use your account or company sign-in.">
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
              href={`/auth/oidc/${encodeURIComponent(p.id)}/login`}
              icon={<LogIn />}
            >
              Continue with {p.name}
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
                <NativeSelect
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
                </NativeSelect>
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
              <SignInPassword
                id="login-password"
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
      </SignInPanel>
    </main>
  );
}
