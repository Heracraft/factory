<!--
  What `repose run` sends, one 10 s loop: three parcels leave the laptop's
  checkout over the one SSH connection (commits as a bundle, changes as a
  diff, untracked files as a tar) and each fills its row on the machine;
  node_modules stays behind and is marked "not sent". Reduced motion shows
  the finished state.
-->
<svg
	viewBox="0 0 640 236"
	class="sync block h-auto w-full"
	role="img"
	aria-labelledby="sync-title"
	font-family="ui-monospace, SFMono-Regular, Menlo, monospace"
>
	<title id="sync-title">
		repose run sends unpushed commits, uncommitted changes and untracked files from the laptop to
		the machine over SSH; node_modules is not sent.
	</title>

	<!-- laptop checkout -->
	<g>
		<rect x="0.5" y="14.5" width="238" height="206" rx="3" class="ink" />
		<line x1="0.5" y1="40.5" x2="238.5" y2="40.5" class="ink" />
		<text x="14" y="32" font-size="10" class="t">laptop</text>
		<text x="226" y="32" font-size="10" class="t dim" text-anchor="end">~/code/izma</text>

		<g font-size="10">
			<g transform="translate(14 62)">
				<circle cx="4" cy="-3.5" r="3" class="ink" />
				<line x1="7" y1="-3.5" x2="13" y2="-3.5" class="ink" />
				<circle cx="16" cy="-3.5" r="3" class="ink" />
				<line x1="19" y1="-3.5" x2="25" y2="-3.5" class="ink" />
				<circle cx="28" cy="-3.5" r="3" class="dotfill" />
				<text x="40" y="0" class="t">3 commits, not pushed</text>
			</g>
			<text x="14" y="94" class="t">src/auth/session.ts</text>
			<text x="226" y="94" class="t dim" text-anchor="end">M</text>
			<text x="14" y="124" class="t">notes.md</text>
			<text x="226" y="124" class="t dim" text-anchor="end">?</text>
			<g class="nm">
				<text x="14" y="154" class="t dim">node_modules/</text>
				<text x="226" y="154" class="t dim" text-anchor="end">2.1 GB</text>
				<line x1="14" y1="150.5" x2="226" y2="150.5" class="strike" />
			</g>
			<text x="14" y="200" class="t dim notsent">not sent: install it on the machine</text>
		</g>
	</g>

	<!-- the connection -->
	<line x1="238.5" y1="117.5" x2="400.5" y2="117.5" class="faint" />
	<text x="319" y="106" font-size="10" class="t dim" text-anchor="middle">ssh</text>
	<g font-size="9">
		<g class="parcel p1">
			<rect x="244.5" y="124.5" width="52" height="18" rx="1" class="box" />
			<text x="270.5" y="137" class="t" text-anchor="middle">bundle</text>
		</g>
		<g class="parcel p2">
			<rect x="244.5" y="124.5" width="52" height="18" rx="1" class="box" />
			<text x="270.5" y="137" class="t" text-anchor="middle">diff</text>
		</g>
		<g class="parcel p3">
			<rect x="244.5" y="124.5" width="52" height="18" rx="1" class="box" />
			<text x="270.5" y="137" class="t" text-anchor="middle">tar</text>
		</g>
	</g>

	<!-- the machine -->
	<g>
		<rect x="400.5" y="14.5" width="238" height="206" rx="3" class="ink" />
		<line x1="400.5" y1="40.5" x2="638.5" y2="40.5" class="ink" />
		<rect x="414" y="23" width="8" height="8" class="run" />
		<text x="430" y="32" font-size="10" class="t">izma</text>
		<text x="626" y="32" font-size="10" class="t dim" text-anchor="end">~/izma</text>

		<g font-size="10">
			<g class="arrive a1" transform="translate(414 62)">
				<circle cx="4" cy="-3.5" r="3" class="ink" />
				<line x1="7" y1="-3.5" x2="13" y2="-3.5" class="ink" />
				<circle cx="16" cy="-3.5" r="3" class="ink" />
				<line x1="19" y1="-3.5" x2="25" y2="-3.5" class="ink" />
				<circle cx="28" cy="-3.5" r="3" class="dotfill" />
				<text x="40" y="0" class="t">main, 3 new commits</text>
			</g>
			<g class="arrive a2">
				<text x="414" y="94" class="t">src/auth/session.ts</text>
				<text x="626" y="94" class="t dim" text-anchor="end">M</text>
			</g>
			<g class="arrive a3">
				<text x="414" y="124" class="t">notes.md</text>
				<text x="626" y="124" class="t dim" text-anchor="end">?</text>
			</g>
			<text x="414" y="200" class="t done">Synced: 1 modified, 1 untracked</text>
		</g>
	</g>
</svg>

<style>
	.sync {
		--T: 10s;
		color: var(--color-zinc-900);
	}
	@media (prefers-color-scheme: dark) {
		.sync {
			color: var(--color-zinc-100);
		}
	}
	.ink {
		fill: none;
		stroke: currentColor;
		stroke-width: 1.25;
	}
	.faint {
		stroke: currentColor;
		stroke-opacity: 0.3;
		stroke-dasharray: 2 4;
	}
	.box {
		fill: var(--surface);
		stroke: currentColor;
		stroke-width: 1.25;
	}
	.dotfill {
		fill: currentColor;
	}
	.t {
		fill: currentColor;
	}
	.dim {
		fill-opacity: 0.5;
	}
	.run {
		fill: var(--color-emerald-500);
	}
	.strike {
		stroke: currentColor;
		stroke-width: 1;
		stroke-dasharray: 212;
		stroke-dashoffset: 212;
		animation: strike var(--T) ease-out infinite;
	}
	@keyframes strike {
		0%,
		62% {
			stroke-dashoffset: 212;
		}
		68%,
		96% {
			stroke-dashoffset: 0;
		}
		100% {
			stroke-dashoffset: 212;
		}
	}

	/* parcels: appear at the laptop, slide to the machine, vanish */
	.parcel {
		opacity: 0;
		animation: var(--T) cubic-bezier(0.45, 0, 0.25, 1) infinite;
	}
	.p1 {
		animation-name: p1;
	}
	.p2 {
		animation-name: p2;
	}
	.p3 {
		animation-name: p3;
	}
	@keyframes p1 {
		0%,
		6% {
			opacity: 0;
			transform: translateX(0);
		}
		8% {
			opacity: 1;
			transform: translateX(0);
		}
		22% {
			opacity: 1;
			transform: translateX(98px);
		}
		24%,
		100% {
			opacity: 0;
			transform: translateX(98px);
		}
	}
	@keyframes p2 {
		0%,
		22% {
			opacity: 0;
			transform: translateX(0);
		}
		24% {
			opacity: 1;
			transform: translateX(0);
		}
		38% {
			opacity: 1;
			transform: translateX(98px);
		}
		40%,
		100% {
			opacity: 0;
			transform: translateX(98px);
		}
	}
	@keyframes p3 {
		0%,
		38% {
			opacity: 0;
			transform: translateX(0);
		}
		40% {
			opacity: 1;
			transform: translateX(0);
		}
		54% {
			opacity: 1;
			transform: translateX(98px);
		}
		56%,
		100% {
			opacity: 0;
			transform: translateX(98px);
		}
	}

	/* rows on the machine appear as their parcel lands */
	.arrive,
	.notsent,
	.done {
		opacity: 0;
		animation: var(--T) ease-out infinite;
	}
	.a1 {
		animation-name: a1;
	}
	.a2 {
		animation-name: a2;
	}
	.a3 {
		animation-name: a3;
	}
	.notsent {
		animation-name: notsent;
	}
	.done {
		animation-name: done;
	}
	@keyframes a1 {
		0%,
		22% {
			opacity: 0;
		}
		25%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes a2 {
		0%,
		38% {
			opacity: 0;
		}
		41%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes a3 {
		0%,
		54% {
			opacity: 0;
		}
		57%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes notsent {
		0%,
		66% {
			opacity: 0;
		}
		70%,
		96% {
			opacity: 0.5;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes done {
		0%,
		74% {
			opacity: 0;
		}
		78%,
		96% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sync * {
			animation-play-state: paused !important;
			animation-delay: calc(var(--T) * -0.9) !important;
		}
	}
</style>
