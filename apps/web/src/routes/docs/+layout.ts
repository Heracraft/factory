// The docs are prerendered into the served HTML (DECISIONS I-344): a reload
// paints the page before any script runs, so the browser puts you back where
// you were, and a crawler or curl reads the text. The rest of the app stays a
// client-rendered SPA (../+layout.ts); the header's sign-in state fills in
// once the page is running, as on the legal pages.
export const ssr = true;
export const prerender = true;
