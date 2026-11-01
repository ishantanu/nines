---
name: Nines Public Site
description: RFC Folio visual system for the Nines public website and documentation
colors:
  folio-paper: "#edf3f4"
  folio-sheet: "#fafcfb"
  folio-wash: "#e1ecef"
  folio-ink: "#17333d"
  folio-muted: "#405963"
  folio-rule: "#aebfc5"
  folio-rule-soft: "#d1dce0"
  folio-reference: "#126476"
  folio-revision: "#a34436"
  folio-code: "#122e38"
typography:
  display:
    fontFamily: '"Barlow Condensed", "Arial Narrow", sans-serif'
    fontSize: "clamp(3.75rem, 8vw, 6rem)"
    fontWeight: 600
    lineHeight: 0.97
    letterSpacing: "-0.02em"
  headline:
    fontFamily: '"Barlow Condensed", "Arial Narrow", sans-serif'
    fontSize: "clamp(2.5rem, 5.2vw, 4rem)"
    fontWeight: 600
    lineHeight: 0.99
  title:
    fontFamily: '"Barlow Condensed", "Arial Narrow", sans-serif'
    fontSize: "clamp(1.7rem, 3.2vw, 2.25rem)"
    fontWeight: 600
    lineHeight: 1.05
  body:
    fontFamily: '"Atkinson Hyperlegible Next", "Segoe UI", sans-serif'
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.8
  label:
    fontFamily: '"Atkinson Hyperlegible Next", "Segoe UI", sans-serif'
    fontSize: "12px"
    fontWeight: 600
    lineHeight: 1.5
  action:
    fontFamily: '"Atkinson Hyperlegible Next", "Segoe UI", sans-serif'
    fontSize: "14px"
    fontWeight: 600
    lineHeight: 1.5
  code:
    fontFamily: '"SFMono-Regular", Consolas, "Liberation Mono", monospace'
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.8
rounded:
  none: "0px"
spacing:
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  2xl: "48px"
  3xl: "64px"
components:
  button-primary:
    backgroundColor: "{colors.folio-ink}"
    textColor: "{colors.folio-sheet}"
    rounded: "{rounded.none}"
    padding: "12px 20px"
    height: "48px"
    typography: "{typography.action}"
  button-outline:
    backgroundColor: "transparent"
    textColor: "{colors.folio-ink}"
    rounded: "{rounded.none}"
    padding: "8px 12px"
    height: "36px"
    typography: "{typography.action}"
  docs-search:
    backgroundColor: "{colors.folio-sheet}"
    textColor: "{colors.folio-ink}"
    rounded: "{rounded.none}"
    padding: "8px 10px"
    height: "38px"
  folio-record:
    backgroundColor: "{colors.folio-sheet}"
    textColor: "{colors.folio-ink}"
    rounded: "{rounded.none}"
    padding: "24px 36px"
  code-panel:
    backgroundColor: "{colors.folio-code}"
    textColor: "{colors.folio-sheet}"
    rounded: "{rounded.none}"
    padding: "16px"
---

# Design System: Nines Public Site

## Overview

**Creative North Star: "The RFC Folio"**

The public-facing system treats reliability content as a maintained engineering reference: cool paper, ink-dark typography, restrained citations, revision marks, and examples rendered as inspectable records. Condensed display type supplies editorial hierarchy; a highly legible body face carries long-form reading; monospace is reserved for code, values, and precise metadata.

Depth comes from paper and sheet contrast, ink-dark code surfaces, and fine rules rather than shadows or decorative gradients. This system documents the public landing page and documentation; the logged-in portal retains its incumbent identity.

**Key Characteristics:**

- Cool paper, near-white sheets, deep ink, and restrained reference and revision accents.
- Barlow Condensed headlines paired with Atkinson Hyperlegible Next reading text.
- Square geometry, fine rules, and flat surfaces; no shadow-based elevation.
- Code and measured values remain visibly distinct from prose.

## Colors

The palette is a cool paper-and-ink base with a restrained teal reference color and a terracotta revision mark.

### Primary

- **Folio Ink** (`#17333d`): Primary text, filled actions, strong borders, and the dark header band.
- **Reference Teal** (`#126476`): Links, focus rings, and the principal data line.

### Secondary

- **Revision Terracotta** (`#a34436`): Revision emphasis and the chart's target line; use sparingly so it retains meaning.

### Neutral

- **Cool Paper** (`#edf3f4`): The public site's page canvas.
- **Folio Sheet** (`#fafcfb`): Reading and preview surfaces, and light text on dark ink.
- **Pale Wash** (`#e1ecef`): Secondary surfaces and selected or quoted content.
- **Muted Ink** (`#405963`): Supporting text on light surfaces.
- **Folio Rule** (`#aebfc5`): Structural borders and dividers.
- **Soft Rule** (`#d1dce0`): Low-emphasis separators, including chart guides.
- **Code Ink** (`#122e38`): Code and command panels.

**The Citation Rule.** Teal identifies references, links, focus, and primary data. Terracotta marks revision or target context; do not use either accent as general decoration.

## Typography

**Display Font:** Barlow Condensed (with Arial Narrow, sans-serif fallbacks)  
**Body Font:** Atkinson Hyperlegible Next (with Segoe UI, sans-serif fallbacks)  
**Label/Mono Font:** SFMono-Regular, Consolas, Liberation Mono, monospace

**Character:** Condensed headlines give the page the authority of a technical folio without competing with its content. Atkinson Hyperlegible Next carries explanatory text; monospace is reserved for actual code, measurements, commands, and compact technical labels.

### Hierarchy

- **Display** (600, `clamp(3.75rem, 8vw, 6rem)`, `0.97` line-height): Landing-page promise; keep its measure compact.
- **Headline** (600, `clamp(40px, 5.2vw, 64px)`, `0.99` line-height): Major public-page section titles.
- **Title** (600, `clamp(27px, 3.2vw, 36px)`, `1.05` line-height): Preview records and subordinate section headings.
- **Body** (400, `16px`, `1.8` line-height): Explanatory and documentation copy. Keep reading text near the implemented `65ch` measure; the landing description uses up to `68ch`.
- **Label** (600, `12px`, `1.5` line-height): Controls, navigation, and supporting metadata. Small monospace labels are reserved for compact file and record identifiers.
- **Code** (400, `13px`, `1.8` line-height): Commands and examples; use tabular figures for measured values.

**The Role Integrity Rule.** Use the condensed family for headings, the body family for prose, and monospace only where code or technical notation gives it meaning.

## Layout

Landing content uses a centered container up to `1320px`, with fluid gutters. Large sections use generous vertical spacing; the recurring spacing rhythm is `8`, `12`, `16`, `24`, `32`, `48`, and `64px`. Distinctive content should retain a clear hierarchy instead of being flattened into equivalent containers.

Documentation uses a maximum `1540px` shell with a chapter rail, a central article, and a contents outline. The article targets `65ch`; the outline disappears at `1200px`, the chapter rail collapses into a native disclosure at `860px`, and phone layouts reduce gutters and type size. Landing navigation becomes a horizontally scrollable row at `760px`; the hero record and section layouts stack at narrow widths. Test layouts at `1200`, `1000`, `860`, `760`, and `520px`.

**The Reading Measure Rule.** Preserve a distinct article column and allow secondary navigation to collapse before compressing the reading surface.

## Elevation & Depth

The system is flat at rest and does not use box shadows. Depth comes from the contrast between paper and sheet, dark code panels, sticky navigation, and precise borders. Interactive controls use brief color transitions; reduced-motion preferences remove transitions and animations while retaining visible focus and state.

**The Flat Folio Rule.** Do not add shadow-based elevation; separate surfaces with tone, rules, and clear content hierarchy.

## Shapes

Corners are square (`0px` radius). Use fine, usually `1px`, borders to frame records, code, and navigation. Focus is a clearly visible `3px` teal outline with a `3px` offset, not a rounded glow.

**The Square Edge Rule.** Keep component corners square and borders deliberate; do not soften the folio into rounded-card UI.

## Components

### Buttons

- **Shape:** Square corners (`0px`).
- **Primary:** Ink fill with sheet text; the main landing action is `48px` high with `20px` horizontal padding.
- **Outline:** Transparent fill with an ink border and ink text; shared controls have a minimum `36px` height.
- **Hover / Focus:** Primary fill shifts to reference teal; outline buttons take on the pale wash. Keyboard focus uses the shared `3px` outline.

### Cards / Containers

- **Corner Style:** Square (`0px`).
- **Background:** Paper for the page, sheet for the checkout record, and code ink for examples.
- **Shadow Strategy:** None; see Elevation & Depth.
- **Border:** Use a strong ink frame for the signature record and quieter rules for subdivisions.
- **Internal Padding:** Follow the `16` to `36px` spacing range, reducing gutters on mobile.

### Inputs / Fields

- **Style:** Documentation search uses a `38px` sheet-colored field with a thin rule and square corners.
- **Focus:** Shared teal outline with offset; preserve the visible label even when placeholder text is present.
- **Disabled:** Keep disabled search visually legible and explain its state.

### Navigation

- **Style:** Compact, ink-colored links with clear underlines on hover and a visible current chapter in the documentation rail.
- **Mobile:** Landing links scroll horizontally; documentation chapters use a keyboard-operable disclosure. Hide the contents outline rather than squeezing the article.

### Signature Component: Checkout Record

The workspace preview reads as a labeled engineering record, not live product telemetry. Use the dark project band, synthetic-data label, measured values, restrained chart rules, journey-to-SLO link, and YAML reference as one coherent unit. Keep synthetic status explicit wherever the illustrative values appear.

### Code Panels

Use a code-ink surface, high-contrast text, a subtle toolbar rule, and horizontal scrolling for long lines. Preserve monospace and tabular figures; do not force code to wrap into prose.

**The Evidence Label Rule.** Keep illustrative measurements explicitly labeled as synthetic and keep code or measured values visually distinct from explanatory prose.

## Do's and Don'ts

### Do:

- **Do** reuse the existing Nines logo on public pages.
- **Do** use the folio palette, square edges, fine rules, and the established type roles.
- **Do** preserve visible focus, readable contrast, and a legible article measure across responsive layouts.
- **Do** label synthetic measurements wherever the example is presented.

### Don't:

- **Don't** use rounded-card styling, decorative gradients, or shadow-heavy elevation.
- **Don't** use accent colors as decoration or rely on color alone to communicate status.
- **Don't** extend this public-site visual system into the logged-in portal without explicit approval.
