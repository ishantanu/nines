import * as React from "react";
import { Dialog as AstryxDialog } from "@astryxdesign/core/Dialog";
import { XIcon } from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";

// Adapter over Astryx Dialog (a native <dialog>, so focus trapping, Escape and
// backdrop dismissal come from the platform). Keeps the compound API: state
// and the title/description ids live in context.
type DialogState = {
  open: boolean;
  setOpen: (open: boolean) => void;
  titleId: string;
  descriptionId: string;
};
const DialogContext = React.createContext<DialogState | null>(null);
function useDialog(): DialogState {
  const state = React.useContext(DialogContext);
  if (!state) throw new Error("Dialog parts must be rendered inside Dialog");
  return state;
}

function Dialog({
  open: controlled,
  defaultOpen = false,
  onOpenChange,
  children,
}: {
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  children?: React.ReactNode;
}) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultOpen);
  const id = React.useId();
  const open = controlled ?? uncontrolled;
  const setOpen = React.useCallback(
    (next: boolean) => {
      if (controlled === undefined) setUncontrolled(next);
      onOpenChange?.(next);
    },
    [controlled, onOpenChange],
  );
  return (
    <DialogContext.Provider
      value={{
        open,
        setOpen,
        titleId: `${id}-title`,
        descriptionId: `${id}-description`,
      }}
    >
      {children}
    </DialogContext.Provider>
  );
}

type Clickable = {
  onClick?: (event: React.MouseEvent) => void;
  "aria-haspopup"?: string;
  "aria-expanded"?: boolean;
};

// Wires an existing element (the previous `asChild` pattern) to open/close.
function wire(
  child: React.ReactNode,
  action: () => void,
  extra: Clickable = {},
) {
  if (!React.isValidElement<Clickable>(child)) return child;
  return React.cloneElement(child, {
    ...extra,
    onClick: (event: React.MouseEvent) => {
      child.props.onClick?.(event);
      if (!event.defaultPrevented) action();
    },
  });
}

function DialogTrigger({
  children,
}: {
  asChild?: boolean;
  children: React.ReactNode;
}) {
  const { open, setOpen } = useDialog();
  return wire(children, () => setOpen(true), {
    "aria-haspopup": "dialog",
    "aria-expanded": open,
  });
}

function DialogClose({
  children,
}: {
  asChild?: boolean;
  children: React.ReactNode;
}) {
  const { setOpen } = useDialog();
  return wire(children, () => setOpen(false));
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  ...props
}: Omit<React.ComponentProps<"div">, "title"> & {
  showCloseButton?: boolean;
}) {
  const { open, setOpen, titleId, descriptionId } = useDialog();
  // Unmount when closed, as before, so closed dialogs never linger in the DOM.
  if (!open) return null;
  return (
    <AstryxDialog
      isOpen={open}
      onOpenChange={setOpen}
      width="100%"
      padding={6}
      role="dialog"
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
      data-slot="dialog-content"
      // No positioning class here: Astryx keeps the dialog position: fixed so
      // it stays centered in the viewport even when the page is scrolled. A
      // fixed element already anchors the absolute close button.
      className={cn("sm:max-w-lg", className)}
      {...(props as object)}
    >
      {children}
      {showCloseButton ? (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Close"
          type="button"
          className="absolute top-3 right-3"
          onClick={() => setOpen(false)}
        >
          <XIcon />
        </Button>
      ) : null}
    </AstryxDialog>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      className={cn("flex flex-col gap-2 pr-6 text-left", className)}
      {...props}
    />
  );
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & { showCloseButton?: boolean }) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn(
        "mt-2 flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end",
        className,
      )}
      {...props}
    >
      {children}
      {showCloseButton ? (
        <DialogClose asChild>
          <Button variant="outline">Close</Button>
        </DialogClose>
      ) : null}
    </div>
  );
}

function DialogTitle({ className, ...props }: React.ComponentProps<"h2">) {
  const { titleId } = useDialog();
  return (
    <h2
      id={titleId}
      data-slot="dialog-title"
      className={cn("text-lg leading-6 font-semibold", className)}
      {...props}
    />
  );
}

function DialogDescription({ className, ...props }: React.ComponentProps<"p">) {
  const { descriptionId } = useDialog();
  return (
    <p
      id={descriptionId}
      data-slot="dialog-description"
      className={cn("text-sm leading-6 text-muted-foreground", className)}
      {...props}
    />
  );
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
};
