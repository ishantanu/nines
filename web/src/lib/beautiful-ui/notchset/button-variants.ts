// Beautiful UI Notchset v1.0.0 (MIT), Copyright (c) 2026 Chimpy.
// Source: https://beautiful-ui.dev/r/radix-nova/sign-in.json
// Notice: /beautiful-ui-license.txt. Nines changes: account-form slot, optional
// passwordless/signup controls, accessible heading, scoped fonts and no sound.
import { cva } from "class-variance-authority";

/*
 * Notchset Button's variants, server-safe like shadcn's: call buttonVariants() from a Server Component
 * (links styled as buttons). The interactive Button lives in a client module.
 */

export const buttonVariants = cva(
  "group/button relative isolate inline-flex shrink-0 cursor-pointer items-center justify-center gap-(--b-gap) rounded-none border-0 font-mono font-medium tracking-[0.06em] whitespace-nowrap uppercase outline-none select-none transition-[background-color,color,translate,border-color] duration-[var(--notchset-fade),var(--notchset-fade),var(--notchset-key-up),var(--notchset-fade)] ease-[var(--notchset-ease-fade),var(--notchset-ease-fade),var(--notchset-ease-key-up),var(--notchset-ease-fade)] disabled:cursor-not-allowed disabled:opacity-40 aria-disabled:cursor-not-allowed aria-disabled:opacity-40 aria-busy:cursor-progress [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-(--b-ic) aria-pressed:bg-primary aria-pressed:text-primary-foreground data-[mode=armed]:bg-[var(--notchset-signal-text,var(--destructive))] data-[mode=armed]:text-primary-foreground data-[mode=hold]:translate-x-[2px] data-[mode=hold]:translate-y-[2px] data-[mode=hold]:duration-[var(--notchset-key-down)] data-[mode=hold]:ease-[var(--notchset-ease-key-down)]",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground hover:bg-[var(--notchset-primary-hover,color-mix(in_oklab,var(--primary)_86%,var(--background)))] active:bg-[var(--notchset-primary-press,var(--primary))] [--b-tick:var(--notchset-tick-on-primary,color-mix(in_oklab,var(--primary-foreground)_34%,transparent))] [--b-kbd:var(--notchset-kbd-on-primary,color-mix(in_oklab,var(--primary-foreground)_45%,transparent))]",
        destructive:
          "bg-[var(--notchset-signal,var(--destructive))] text-[color:var(--notchset-on-signal,white)] hover:bg-[var(--notchset-signal-hover,var(--destructive))] active:bg-[var(--notchset-signal-press,var(--destructive))] [--b-tick:var(--notchset-tick-on-primary,color-mix(in_oklab,white_34%,transparent))] [--b-kbd:var(--notchset-kbd-on-signal,color-mix(in_oklab,black_40%,transparent))] [--notchset-focus-color:var(--notchset-signal-text,var(--destructive))]",
        outline:
          "border border-solid border-foreground bg-transparent text-foreground hover:bg-accent active:bg-[var(--notchset-tint-press,var(--accent))]",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-[var(--notchset-secondary-hover,var(--secondary))] active:bg-[var(--notchset-secondary-press,var(--secondary))]",
        signal:
          "border border-solid border-[var(--notchset-signal-text,var(--destructive))] bg-background text-[color:var(--notchset-signal-text,var(--destructive))] hover:bg-[var(--notchset-tint,color-mix(in_oklab,var(--destructive)_12%,var(--background)))] [--notchset-button-fill:color-mix(in_oklab,var(--notchset-signal,var(--destructive))_26%,var(--background))] [--notchset-button-meter:var(--notchset-signal,var(--destructive))] [--notchset-focus-color:var(--notchset-signal-text,var(--destructive))]",
        ghost:
          "bg-transparent text-foreground hover:bg-accent active:bg-[var(--notchset-tint-press,var(--accent))]",
        link: "h-auto min-w-0 justify-start bg-transparent px-0.5 pt-[3px] pb-1.5 text-foreground active:text-[var(--notchset-signal-text,var(--destructive))]",
        plate:
          "bg-primary text-primary-foreground hover:bg-[var(--notchset-primary-hover,color-mix(in_oklab,var(--primary)_86%,var(--background)))] active:translate-x-[2px] active:translate-y-[2px] active:duration-[var(--notchset-key-down)] active:ease-[var(--notchset-ease-key-down)]",
      },
      size: {
        // The market's heights (shadcn, and Kobra on it): XS 24, S 28, default 32, L 36. Labels are mono caps,
        // which read larger than the 14px sans those libraries use, so 12px (13px on L) matches them optically.
        // Same size names as shadcn, so it stays a drop-in. At least 44px tall on touch screens.
        default:
          "h-8 min-w-8 px-3 text-[12px] [--b-gap:8px] [--b-ic:16px] [--b-kh:18px] [--b-kfs:10px] pointer-coarse:min-h-11",
        xs: "h-6 min-w-6 px-2 text-[12px] [--b-gap:6px] [--b-ic:14px] [--b-kh:14px] [--b-kfs:10px] pointer-coarse:min-h-11",
        sm: "h-7 min-w-7 px-2.5 text-[12px] [--b-gap:7px] [--b-ic:16px] [--b-kh:16px] [--b-kfs:10px] pointer-coarse:min-h-11",
        lg: "h-9 min-w-9 px-4 text-[13px] [--b-gap:8px] [--b-ic:16px] [--b-kh:20px] [--b-kfs:10px] pointer-coarse:min-h-11",
        icon: "size-8 [--b-ic:16px]",
        "icon-xs": "size-6 [--b-ic:14px]",
        "icon-sm": "size-7 [--b-ic:16px]",
        "icon-lg": "size-9 [--b-ic:16px]",
      },
    },
    // The link's drawn rule lives in the Notchset foundation (a system class, kept out of the utilities).
    // The shared press (a 1px content dip) on every variant but plate, which moves whole onto its plate.
    compoundVariants: [
      { variant: "link", class: `notchset-link` },
      {
        variant: [
          "default",
          "destructive",
          "outline",
          "secondary",
          "signal",
          "ghost",
          "link",
        ],
        class: `notchset-press`,
      },
    ],
    defaultVariants: { variant: "default", size: "default" },
  },
);

export type ButtonVariant = NonNullable<
  Parameters<typeof buttonVariants>[0]
>["variant"];
export type ButtonSize = NonNullable<
  Parameters<typeof buttonVariants>[0]
>["size"];
