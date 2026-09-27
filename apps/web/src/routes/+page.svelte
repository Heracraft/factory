<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { signIn } from '$lib/auth.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Hero from '$lib/components/landing/Hero.svelte';
	import Perch, { type Beat } from '$lib/components/landing/Perch.svelte';
	import Shape, { type Kind } from '$lib/components/landing/Shape.svelte';
	import SectionHead from '$lib/components/landing/SectionHead.svelte';
	import { landOnView } from '$lib/components/landing/inview';
	import OneCommand from '$lib/components/landing/OneCommand.svelte';
	import ComesBack from '$lib/components/landing/ComesBack.svelte';
	import Browser from '$lib/components/landing/Browser.svelte';
	import Localhost from '$lib/components/landing/Localhost.svelte';
	import Ready from '$lib/components/landing/Ready.svelte';
	import Editor from '$lib/components/landing/Editor.svelte';

	const INSTALL_COMMAND = 'curl -fsSL https://repose.herakraft.co/install.sh | sh';
	const SOURCE_URL = 'https://github.com/Heracraft/factory';

	let signingIn = $state(false);
	let beat: { kind: Beat; n: number } | undefined = $state();
	let beats = 0;
	let copied = $state(false);

	onMount(() => {
		// Logto sends the user back here (not /callback) when sign-in itself
		// was cancelled or failed before a code was issued.
		const params = new URLSearchParams(location.search);
		if (params.get('error')) {
			toast.error('Sign-in was cancelled or failed; try again.');
			history.replaceState(null, '', location.pathname);
		}
	});

	async function onSignIn() {
		signingIn = true;
		try {
			await signIn();
		} catch {
			signingIn = false;
			toast.error('Sign-in was cancelled or failed; try again.');
		}
	}

	async function copyInstall() {
		try {
			await navigator.clipboard.writeText(INSTALL_COMMAND);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			toast.error('Could not copy. Select the command instead.');
		}
	}

	const steps = [
		{
			title: 'Install the CLI',
			text: 'One binary for macOS and Linux.',
			command: INSTALL_COMMAND
		},
		{
			title: 'Sign in',
			text: 'GitHub sign-in opens in your browser.',
			command: 'repose login'
		},
		{
			title: 'Run in any checkout',
			text: 'The first run creates the machine, syncs your work and attaches you to it.',
			command: 'cd ~/code/recruiting && repose run'
		}
	];
	// The steps' marks: one shape each, the last the repose ring itself.
	const stepShapes: Kind[] = ['sphere', 'pinwheel', 'ring'];
	const tiers: {
		name: string;
		vcpu: number;
		ram: string;
		disk: string;
		hour: string;
		cap: string;
		shape: Kind;
	}[] = [
		{ name: 'small', vcpu: 2, ram: '4 GB', disk: '20 GB', hour: '$0.07', cap: '$49', shape: 'sun' },
		{
			name: 'large',
			vcpu: 4,
			ram: '8 GB',
			disk: '40 GB',
			hour: '$0.14',
			cap: '$99',
			shape: 'halves'
		},
		{ name: 'xl', vcpu: 8, ram: '16 GB', disk: '80 GB', hour: '$0.28', cap: '$199', shape: 'burst' }
	];
	const frieze: Kind[] = [
		'sun',
		'moon',
		'asterisk',
		'pinwheel',
		'arch',
		'ring',
		'pie',
		'star',
		'halves',
		'leaf',
		'sphere',
		'pill'
	];
	let friezeTurn = $state(frieze.map(() => 0));
</script>

<svelte:head>
	<title>repose: let your agents run with full permissions</title>
	<meta
		name="description"
		content="A disposable dev machine per project with your code, tools and secrets on it in 15 seconds, so coding agents can run with full permissions and your laptop stays out of reach."
	/>
</svelte:head>

{#snippet sample(lines: string[])}
	<pre class="codeblock !text-[12.5px] !leading-6">{#each lines as line, i (i)}<span
				class="block min-h-6"
				>{#if line.startsWith('$ ')}<span class="text-zinc-600 select-none dark:text-zinc-400"
						>$ </span>{line.slice(2)}{:else}<span class="text-zinc-600 dark:text-zinc-400"
						>{line}</span
					>{/if}</span
			>{/each}</pre>
{/snippet}

<header class="border-b border-[var(--rule)]">
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between gap-4 px-5">
		<a href={resolve('/')} aria-label="repose, home"><Logo mark /></a>
		<nav class="flex items-center gap-6 text-sm" aria-label="Main">
			<a
				href={resolve('/docs')}
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>Docs</a
			>
			<a
				href="#pricing"
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>Pricing</a
			>
			<a
				href={SOURCE_URL}
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>GitHub</a
			>
			<button type="button" class="btn-quiet !py-1.5" disabled={signingIn} onclick={onSignIn}
				>Sign in</button
			>
		</nav>
	</div>
</header>

<main>
	<section class="mx-auto max-w-5xl px-5 pt-14 pb-16">
		<div class="min-w-0">
			<h1 class="hero-h">
				<span class="line">Let your agents run</span>
				<span class="line">with <span class="bar">full permissions</span></span>
			</h1>
			<p class="mt-6 max-w-xl text-lg leading-relaxed text-zinc-600 dark:text-zinc-400">
				One command puts your work on a machine of its own. The agent can wreck it, and a snapshot
				puts it back.
			</p>
			<div class="mt-8 flex flex-wrap items-center gap-3">
				<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
					Sign in with GitHub
				</button>
				<div
					class="flex max-w-full min-w-0 items-center gap-2 rounded-sm border border-[var(--rule-strong)] bg-[var(--sunken)] py-1.5 pr-1.5 pl-3.5 font-mono text-[13px]"
				>
					<span class="truncate select-all">{INSTALL_COMMAND}</span>
					<button
						type="button"
						onclick={copyInstall}
						class="inline-flex shrink-0 cursor-pointer items-center gap-1.5 rounded-sm border border-[var(--rule-strong)] bg-[var(--surface)] px-2 py-1 font-sans text-xs text-zinc-700 hover:border-zinc-500 dark:text-zinc-300"
						aria-label="Copy the install command"
					>
						<svg
							viewBox="0 0 16 16"
							class="h-3.5 w-3.5"
							fill="none"
							stroke="currentColor"
							stroke-width="1.4"
							aria-hidden="true"
							><rect x="5.5" y="5.5" width="8" height="8" rx="1" /><path
								d="M10.5 3.5v-1h-8v8h1"
							/></svg
						>
						{copied ? 'Copied' : 'Copy'}
					</button>
				</div>
			</div>
		</div>
		<div class="landing-stage mt-20">
			<Perch large shapes={['diamond', 'pie', 'agent', 'halves', 'ring', 'leaf']} {beat} />
			<Hero onbeat={(kind) => (beat = { kind, n: ++beats })} />
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<SectionHead title="Your working state, in one command">
				Run <code
					class="rounded-xs bg-[var(--sunken)] px-1 py-px text-[0.88em] whitespace-nowrap text-zinc-800 dark:text-zinc-200"
					>repose run</code
				> in any checkout and your cloud machine picks up where your laptop is, down to the uncommitted
				edits.
			</SectionHead>
			<div class="landing-stage mt-14">
				<Perch shapes={['pill', 'star', 'sun']} />
				<OneCommand animated />
			</div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<SectionHead id="features" title="On every machine" />
			<div class="relative mt-14 grid gap-x-10 gap-y-12 md:grid-cols-2">
				<Perch shapes={['arch', 'moon', 'asterisk']} />
				<ComesBack />
				<Localhost />
				<Browser />
				<Editor />
			</div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<SectionHead title="Five agents and a full toolchain on first boot">
				The agent has sudo to install anything else, and
				<code
					class="rounded-xs bg-[var(--sunken)] px-1 py-px text-[0.88em] whitespace-nowrap text-zinc-800 dark:text-zinc-200"
					>repose config add</code
				> keeps it on every rebuild.
			</SectionHead>
			<div class="relative mt-14">
				<Perch shapes={['halves', 'diamond', 'leaf']} />
				<Ready />
			</div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<SectionHead title="Start in three commands" />
			<ol class="mt-8 border-b border-[var(--rule)]" use:landOnView>
				{#each steps as step, i (step.title)}
					<li
						class="step grid gap-3 border-t border-[var(--rule)] py-6 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:gap-10"
					>
						<div>
							<h3 class="flex items-center gap-3 text-lg font-semibold">
								<span class="step-mark land" style="--d: {i * 110}ms" aria-hidden="true"
									><span class="turn block h-full w-full"><Shape kind={stepShapes[i]} /></span
									></span
								>{step.title}
							</h3>
							<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">{step.text}</p>
						</div>
						<div class="min-w-0">
							{@render sample([`$ ${step.command}`])}
						</div>
					</li>
				{/each}
			</ol>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<SectionHead id="pricing" title="Pricing">
				Per hour while a machine runs, capped each month.
			</SectionHead>

			<ul class="mt-8 grid gap-5 md:grid-cols-3" use:landOnView>
				{#each tiers as t, i (t.name)}
					<li class="tier">
						<span class="tier-shape land" style="--d: {i * 110}ms" aria-hidden="true"
							><span class="turn block h-full w-full"><Shape kind={t.shape} /></span></span
						>
						<h3 class="tier-name">{t.name}</h3>
						<p class="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
							{t.vcpu} vCPU · {t.ram} memory · {t.disk} disk
						</p>
						<p class="tier-price">
							<span class="font-display text-5xl font-bold tracking-tight">{t.hour}</span>
							<span
								class="text-xs font-semibold tracking-wider text-zinc-600 uppercase dark:text-zinc-400"
								>per hour</span
							>
						</p>
						<p class="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
							Capped at <b class="font-semibold text-zinc-900 dark:text-zinc-100">{t.cap}</b> a month
						</p>
					</li>
				{/each}
			</ul>

			<div class="mt-10 flex flex-wrap items-center gap-4">
				<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
					Start with GitHub
				</button>
			</div>
		</div>
	</section>
</main>

<footer>
	<div class="border-b border-[var(--rule)]">
		<div class="frieze" aria-hidden="true" use:landOnView>
			{#each frieze as k, i (i)}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<span
					class="frieze-tile land"
					style="--d: {i * 60}ms"
					onpointerenter={() => (friezeTurn[i] += 90)}
					><span class="turn block h-full w-full" style="--a: {friezeTurn[i]}deg"
						><Shape kind={k} /></span
					></span
				>
			{/each}
		</div>
	</div>
	<div
		class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-4 px-5 py-8 text-sm text-zinc-600 dark:text-zinc-400"
	>
		<Logo size="sm" mark />
		<nav class="flex gap-6" aria-label="Footer">
			<a href={resolve('/docs')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Docs</a>
			<a href={SOURCE_URL} class="hover:text-zinc-900 dark:hover:text-zinc-100">GitHub</a>
			<a href={resolve('/terms')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Terms</a>
			<a href={resolve('/privacy')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Privacy</a>
		</nav>
	</div>
</footer>

<style>
	/* The headline: bold serif, one thick amber bar under "full
	   permissions", drawn in on load. */
	.hero-h {
		font-size: clamp(2.4rem, 5vw, 3.5rem);
		font-weight: 700;
		line-height: 1.02;
		letter-spacing: -0.02em;
		text-wrap: balance;
	}
	.hero-h .line {
		display: block;
	}
	/* Inline, so if it wraps on a phone each part gets its bar. */
	.hero-h .bar {
		padding-bottom: 0.1em;
		line-height: 1.22;
		-webkit-box-decoration-break: clone;
		box-decoration-break: clone;
		background: linear-gradient(var(--sh-amber), var(--sh-amber)) no-repeat 0 100% / 100% 0.13em;
	}
	@media (prefers-reduced-motion: no-preference) {
		.hero-h .bar {
			animation: bar 0.8s cubic-bezier(0.65, 0, 0.35, 1) 0.35s both;
		}
	}
	@keyframes bar {
		from {
			background-size: 0% 0.13em;
		}
	}

	/* Pricing: a card per size, its shape bleeding off the corner. */
	.tier {
		position: relative;
		overflow: hidden;
		padding: 1.75rem;
		border: 1px solid var(--rule);
		border-radius: 3px;
		background: var(--surface);
	}
	.tier-shape {
		position: absolute;
		top: -3rem;
		right: -3rem;
		width: 7.5rem;
		height: 7.5rem;
	}
	.tier:hover .turn,
	.step:hover .turn {
		--a: 90deg;
	}
	.tier-name {
		position: relative;
		font-size: 2.5rem;
		line-height: 1;
		font-weight: 700;
		letter-spacing: -0.01em;
		text-transform: uppercase;
	}
	.tier-price {
		display: flex;
		align-items: baseline;
		gap: 0.6rem;
		margin-top: 1.5rem;
		padding-top: 1.25rem;
		background: linear-gradient(var(--sh-amber), var(--sh-amber)) no-repeat 0 0 / 100% 4px;
	}

	.step-mark {
		width: 1.5rem;
		height: 1.5rem;
		flex: none;
	}

	/* The footer's row of shapes across the page, standing on its rule,
	   with room between them and above them. */
	.frieze {
		display: flex;
		align-items: flex-end;
		justify-content: center;
		gap: clamp(14px, 2.6vw, 44px);
		overflow: hidden;
		margin-top: clamp(1rem, 4vw, 3rem);
		padding: 0 clamp(14px, 2.6vw, 44px);
	}
	/* Where the row is wider than the screen it starts at the left edge,
	   rather than losing shapes off both ends. */
	@media (max-width: 1023px) {
		.frieze {
			justify-content: flex-start;
		}
	}
	.frieze-tile {
		flex: 0 0 clamp(40px, 5.6vw, 88px);
		aspect-ratio: 1;
	}
</style>
