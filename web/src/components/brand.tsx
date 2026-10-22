import { cn } from "@/lib/utils";

// Both files are emitted from docs/public/brand by the shared siteBrand plugin.
export function Brand({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex h-8 shrink-0", className)}>
      <img
        src="/nines.svg"
        alt="Nines"
        width="124"
        height="35"
        className="h-full w-auto dark:hidden"
      />
      <img
        src="/nines-dark.svg"
        alt="Nines"
        width="124"
        height="35"
        className="hidden h-full w-auto dark:block"
      />
    </span>
  );
}
