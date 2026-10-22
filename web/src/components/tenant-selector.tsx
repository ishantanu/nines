import { useState } from "react";
import { Check, Layers3, ChevronsUpDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import type { Tenant } from "@/lib/api";

export function TenantSelector({
  tenants,
  selected,
  onChange,
}: {
  tenants: Tenant[];
  selected: string;
  onChange: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="min-w-0 shrink"
          aria-label="Switch tenant"
        >
          <Layers3 className="size-4" />
          <span className="max-w-40 truncate">
            {tenants.find((t) => t.id === selected)?.name ?? selected}
          </span>
          <ChevronsUpDown
            className="size-3.5 text-muted-foreground"
            aria-hidden="true"
          />
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Switch tenant</DialogTitle>
          <DialogDescription>
            Views, queries, workflows, and changes use the selected tenant.
            Switching closes open editors and clears their drafts.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          {tenants.map((tenant) => (
            <Button
              key={tenant.id}
              className="h-auto w-full justify-between py-3"
              variant={tenant.id === selected ? "secondary" : "outline"}
              onClick={() => {
                setOpen(false);
                if (tenant.id !== selected) onChange(tenant.id);
              }}
              aria-current={tenant.id === selected ? "true" : undefined}
            >
              <span className="text-left">
                <span className="block">{tenant.name}</span>
                <span className="block text-xs font-normal text-muted-foreground">
                  {tenant.id}
                </span>
              </span>
              {tenant.id === selected ? <Check className="size-4" /> : null}
            </Button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
