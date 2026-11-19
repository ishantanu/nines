// Beautiful UI Notchset v1.0.0 (MIT), Copyright (c) 2026 Chimpy.
// Source: https://beautiful-ui.dev/r/radix-nova/sign-in.json
// Notice: /beautiful-ui-license.txt. Nines changes: account-form slot, optional
// passwordless/signup controls, accessible heading, scoped fonts and no sound.
/*
 * Notchset's root attributes, server-safe (no client directive), so server components can spread
 * them too. Every Notchset root (and portalled popup) carries data-notchset (the motion policy's
 * scope), plays its own cues in the mechanical voice, and keeps the page's click layer out.
 */
export const NOTCHSET_ROOT = {
  "data-notchset": "",
  "data-sound": "none",
  "data-sound-voice": "analogue",
} as const;

/** On every interactive element: it plays its own cues, so the page's click layer adds no tap. */
export const OWN_SOUND = { "data-sound": "none" } as const;
