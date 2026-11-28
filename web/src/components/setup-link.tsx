import { useState } from "react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

export type SetupLink = { url: string; expiresAt: string };

// Shows a new account's one-time setup link. It is not shown again, so the
// administrator copies it here and sends it to the person.
export function SetupLinkNotice({
  link,
  person,
  onDone,
}: {
  link: SetupLink;
  person: string;
  onDone: () => void;
}) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(link.url);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }
  return (
    <div className="space-y-4" data-private data-replay-block>
      <p className="text-sm">
        Send this link to <strong>{person}</strong>. They open it to choose
        their own password. It works once and expires{" "}
        {new Date(link.expiresAt).toLocaleString()}.
      </p>
      <div>
        <Label htmlFor="setup-link">Setup link</Label>
        <Input
          id="setup-link"
          readOnly
          value={link.url}
          className="mt-2 font-mono text-xs"
          onFocus={(e) => e.currentTarget.select()}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        This link is not shown again. If it is lost or expires, issue a new one
        from Users.
      </p>
      <div className="dialog-actions">
        <Button type="button" variant="outline" onClick={() => void copy()}>
          {copied ? "Copied" : "Copy link"}
        </Button>
        <Button type="button" onClick={onDone}>
          Done
        </Button>
      </div>
    </div>
  );
}
