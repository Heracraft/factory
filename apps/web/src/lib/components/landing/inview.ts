/** Whether the visitor asked for less motion; false during SSR. */
export const reducedMotion = (): boolean =>
	typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
