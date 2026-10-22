// Browser storage is optional. Denied/quota-limited storage must not prevent login.
export function readPreference(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}
export function savePreference(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    /* Session state still works. */
  }
}
export function initialDarkMode(): boolean {
  const saved = readPreference("nines-theme");
  if (saved !== null) return saved === "dark";
  try {
    return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
  } catch {
    return false;
  }
}
