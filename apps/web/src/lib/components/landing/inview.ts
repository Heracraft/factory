/** Whether the visitor asked for less motion; false during SSR. */
export const reducedMotion = (): boolean =>
	typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

/**
 * Marks a group of shapes as landed (data-land="in") the first time it
 * comes into view; `.land` children in layout.css then land one after
 * another, each by its own --d delay.
 */
export function landOnView(node: HTMLElement) {
	node.dataset.land = 'out';
	if (reducedMotion()) {
		node.dataset.land = 'in';
		return {};
	}
	const io = new IntersectionObserver(
		([e]) => {
			if (!e.isIntersecting) return;
			node.dataset.land = 'in';
			io.disconnect();
		},
		{ threshold: 0.15 }
	);
	io.observe(node);
	return { destroy: () => io.disconnect() };
}
