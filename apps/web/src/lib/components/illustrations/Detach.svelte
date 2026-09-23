<!--
  The hero drawing, one 14 s loop: the laptop is attached to the project's
  machine; the lid closes and the link goes idle while the machine keeps
  working through the night; the lid opens and `repose attach` picks the
  session up again. Line art in currentColor, one accent for "running".
  Reduced motion shows the middle of the night, lid closed, work going on.
-->
<svg
	viewBox="0 0 640 300"
	class="detach block h-auto w-full"
	role="img"
	aria-labelledby="detach-title"
	font-family="ui-monospace, SFMono-Regular, Menlo, monospace"
>
	<title id="detach-title">
		A laptop attached to a remote machine closes its lid; the machine keeps running the agent
		overnight, and the laptop attaches again in the morning.
	</title>

	<!-- ground line -->
	<line x1="0" y1="252.5" x2="640" y2="252.5" class="faint" />

	<!-- ============ laptop ============ -->
	<g class="laptop">
		<g class="lid">
			<rect x="30.5" y="96.5" width="200" height="130" rx="3" class="ink" />
			<rect x="40.5" y="106.5" width="180" height="110" rx="1" class="faint" />
			<g font-size="9" class="screen">
				<text x="50" y="126" class="t-dim">~/code/izma</text>
				<g class="cmd-run">
					<text x="50" y="142"><tspan class="t-acc">❯</tspan> repose run</text>
					<text x="50" y="158" class="t-dim">Ready in 12s.</text>
				</g>
				<g class="cmd-attach">
					<text x="50" y="142"><tspan class="t-acc">❯</tspan> repose attach izma</text>
					<text x="50" y="158" class="t-dim">[izma] 0:claude*</text>
				</g>
				<rect x="50" y="166" width="5.5" height="9" class="caret" />
			</g>
		</g>
		<path
			d="M18.5 230.5 H242.5 L252.5 244.5 Q253 248.5 249 248.5 H12 Q8 248.5 8.5 244.5 Z"
			class="ink"
		/>
		<line x1="112" y1="236.5" x2="150" y2="236.5" class="faint" />
	</g>

	<!-- ============ the link ============ -->
	<path d="M252 238 C318 238 318 160 380 160" class="link-idle" />
	<path d="M252 238 C318 238 318 160 380 160" class="link-live" />
	<g font-size="10" class="t-dim" text-anchor="middle">
		<text x="316" y="276" class="lbl lbl-a">attached</text>
		<text x="316" y="276" class="lbl lbl-b">detached, still running</text>
		<text x="316" y="276" class="lbl lbl-c">attached again</text>
	</g>

	<!-- ============ the machine ============ -->
	<g class="machine">
		<rect x="380.5" y="40.5" width="236" height="212" rx="3" class="ink" />
		<line x1="380.5" y1="66.5" x2="616.5" y2="66.5" class="ink" />
		<rect x="392" y="50" width="8" height="8" class="run" />
		<text x="408" y="58" font-size="10" class="t-ink">izma</text>
		<text x="438" y="58" font-size="10" class="t-dim">large</text>
		<g font-size="10" class="t-dim" text-anchor="end">
			<text x="606" y="58" class="clk c1">18:40</text>
			<text x="606" y="58" class="clk c2">18:41</text>
			<text x="606" y="58" class="clk c3">23:58</text>
			<text x="606" y="58" class="clk c4">02:13</text>
			<text x="606" y="58" class="clk c5">08:05</text>
		</g>

		<g font-size="9">
			<text x="394" y="86" class="ln l1"><tspan class="t-acc">claude</tspan> reading src/auth/</text
			>
			<text x="394" y="102" class="ln l2"
				><tspan class="t-acc">claude</tspan> editing session.ts</text
			>
			<text x="394" y="118" class="ln l3"
				><tspan class="t-acc">claude</tspan> editing tokens.ts</text
			>
			<text x="394" y="134" class="ln l4">$ pnpm test <tspan class="t-dim">· 9 failed</tspan></text>
			<text x="394" y="150" class="ln l5"
				><tspan class="t-acc">claude</tspan> fixing refresh flow</text
			>
			<text x="394" y="166" class="ln l6">$ pnpm test <tspan class="t-dim">· 2 failed</tspan></text>
			<text x="394" y="182" class="ln l7"
				><tspan class="t-acc">claude</tspan> fixing clock skew</text
			>
			<text x="394" y="198" class="ln l8">$ pnpm test <tspan class="t-ok">· 142 passed</tspan></text
			>
			<text x="394" y="214" class="ln l9"><tspan class="t-acc">claude</tspan> waiting for you</text>
		</g>

		<line x1="380.5" y1="228.5" x2="616.5" y2="228.5" class="faint" />
		<text x="394" y="243" font-size="9" class="t-dim">[izma] 0:claude*</text>
		<text x="604" y="243" font-size="9" class="t-dim" text-anchor="end">tmux</text>
	</g>
</svg>

<style>
	.detach {
		--T: 14s;
		color: var(--color-zinc-900);
		overflow: visible;
	}
	:global(.dark) .detach,
	.detach {
		color-scheme: inherit;
	}
	@media (prefers-color-scheme: dark) {
		.detach {
			color: var(--color-zinc-100);
		}
	}
	.ink {
		fill: none;
		stroke: currentColor;
		stroke-width: 1.25;
	}
	.faint {
		fill: none;
		stroke: currentColor;
		stroke-opacity: 0.22;
		stroke-width: 1;
	}
	text {
		fill: currentColor;
	}
	.t-ink {
		fill: currentColor;
	}
	.t-dim,
	.t-dim text {
		fill: currentColor;
		fill-opacity: 0.5;
	}
	.t-acc {
		fill: var(--color-blue-600);
	}
	.t-ok {
		fill: var(--color-emerald-600);
		fill-opacity: 1;
	}
	@media (prefers-color-scheme: dark) {
		.t-acc {
			fill: var(--color-blue-300);
		}
		.t-ok {
			fill: var(--color-emerald-400);
		}
	}
	.run {
		fill: var(--color-emerald-500);
	}
	.caret {
		fill: currentColor;
		fill-opacity: 0.7;
	}

	/* ---- the link ---- */
	.link-idle {
		fill: none;
		stroke: currentColor;
		stroke-opacity: 0.25;
		stroke-width: 1;
		stroke-dasharray: 2 5;
	}
	.link-live {
		fill: none;
		stroke: currentColor;
		stroke-width: 1.5;
		stroke-dasharray: 10 8;
		animation:
			flow 1.2s linear infinite,
			live var(--T) linear infinite;
	}
	@keyframes flow {
		to {
			stroke-dashoffset: -36;
		}
	}
	@keyframes live {
		0%,
		13% {
			opacity: 1;
		}
		16%,
		76% {
			opacity: 0;
		}
		80%,
		100% {
			opacity: 1;
		}
	}

	/* ---- the lid folds down onto the base and comes back ---- */
	.lid {
		transform-box: fill-box;
		transform-origin: 50% 100%;
		animation: lid var(--T) cubic-bezier(0.55, 0, 0.3, 1) infinite;
	}
	@keyframes lid {
		0%,
		13% {
			transform: scaleY(1);
		}
		19%,
		72% {
			transform: scaleY(0.03);
		}
		78%,
		100% {
			transform: scaleY(1);
		}
	}
	.cmd-run {
		animation: show-run var(--T) steps(1) infinite;
	}
	.cmd-attach {
		animation: show-attach var(--T) steps(1) infinite;
	}
	@keyframes show-run {
		0% {
			opacity: 1;
		}
		50% {
			opacity: 0;
		}
	}
	@keyframes show-attach {
		0% {
			opacity: 0;
		}
		50% {
			opacity: 1;
		}
	}
	.caret {
		animation: blink 1s steps(1) infinite;
	}
	@keyframes blink {
		50% {
			fill-opacity: 0;
		}
	}

	/* ---- labels and clock swap in place ---- */
	.lbl,
	.clk {
		opacity: 0;
		animation: var(--T) steps(1) infinite;
	}
	.lbl-a {
		animation-name: lbl-a;
	}
	.lbl-b {
		animation-name: lbl-b;
	}
	.lbl-c {
		animation-name: lbl-c;
	}
	@keyframes lbl-a {
		0% {
			opacity: 1;
		}
		15% {
			opacity: 0;
		}
	}
	@keyframes lbl-b {
		0% {
			opacity: 0;
		}
		19% {
			opacity: 1;
		}
		76% {
			opacity: 0;
		}
	}
	@keyframes lbl-c {
		0% {
			opacity: 0;
		}
		79% {
			opacity: 1;
		}
	}
	.c1 {
		animation-name: c1;
	}
	.c2 {
		animation-name: c2;
	}
	.c3 {
		animation-name: c3;
	}
	.c4 {
		animation-name: c4;
	}
	.c5 {
		animation-name: c5;
	}
	@keyframes c1 {
		0% {
			opacity: 1;
		}
		16% {
			opacity: 0;
		}
	}
	@keyframes c2 {
		0% {
			opacity: 0;
		}
		16% {
			opacity: 1;
		}
		32% {
			opacity: 0;
		}
	}
	@keyframes c3 {
		0% {
			opacity: 0;
		}
		32% {
			opacity: 1;
		}
		52% {
			opacity: 0;
		}
	}
	@keyframes c4 {
		0% {
			opacity: 0;
		}
		52% {
			opacity: 1;
		}
		74% {
			opacity: 0;
		}
	}
	@keyframes c5 {
		0% {
			opacity: 0;
		}
		74% {
			opacity: 1;
		}
	}

	/* ---- the machine keeps writing, lid open or not ---- */
	.ln {
		opacity: 0;
		animation: var(--T) linear infinite;
	}
	.l1 {
		animation-name: l1;
	}
	.l2 {
		animation-name: l2;
	}
	.l3 {
		animation-name: l3;
	}
	.l4 {
		animation-name: l4;
	}
	.l5 {
		animation-name: l5;
	}
	.l6 {
		animation-name: l6;
	}
	.l7 {
		animation-name: l7;
	}
	.l8 {
		animation-name: l8;
	}
	.l9 {
		animation-name: l9;
	}
	/* each line: hidden until its moment, fades in over ~1%, stays to the end */
	@keyframes l1 {
		0%,
		3% {
			opacity: 0;
		}
		4%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l2 {
		0%,
		9% {
			opacity: 0;
		}
		10%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l3 {
		0%,
		22% {
			opacity: 0;
		}
		23%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l4 {
		0%,
		29% {
			opacity: 0;
		}
		30%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l5 {
		0%,
		36% {
			opacity: 0;
		}
		37%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l6 {
		0%,
		44% {
			opacity: 0;
		}
		45%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l7 {
		0%,
		51% {
			opacity: 0;
		}
		52%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l8 {
		0%,
		59% {
			opacity: 0;
		}
		60%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}
	@keyframes l9 {
		0%,
		65% {
			opacity: 0;
		}
		66%,
		97% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.detach * {
			animation-play-state: paused !important;
			animation-delay: calc(var(--T) * -0.62) !important;
		}
		.link-live {
			animation: none;
			opacity: 0;
		}
	}
</style>
