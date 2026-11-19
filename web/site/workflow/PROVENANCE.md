# Workflow illustration provenance

- Original Nines composition using the existing shadcn/ui Tabs and public-site palette.
- Retained-text motion adapted from Stageshow `kit/motion/Swap.tsx`, `Swap.css` and `kit/reduced-motion.ts`, benjitaylor/skills commit `802057409787133703c56001080169e347305572` (MIT, Benji Taylor). License retained alongside the source.
- Changes: local CSS scope, hidden outgoing text for assistive technology, no initial entry fade, 220 ms user-triggered transitions. No Stage runtime, film dependency or animation package is shipped.
- Layout reference: Stageshow choreography and static-card gold examples. Existing Nines components and RFC Folio surface take precedence over the kit's example chrome.
