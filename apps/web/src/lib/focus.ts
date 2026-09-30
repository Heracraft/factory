// Keyboard focus for the panels that open in place of the button that
// asked for them (a two-step delete, a restore panel). When a click swaps
// the button out of the page, the browser drops focus to <body>, and a
// keyboard user starts again from the top (WCAG 2.4.3). These two helpers
// put focus where the person was: into the panel when it opens, and back on
// the button when it closes.
import { tick } from 'svelte';

/** A Svelte action: focus the element once it is on the page. */
export function focusOnMount(node: HTMLElement, enabled: boolean = true): void {
	if (enabled) node.focus();
}

/**
 * Focus the first element with one of these ids once the next render has
 * run, so a button that an {#if} brings back is there to take it. The
 * first id that exists wins, which lets a caller name a fallback for a row
 * that has gone.
 */
export async function focusAfterRender(...ids: string[]): Promise<void> {
	await tick();
	for (const id of ids) {
		const el = document.getElementById(id);
		if (el) {
			el.focus();
			return;
		}
	}
}
