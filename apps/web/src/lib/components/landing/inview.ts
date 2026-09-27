/**
 * Calls `onChange(true)` when the node scrolls into view and `onChange(false)`
 * when it leaves, so the landing's demos only play while someone can see them.
 */
export function inview(node: HTMLElement, onChange: (visible: boolean) => void) {
	const io = new IntersectionObserver(([e]) => onChange(e.isIntersecting), { threshold: 0.35 });
	io.observe(node);
	return { destroy: () => io.disconnect() };
}

export const reducedMotion = (): boolean =>
	typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

/** A cancellable sleep for scripted sequences. */
export function sleeper() {
	let alive = true;
	return {
		get alive() {
			return alive;
		},
		wait: (ms: number) =>
			new Promise<void>((res, rej) =>
				setTimeout(() => (alive ? res() : rej(new Error('stopped'))), ms)
			),
		stop: () => {
			alive = false;
		}
	};
}
