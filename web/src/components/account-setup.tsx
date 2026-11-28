import { useEffect, useState, type FormEvent } from "react";
import { Theme } from "@astryxdesign/core/theme";
import { ninesTheme } from "@/astryx-theme";
import { SignInPanel, SignInPassword } from "./ui/notchset/sign-in";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { initialDarkMode } from "@/lib/preferences";

// The token arrives in the URL fragment, which browsers never send to the
// server. Read it once and drop it from the address bar and history.
function takeToken(): string {
  const token = window.location.hash.replace(/^#/, "");
  if (token) window.history.replaceState(null, "", "/setup");
  return token;
}

// Where a person opens the setup link an administrator sent them, and
// chooses the password for their new local account.
export function AccountSetup() {
  const [token] = useState(takeToken);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState(
    token
      ? ""
      : "This page needs the full setup link your administrator sent you. Open the link again, or ask for a new one.",
  );
  const dark = initialDarkMode();
  useEffect(() => {
    document.title = "Set your password | Nines";
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);

  const mismatch = confirm !== "" && confirm !== password;
  const ready =
    token !== "" &&
    password.length >= 12 &&
    password.length <= 72 &&
    password === confirm;

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/auth/local/setup", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token, password }),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(
          body.error ?? "Your password could not be set. Try again.",
        );
      }
      setDone(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setPassword("");
      setConfirm("");
      setBusy(false);
    }
  }

  return (
    <Theme theme={ninesTheme} mode={dark ? "dark" : "light"}>
      <main
        data-private
        data-replay-block
        className="flex min-h-svh items-center justify-center auth-stage px-5 pb-10 pt-16 sm:py-10"
      >
        <SignInPanel
          title={done ? "Password set" : "Set your password"}
          description={
            done
              ? "Your password is set. Sign in with your username and new password."
              : "Choose the password for your Nines account. Only you will know it."
          }
        >
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          {done ? (
            <Button className="w-full" href="/login">
              Sign in
            </Button>
          ) : token ? (
            <form onSubmit={submit} className="space-y-4">
              <div>
                <Label htmlFor="setup-password">New password</Label>
                <SignInPassword
                  id="setup-password"
                  autoComplete="new-password"
                  required
                  minLength={12}
                  maxLength={72}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  aria-describedby="setup-password-hint"
                />
                <p
                  id="setup-password-hint"
                  className="mt-1 text-xs text-muted-foreground"
                >
                  12 to 72 characters.
                </p>
              </div>
              <div>
                <Label htmlFor="setup-confirm">Confirm password</Label>
                <SignInPassword
                  id="setup-confirm"
                  autoComplete="new-password"
                  required
                  maxLength={72}
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  aria-invalid={mismatch}
                />
                {mismatch ? (
                  <p className="mt-1 text-xs text-destructive">
                    The passwords do not match.
                  </p>
                ) : null}
              </div>
              <Button
                type="submit"
                className="w-full"
                disabled={busy || !ready}
              >
                {busy ? "Saving…" : "Set password"}
              </Button>
            </form>
          ) : null}
        </SignInPanel>
      </main>
    </Theme>
  );
}
