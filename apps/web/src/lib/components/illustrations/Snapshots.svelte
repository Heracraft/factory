<!--
  A project's life on one time axis, one 12 s loop drawn by a playhead:
  running, stopped (a snapshot), running again, destroyed (a final
  snapshot, kept 30 days), restored from that snapshot. Reduced motion
  shows the whole story at once.
-->
<svg
	viewBox="0 0 640 190"
	class="snaps block h-auto w-full"
	role="img"
	aria-labelledby="snaps-title"
	font-family="ui-monospace, SFMono-Regular, Menlo, monospace"
>
	<title id="snaps-title">
		A project runs, is stopped with a snapshot, runs again, is destroyed with a final snapshot kept
		for 30 days, and is restored from that snapshot.
	</title>

	<!-- row labels -->
	<g font-size="10" class="t dim">
		<text x="0" y="54">machine</text>
		<text x="0" y="114">snapshots</text>
	</g>

	<!-- axis -->
	<line x1="84" y1="160.5" x2="636" y2="160.5" class="ink-faint" />
	<g font-size="9" class="t dim" text-anchor="middle">
		<text x="100" y="178">day 1</text>
		<text x="228" y="178">day 3</text>
		<text x="380" y="178">day 9</text>
		<text x="600" y="178">day 24</text>
	</g>

	<!-- machine bars: grow left to right as the playhead passes -->
	<rect x="100" y="44" width="92" height="12" class="bar b1" />
	<line x1="192" y1="50.5" x2="228" y2="50.5" class="gap g1" />
	<rect x="228" y="44" width="152" height="12" class="bar b2" />
	<rect x="600" y="44" width="36" height="12" class="bar b3" />

	<!-- events on the machine row -->
	<g font-size="9" class="t">
		<text x="100" y="36" class="ev e0">run</text>
		<text x="192" y="36" class="ev e1" text-anchor="middle">stop</text>
		<text x="228" y="36" class="ev e2">start</text>
		<text x="380" y="36" class="ev e3" text-anchor="middle">destroy</text>
		<text x="636" y="36" class="ev e5" text-anchor="end">restore izma</text>
	</g>
	<g class="ev e3">
		<line x1="374" y1="44" x2="386" y2="56" class="ink" />
		<line x1="386" y1="44" x2="374" y2="56" class="ink" />
	</g>

	<!-- snapshots -->
	<rect x="187" y="104" width="10" height="10" transform="rotate(45 192 109)" class="snap s1" />
	<rect x="375" y="104" width="10" height="10" transform="rotate(45 380 109)" class="snap s3" />

	<!-- 30 day retention bracket after destroy -->
	<g class="keep">
		<path d="M392 109.5 H628 M628 103 V116" class="ink" />
		<text x="510" y="132" font-size="9" class="t dim" text-anchor="middle">kept 30 days</text>
	</g>

	<!-- restore: from the final snapshot up to the new machine -->
	<path d="M386 102 C470 72 560 72 596 58" class="restore" />

	<!-- playhead -->
	<g class="head">
		<line x1="100" y1="22" x2="100" y2="160" class="headline" />
	</g>
</svg>

<style>
	.snaps {
		--T: 12s;
		color: var(--color-zinc-900);
	}
	@media (prefers-color-scheme: dark) {
		.snaps {
			color: var(--color-zinc-100);
		}
	}
	.t {
		fill: currentColor;
	}
	.dim {
		fill-opacity: 0.5;
	}
	.ink {
		fill: none;
		stroke: currentColor;
		stroke-width: 1.25;
	}
	.ink-faint {
		stroke: currentColor;
		stroke-opacity: 0.25;
	}
	.bar {
		fill: var(--color-emerald-500);
		transform-box: fill-box;
		transform-origin: 0 50%;
		animation: var(--T) linear infinite;
	}
	.gap {
		stroke: currentColor;
		stroke-opacity: 0.35;
		stroke-dasharray: 2 3;
		opacity: 0;
		animation: var(--T) steps(1) infinite;
	}
	.snap {
		fill: var(--surface);
		stroke: currentColor;
		stroke-width: 1.25;
		opacity: 0;
		animation: var(--T) ease-out infinite;
	}
	.ev,
	.keep {
		opacity: 0;
		animation: var(--T) ease-out infinite;
	}
	.restore {
		fill: none;
		stroke: currentColor;
		stroke-width: 1.25;
		stroke-dasharray: 240;
		stroke-dashoffset: 240;
		animation: restore var(--T) ease-in-out infinite;
	}
	.headline {
		stroke: var(--color-blue-600);
		stroke-width: 1;
	}
	@media (prefers-color-scheme: dark) {
		.headline {
			stroke: var(--color-blue-300);
		}
	}
	/* the playhead crosses 100→636 between 4% and 88%; each element's
	   moment below is where the head reaches its x */
	.head {
		animation: head var(--T) linear infinite;
	}
	@keyframes head {
		0%,
		4% {
			transform: translateX(0);
			opacity: 1;
		}
		88% {
			transform: translateX(536px);
			opacity: 1;
		}
		92%,
		100% {
			transform: translateX(536px);
			opacity: 0;
		}
	}
	/* x → % : 4 + (x - 100) / 536 * 84 */
	.b1 {
		animation-name: b1;
	}
	@keyframes b1 {
		0%,
		4% {
			transform: scaleX(0);
		}
		18.4% {
			transform: scaleX(1);
		}
		96% {
			transform: scaleX(1);
			opacity: 1;
		}
		100% {
			transform: scaleX(1);
			opacity: 0;
		}
	}
	.g1 {
		animation-name: g1;
	}
	@keyframes g1 {
		0% {
			opacity: 0;
		}
		18.4% {
			opacity: 1;
		}
		96% {
			opacity: 0;
		}
	}
	.b2 {
		animation-name: b2;
	}
	@keyframes b2 {
		0%,
		24.1% {
			transform: scaleX(0);
		}
		47.9% {
			transform: scaleX(1);
		}
		96% {
			transform: scaleX(1);
			opacity: 1;
		}
		100% {
			transform: scaleX(1);
			opacity: 0;
		}
	}
	.b3 {
		animation-name: b3;
	}
	@keyframes b3 {
		0%,
		82.4% {
			transform: scaleX(0);
		}
		88% {
			transform: scaleX(1);
		}
		96% {
			transform: scaleX(1);
			opacity: 1;
		}
		100% {
			transform: scaleX(1);
			opacity: 0;
		}
	}
	.e0 {
		animation-name: e0;
	}
	.e1,
	.s1 {
		animation-name: e1;
	}
	.e2 {
		animation-name: e2;
	}
	.e3,
	.s3 {
		animation-name: e3;
	}
	.keep {
		animation-name: keep;
	}
	.e5 {
		animation-name: e5;
	}
	@keyframes e0 {
		0%,
		3% {
			opacity: 0;
		}
		5%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes e1 {
		0%,
		18% {
			opacity: 0;
		}
		20%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes e2 {
		0%,
		23.8% {
			opacity: 0;
		}
		25.5%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes e3 {
		0%,
		47.8% {
			opacity: 0;
		}
		49.5%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes keep {
		0%,
		50% {
			opacity: 0;
		}
		56%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes restore {
		0%,
		72% {
			stroke-dashoffset: 240;
			opacity: 1;
		}
		82% {
			stroke-dashoffset: 0;
			opacity: 1;
		}
		96% {
			stroke-dashoffset: 0;
			opacity: 1;
		}
		100% {
			stroke-dashoffset: 0;
			opacity: 0;
		}
	}
	@keyframes e5 {
		0%,
		81% {
			opacity: 0;
		}
		84%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.snaps * {
			animation-play-state: paused !important;
			animation-delay: calc(var(--T) * -0.9) !important;
		}
		.head {
			display: none;
		}
	}
</style>
