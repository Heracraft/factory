// The two legal pages are prerendered into the served HTML so that a
// crawler, a curl or a browser with scripts off reads the policy text,
// and the release checklist's "published" row can be checked with grep
// (docs/security/review-2026-09-21.md M5-7). Everything else in the app
// stays a client-rendered SPA (../+layout.ts).
export const ssr = true;
export const prerender = true;
