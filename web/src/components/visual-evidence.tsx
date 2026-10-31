import { useEffect, useRef, useState } from "react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Textarea } from "./ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import { date, message, percent, type RequestScope } from "@/lib/api";
import {
  createVisual,
  listVisual,
  readVisualFile,
  reviewVisual,
  visualCapabilities,
  visualNotice,
  type VisualReceipt,
} from "@/lib/visual-evaluations";

type LocalVisualEvidence = { id: string; preview: string; criterion: string };

const ALLOWED_VISUAL_MIME_TYPES = new Set(["image/png", "image/jpeg"]);

function Receipt({
  receipt,
  scope,
  onSaved,
  localEvidence,
  onForget,
}: {
  localEvidence?: LocalVisualEvidence;
  onForget: () => void;
  receipt: VisualReceipt;
  scope: RequestScope;
  onSaved: (value: VisualReceipt) => void;
}) {
  const [outcome, setOutcome] = useState("inconclusive");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const canReview =
    ["owner", "editor"].includes(scope.role) &&
    receipt.assessment.status === "assessed" &&
    !receipt.review;
  const j = receipt.assessment.judgments;
  return (
    <details className="border-t py-4" data-replay-block>
      <summary className="cursor-pointer text-sm font-medium">
        {date(receipt.createdAt)} · {receipt.route.model} ·{" "}
        {receipt.review
          ? `Reviewed: ${receipt.review.outcome}`
          : receipt.assessment.status === "assessed"
            ? "Needs human review"
            : receipt.assessment.status.replaceAll("_", " ")}
      </summary>
      <div className="mt-4 space-y-4 text-sm">
        {localEvidence ? (
          <div className="space-y-3">
            <p className="text-xs text-muted-foreground">
              This original image and criterion remain only in this tab for your
              review. They disappear when cleared, reviewed or you leave this
              journey.
            </p>
            <img
              src={localEvidence.preview}
              alt="Original screenshot held locally for this receipt review"
              className="max-h-80 max-w-full rounded-md border object-contain"
            />
            <p className="whitespace-pre-wrap break-words">
              {localEvidence.criterion}
            </p>
            <Button size="sm" variant="outline" onClick={onForget}>
              Clear local evidence
            </Button>
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            The original image and criterion are not stored by Nines. Inspect
            your original evidence before reviewing this receipt; use
            Inconclusive when it is unavailable.
          </p>
        )}

        {j ? (
          <>
            <p className="text-muted-foreground">
              Model judgments about visible evidence. These probabilities are
              not SLI values or proof of backend success.
            </p>
            <dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2">
              <dt>Success visible</dt>
              <dd className="tabular-nums">{percent(j.successVisible, 1)}</dd>
              <dt>Application error visible</dt>
              <dd className="tabular-nums">{percent(j.errorVisible, 1)}</dd>
              <dt>Sufficient visual evidence</dt>
              <dd className="tabular-nums">
                {percent(j.evidenceSufficient, 1)}
              </dd>
            </dl>
          </>
        ) : (
          <p className="text-muted-foreground">
            No usable judgment was recorded. This is an evaluator gap, not an
            application failure. Pending receipts are not automatically retried.
          </p>
        )}
        <dl className="space-y-2 text-xs text-muted-foreground">
          <div>
            <dt>Journey revision</dt>
            <dd>{receipt.journeyRevision}</dd>
          </div>
          <div>
            <dt>Image SHA-256</dt>
            <dd className="break-all font-mono">{receipt.imageDigest}</dd>
          </div>
          <div>
            <dt>Sharing authorization</dt>
            <dd>
              {receipt.consent.actor.name} · {date(receipt.consent.at)} ·{" "}
              {receipt.consent.noticeVersion}
            </dd>
          </div>
          <div>
            <dt>Receipt expires</dt>
            <dd>{date(receipt.expiresAt)}</dd>
          </div>
        </dl>
        {receipt.review ? (
          <div className="space-y-1">
            <p>
              {receipt.review.actor.name} recorded{" "}
              <strong>{receipt.review.outcome}</strong>.
            </p>
            <p className="whitespace-pre-wrap break-words text-muted-foreground">
              {receipt.review.reason}
            </p>
          </div>
        ) : null}
        {canReview ? (
          <form
            className="space-y-3"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                const value = await reviewVisual(
                  scope,
                  receipt,
                  outcome,
                  reason,
                );
                if (!scope.signal.aborted) onSaved(value);
              } catch (e) {
                if (!scope.signal.aborted) setError(message(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            <Label htmlFor={`visual-outcome-${receipt.id}`}>
              Your assessment
            </Label>
            <Select value={outcome} onValueChange={setOutcome}>
              <SelectTrigger id={`visual-outcome-${receipt.id}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="success">Success</SelectItem>
                <SelectItem value="failure">Failure</SelectItem>
                <SelectItem value="inconclusive">Inconclusive</SelectItem>
              </SelectContent>
            </Select>
            <Label htmlFor={`visual-reason-${receipt.id}`}>Review reason</Label>
            <Textarea
              id={`visual-reason-${receipt.id}`}
              value={reason}
              maxLength={2000}
              required
              onChange={(e) => setReason(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              This saves your review with the receipt. It does not submit run
              evidence, publish metrics or change an SLO. Reviews are immutable.
            </p>
            <Button size="sm" disabled={busy || !reason.trim()}>
              {busy ? "Saving review…" : "Save review"}
            </Button>
          </form>
        ) : null}
        {error ? (
          <p role="alert" className="text-destructive">
            {error}
          </p>
        ) : null}
      </div>
    </details>
  );
}
export function VisualEvidence({
  scope,
  journeyID,
  revision,
}: {
  scope: RequestScope;
  journeyID: string;
  revision: number;
}) {
  const [route, setRoute] = useState<{
    enabled: boolean;
    routeDigest: string;
    route?: { provider: string; model: string };
  } | null>(null);
  const [items, setItems] = useState<VisualReceipt[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [criterion, setCriterion] = useState("");
  const [consent, setConsent] = useState(false);
  const [image, setImage] = useState("");
  const [localEvidence, setLocalEvidence] =
    useState<LocalVisualEvidence | null>(null);
  const [preview, setPreview] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const pending = useRef("");
  const selection = useRef(0);
  const writable = ["owner", "editor"].includes(scope.role);
  useEffect(() => {
    let current = true;
    setRoute(null);
    setLocalEvidence(null);
    setItems([]);
    setConsent(false);
    setCriterion("");
    setImage("");
    pending.current = "";
    selection.current++;
    Promise.all([visualCapabilities(scope), listVisual(scope, journeyID)])
      .then(([cap, rows]) => {
        if (current && !scope.signal.aborted) {
          if (
            rows.some(
              (r) => r.tenant !== scope.tenant || r.journeyId !== journeyID,
            )
          )
            throw Error("Visual evidence scope mismatch");
          setRoute(cap);
          setItems(rows);
          setError("");
        }
      })
      .catch((e) => {
        if (current && !scope.signal.aborted) setError(message(e));
      });
    return () => {
      current = false;
      selection.current++;
    };
  }, [scope, journeyID, revision]);

  function accept(r: VisualReceipt) {
    if (
      !scope.signal.aborted &&
      r.tenant === scope.tenant &&
      r.journeyId === journeyID
    ) {
      setItems((old) => [r, ...old.filter((x) => x.id !== r.id)].slice(0, 100));
      if (r.review) setLocalEvidence((old) => (old?.id === r.id ? null : old));
    }
  }
  return (
    <section className="mt-6 space-y-4 border-t pt-6" data-replay-block>
      <div className="space-y-2">
        <h3 className="text-base font-semibold">Visual evidence</h3>
        <p className="max-w-prose text-sm text-muted-foreground">
          Experimental screenshot assessment for this critical user journey. An
          engineer reviews the result separately; visual judgments do not
          establish end-to-end reliability.
        </p>
      </div>
      {!route ? (
        <p className="text-sm text-muted-foreground">
          {error
            ? "Visual evaluation is unavailable."
            : "Loading visual evaluation…"}
        </p>
      ) : !route.enabled ? (
        <p className="text-sm text-muted-foreground">
          Image assessments are disabled. A tenant administrator can configure
          Jev and explicitly enable <code>systemOne.imagesEnabled</code>.
        </p>
      ) : writable ? (
        <form
          className="max-w-3xl space-y-5"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!consent || !image || busy) return;
            setBusy(true);
            setError("");
            pending.current ||= crypto.randomUUID();
            try {
              const r = await createVisual(scope, journeyID, {
                id: pending.current,
                routeDigest: route.routeDigest,
                journeyRevision: revision,
                successCriterion: criterion,
                imageBase64: image,
                consent,
                noticeVersion: visualNotice,
              });
              accept(r);
              if (!scope.signal.aborted && r.assessment.status === "assessed")
                setLocalEvidence({ id: r.id, preview, criterion });
              setImage("");
              setPreview("");
              setCriterion("");
              pending.current = "";
              if (file.current) file.current.value = "";
            } catch (e) {
              if (!scope.signal.aborted) setError(message(e));
            } finally {
              setBusy(false);
              setConsent(false);
            }
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="visual-image">Screenshot</Label>
            <Input
              ref={file}
              id="visual-image"
              type="file"
              accept="image/png,image/jpeg"
              disabled={busy}
              onChange={async (e) => {
                const chosen = e.target.files?.[0];
                const version = ++selection.current;
                setConsent(false);
                setImage("");
                setPreview("");
                pending.current = "";
                setError("");
                if (!chosen) return;
                const mimeType = chosen.type.toLowerCase();
                if (!ALLOWED_VISUAL_MIME_TYPES.has(mimeType)) {
                  setError("Please upload a PNG or JPEG image.");
                  return;
                }
                try {
                  const value = await readVisualFile(chosen);
                  if (version !== selection.current || scope.signal.aborted)
                    return;
                  setImage(value);
                  setPreview(`data:${mimeType};base64,${value}`);
                } catch (e) {
                  if (version === selection.current) setError(message(e));
                }
              }}
            />
            <p className="text-xs text-muted-foreground">
              PNG or JPEG, at most 2 MiB, 280–1568 pixels on each side. Redact
              personal data, secrets and unrelated content before selecting the
              image.
            </p>
          </div>
          {preview ? (
            <img
              src={preview}
              alt="Selected screenshot for your review before sharing"
              className="max-h-80 max-w-full rounded-md border object-contain"
            />
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="visual-criterion">Visible success criterion</Label>
            <Textarea
              id="visual-criterion"
              value={criterion}
              maxLength={2000}
              required
              disabled={busy}
              placeholder="For example: an order confirmation is visible with no payment error."
              onChange={(e) => {
                setCriterion(e.target.value);
                setConsent(false);
                pending.current = "";
              }}
            />
          </div>
          <div className="space-y-3 rounded-md border p-4 text-sm">
            <p>
              The selected image and criterion will be sent to{" "}
              <strong>
                {route.route?.provider === "typesafe"
                  ? "TypeSafe"
                  : route.route?.provider}
              </strong>{" "}
              using {route.route?.model}. Nines removes image metadata before
              sending, but does not automatically redact the pixels. Nines
              stores a hash, consent, model judgments and review for 30 days,
              not the image or criterion text. The provider processes the image
              under the operator’s provider agreement.
            </p>
            <Label className="flex items-start gap-3 text-sm">
              <input
                type="checkbox"
                className="mt-0.5 size-4 shrink-0 accent-primary"
                checked={consent}
                disabled={busy || !image || !criterion.trim()}
                onChange={(e) => setConsent(e.target.checked)}
              />
              <span>
                I have permission to share this image and criterion with this
                provider for this one assessment.
              </span>
            </Label>
          </div>
          <Button
            size="sm"
            disabled={busy || !consent || !image || !criterion.trim()}
          >
            {busy ? "Assessing image…" : "Share image and assess"}
          </Button>
        </form>
      ) : (
        <p className="text-sm text-muted-foreground">
          An owner or editor must explicitly authorize image sharing and review.
        </p>
      )}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      <div>
        <h4 className="mb-2 text-sm font-medium">Assessment receipts</h4>
        {items.length ? (
          items.map((r) => (
            <Receipt
              key={r.id}
              receipt={r}
              scope={scope}
              onSaved={accept}
              localEvidence={
                localEvidence?.id === r.id ? localEvidence : undefined
              }
              onForget={() => setLocalEvidence(null)}
            />
          ))
        ) : (
          <p className="text-sm text-muted-foreground">
            No visual assessments recorded for this journey.
          </p>
        )}
      </div>
    </section>
  );
}
