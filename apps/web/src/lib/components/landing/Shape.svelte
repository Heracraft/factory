<!--
  One tile of the landing's shape language: flat geometric forms in a
  100 x 100 box, plus two textured ones (marble, sphere) drawn with SVG
  noise so no image is fetched. Colours are the pictures' own, by role
  (the --sh-* tokens in routes/layout.css): two greys, ink, the blue
  accent and Claude Code's orange, nothing else. Each shape has one main
  tone (neutral, accent or warm); its other parts stay grey or ink, so a
  group is mostly grey with a spot of colour, the way the pictures are
  (docs/LANDING.md, "Shape language"). The star is Gemini's
  sparkle (marks.ts), so the one form is both a shape and an agent's
  mark. Always decorative, so always aria-hidden.
-->
<script lang="ts" module>
	let next = 0;
	export type Kind =
		| 'diamond'
		| 'pie'
		| 'pill'
		| 'halves'
		| 'asterisk'
		| 'ring'
		| 'star'
		| 'marble'
		| 'arch'
		| 'sphere'
		| 'leaf'
		| 'sun'
		| 'moon'
		| 'burst'
		| 'pinwheel';
	export type Tone = 'neutral' | 'accent' | 'warm';

	// A shape's tone when the page doesn't pick one.
	const TONE: Record<Kind, Tone> = {
		diamond: 'neutral',
		pie: 'accent',
		pill: 'neutral',
		halves: 'neutral',
		asterisk: 'neutral',
		ring: 'warm',
		star: 'accent',
		marble: 'neutral',
		arch: 'neutral',
		sphere: 'warm',
		leaf: 'neutral',
		sun: 'warm',
		moon: 'neutral',
		burst: 'accent',
		pinwheel: 'neutral'
	};
	const MAIN: Record<Tone, string> = {
		neutral: 'var(--sh-grey)',
		accent: 'var(--sh-accent)',
		warm: 'var(--sh-warm)'
	};
</script>

<script lang="ts">
	import { SPARKLE } from '$lib/components/illustrations/marks';

	let { kind, tone, class: klass = '' }: { kind: Kind; tone?: Tone; class?: string } = $props();
	const id = `sh${next++}`;
	let main = $derived(MAIN[tone ?? TONE[kind]]);
</script>

<svg
	viewBox="0 0 100 100"
	class="block h-full w-full overflow-visible {klass}"
	style="--main: {main}"
	aria-hidden="true"
>
	{#if kind === 'diamond'}
		<circle cx="50" cy="50" r="50" fill="var(--sh-light)" />
		<path d="M50 13 L82 50 L18 50 Z" fill="var(--sh-ink)" />
		<path d="M18 50 L82 50 L50 87 Z" fill="var(--main)" />
	{:else if kind === 'pie'}
		<path d="M50 0 A50 50 0 1 0 100 50 L50 50 Z" fill="var(--main)" />
	{:else if kind === 'pill'}
		<path d="M100 6 H48 A44 44 0 0 0 48 94 H100 Z" fill="var(--main)" />
		<path d="M100 30 H48 A20 20 0 0 0 48 70 H100 Z" fill="var(--sh-light)" />
	{:else if kind === 'halves'}
		<path d="M8 46 A42 42 0 0 1 92 46 Z" fill="var(--main)" />
		<path d="M8 100 A42 42 0 0 1 92 100 Z" fill="var(--main)" />
	{:else if kind === 'asterisk'}
		<g stroke="var(--main)" stroke-width="11">
			<path d="M50 0 V100 M0 50 H100 M15 15 L85 85 M85 15 L15 85" />
		</g>
	{:else if kind === 'ring'}
		<circle cx="50" cy="50" r="50" fill="var(--main)" />
		<path d="M50 50 V16 A34 34 0 0 0 16 50 Z" fill="var(--sh-paper)" />
		<path d="M50 50 H84 A34 34 0 0 0 50 16 Z" fill="var(--sh-light)" />
		<path d="M50 50 H16 A34 34 0 0 0 50 84 Z" fill="var(--sh-ink)" />
		<path d="M50 50 V84 A34 34 0 0 0 84 50 Z" fill="var(--sh-accent)" />
	{:else if kind === 'star'}
		<path d={SPARKLE} transform="translate(-2 -2) scale(4.3333)" fill="var(--main)" />
	{:else if kind === 'marble'}
		<defs>
			<!-- Noise folded into bands reads as the veins of grey marble. -->
			<filter id="{id}f" x="0" y="0" width="100%" height="100%" color-interpolation-filters="sRGB">
				<feTurbulence type="fractalNoise" baseFrequency="0.022 0.045" numOctaves="4" seed="11" />
				<feColorMatrix values="1.4 0 0 0 -0.2  1.4 0 0 0 -0.2  1.4 0 0 0 -0.2  0 0 0 0 1" />
				<feComponentTransfer>
					<feFuncR type="table" tableValues="0.28 0.8 0.46 0.86 0.36 0.78 0.3" />
					<feFuncG type="table" tableValues="0.28 0.8 0.46 0.86 0.36 0.78 0.3" />
					<feFuncB type="table" tableValues="0.28 0.79 0.45 0.84 0.36 0.77 0.3" />
				</feComponentTransfer>
			</filter>
			<clipPath id="{id}c"><circle cx="50" cy="50" r="50" /></clipPath>
		</defs>
		<rect width="100" height="100" filter="url(#{id}f)" clip-path="url(#{id}c)" />
	{:else if kind === 'arch'}
		<path d="M8 100 V44 A42 42 0 0 1 92 44 V100 Z" fill="var(--main)" />
		<path d="M29 100 V46 A21 21 0 0 1 71 46 V100 Z" fill="var(--sh-ink)" />
	{:else if kind === 'sphere'}
		<defs>
			<radialGradient id="{id}g" cx="0.34" cy="0.3" r="0.8">
				<stop offset="0" style="stop-color: color-mix(in oklab, var(--main) 55%, white)" />
				<stop offset="0.45" style="stop-color: var(--main)" />
				<stop offset="1" style="stop-color: color-mix(in oklab, var(--main) 75%, black)" />
			</radialGradient>
			<filter id="{id}n" x="0" y="0" width="100%" height="100%">
				<feTurbulence type="fractalNoise" baseFrequency="1.1" numOctaves="2" seed="3" />
				<feColorMatrix values="0 0 0 0 1  0 0 0 0 0.85  0 0 0 0 0.8  0 0 0 -1.6 1.05" />
				<feComposite in2="SourceGraphic" operator="in" />
			</filter>
		</defs>
		<circle cx="50" cy="50" r="50" fill="url(#{id}g)" />
		<circle cx="50" cy="50" r="50" fill="#000" filter="url(#{id}n)" opacity="0.28" />
	{:else if kind === 'leaf'}
		<path d="M100 0 V100 H0 A100 100 0 0 1 100 0 Z" fill="var(--main)" />
	{:else if kind === 'sun'}
		<circle cx="50" cy="50" r="50" fill="var(--main)" />
	{:else if kind === 'moon'}
		<path d="M72 4 A50 50 0 1 0 72 96 A40 40 0 1 1 72 4 Z" fill="var(--main)" />
	{:else if kind === 'burst'}
		<g stroke="var(--main)" stroke-width="6">
			{#each Array.from({ length: 12 }, (_, i) => i * 15) as a (a)}
				<path d="M0 50 H100" transform="rotate({a} 50 50)" />
			{/each}
		</g>
		<circle cx="50" cy="50" r="15" fill="var(--main)" />
	{:else if kind === 'pinwheel'}
		<path d="M50 50 V0 A50 50 0 0 0 0 50 Z" fill="var(--main)" />
		<path d="M50 50 V100 A50 50 0 0 0 100 50 Z" fill="var(--sh-light)" />
	{/if}
</svg>
