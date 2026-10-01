// A box that scrolls sideways (a command block on a phone) is one a
// keyboard user cannot read to its end unless it takes focus (axe
// scrollable-region-focusable; only Chromium focuses one on its own).
// DocPage marks the docs' blocks the same way.

/**
 * A Svelte action: the box takes a tab stop while it is wider than its
 * content box and gives it up when it fits, checked again on resize, so
 * where the text fits Tab skips it.
 */
export function tabStopWhenScrolls(node: HTMLElement) {
	const mark = () => {
		if (node.scrollWidth > node.clientWidth) node.tabIndex = 0;
		else node.removeAttribute('tabindex');
	};
	mark();
	const ro = new ResizeObserver(mark);
	ro.observe(node);
	return {
		destroy() {
			ro.disconnect();
		}
	};
}
