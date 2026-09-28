<script lang="ts">
	import './landing.css';
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { authState, signIn } from '$lib/auth.svelte';
	import { publicSeats } from '$lib/api/client';
	import type { PublicSeats } from '$lib/api/types';
	import Logo from '$lib/components/Logo.svelte';
	import Hero from '$lib/components/landing/Hero.svelte';
	import Gauge from '$lib/components/landing/Gauge.svelte';
	import Units from '$lib/components/landing/Units.svelte';
	import Shape, { type Kind, type Tone } from '$lib/components/landing/Shape.svelte';
	import SectionHead from '$lib/components/landing/SectionHead.svelte';
	import { landOnView } from '$lib/components/landing/inview';
	import OneCommand from '$lib/components/landing/OneCommand.svelte';
	import ComesBack from '$lib/components/landing/ComesBack.svelte';
	import Browser from '$lib/components/landing/Browser.svelte';
	import Localhost from '$lib/components/landing/Localhost.svelte';
	import Ready from '$lib/components/landing/Ready.svelte';
	import Editor from '$lib/components/landing/Editor.svelte';

	const INSTALL_COMMAND = 'curl -fsSL https://repose.herakraft.co/install.sh | sh';
	const SOURCE_URL = 'https://github.com/Heracraft/repose';

	let signingIn = $state(false);
	let copied = $state(false);
	/** GET /public/seats (I-290): the launch's gauge; undefined until it answers, and if it never does. */
	let seats = $state<PublicSeats | undefined>(undefined);

	onMount(() => {
		publicSeats()
			.then((s) => (seats = s))
			.catch(() => {
				// The plans stand on their own without the count.
			});
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
		{ title: 'Install the CLI', command: INSTALL_COMMAND },
		{ title: 'Sign in', command: 'repose login' },
		{ title: 'Run in any checkout', command: 'cd ~/code/recruiting && repose run' }
	];
	// docs/PRICING.md's two plans. The Units count is the memory that may
	// run at once, one square per GB, so the plans compare at a glance.
	const plans: {
		name: string;
		price: string;
		memory: number;
		runs: string;
		disk: string;
		egress: string;
	}[] = [
		{
			name: 'Solo',
			price: '$29',
			memory: 8,
			runs: 'one large, or two small',
			disk: '100 GB',
			egress: '250 GB'
		},
		{
			name: 'Pro',
			price: '$59',
			memory: 16,
			runs: 'one xl, two large, or any mix',
			disk: '250 GB',
			egress: '500 GB'
		}
	];
	// The footer's row: every shape the page used, in the order it used
	// them, so the row reads as the page's own symbols and none appears
	// from nowhere. Mostly grey, a spot of blue, as the pictures are.
	const frieze: [Kind, Tone][] = [
		['pinwheel', 'accent'],
		['sphere', 'neutral'],
		['pill', 'neutral'],
		['halves', 'neutral'],
		['ring', 'neutral'],
		['arch', 'neutral'],
		['asterisk', 'neutral'],
		['star', 'accent']
	];
</script>

<svelte:head>
	<title>repose: let your agents run with full permissions</title>
	<meta
		name="description"
		content="A disposable dev machine per project with your code, tools and secrets on it in 15 seconds, so coding agents can run with full permissions and your laptop stays out of reach."
	/>
</svelte:head>

{#snippet cellMark(kind: Kind)}
	<span class="cell-mark" aria-hidden="true"><Shape {kind} /></span>
{/snippet}

{#snippet copyIcon()}
	<svg
		viewBox="0 0 16 16"
		class="h-3.5 w-3.5"
		fill="none"
		stroke="currentColor"
		stroke-width="1.4"
		aria-hidden="true"
		><rect x="5.5" y="5.5" width="8" height="8" rx="1" /><path d="M10.5 3.5v-1h-8v8h1" /></svg
	>
{/snippet}

<div class="rails">
	<header class="topbar inset">
		<a href={resolve('/')} aria-label="repose, home"><Logo mark /></a>
		<nav class="flex items-center gap-6" aria-label="Main">
			<a href={resolve('/docs')} class="hidden sm:inline">Docs</a>
			<a href="#pricing" class="hidden sm:inline">Pricing</a>
			<a href={SOURCE_URL} class="hidden sm:inline">GitHub</a>
			{#if authState.authenticated}
				<a href={resolve('/projects')} class="btn-quiet !py-1.5">Dashboard</a>
			{:else}
				<button type="button" class="btn-quiet !py-1.5" disabled={signingIn} onclick={onSignIn}
					>Sign in</button
				>
			{/if}
		</nav>
	</header>

	<main>
		<section class="sec">
			<div class="hero-grid inset">
				<h1 class="hero-h">
					<span class="line">Let your agents run</span>
					<span class="line">with <span class="bar">full permissions</span></span>
				</h1>
				<p class="lead">
					Your work on a machine of its own. The agent can wreck it. A snapshot puts it back.
				</p>
				<div class="hero-ctas">
					{#if authState.authenticated}
						<a href={resolve('/projects')} class="btn !px-5 !py-2.5">Open the dashboard</a>
					{:else}
						<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
							Get started
						</button>
					{/if}
					<div class="cmd">
						<span class="text">{INSTALL_COMMAND}</span>
						<button
							type="button"
							class="copy"
							onclick={copyInstall}
							aria-label="Copy the install command"
						>
							{@render copyIcon()}
							{copied ? 'Copied' : 'Copy'}
						</button>
					</div>
				</div>
			</div>
			<div class="landing-stage">
				<Hero />
			</div>
		</section>

		<section class="sec">
			<SectionHead shape="pill" title="Your working state, in one command" />
			<div class="landing-stage">
				<OneCommand animated />
			</div>
		</section>

		<section class="sec">
			<SectionHead id="features" title="On every machine" />
			<ul class="cells">
				<li class="cell">
					<ComesBack />
					<h3>{@render cellMark('pinwheel')}Let it break the whole machine</h3>
					<p>Databases, tools, logins, uncommitted work. Back in minutes.</p>
				</li>
				<li class="cell">
					<Localhost />
					<h3>{@render cellMark('halves')}Your dev server on your localhost</h3>
					<p>
						Every port the machine listens on, on your laptop. Cookies and OAuth redirects included.
					</p>
				</li>
				<li class="cell">
					<Browser />
					<h3>{@render cellMark('ring')}Watch the agent use the browser</h3>
					<p><code>repose browser</code> puts you in the same window. Take over any time.</p>
				</li>
				<li class="cell">
					<Editor />
					<h3>{@render cellMark('arch')}Open it in your editor</h3>
					<p>Every machine is an SSH host. Neovim on it, VS Code, Cursor or Zed over SSH.</p>
				</li>
			</ul>
		</section>

		<section class="sec">
			<SectionHead shape="asterisk" title="Five agents and a full toolchain on first boot" />
			<div class="landing-stage">
				<Ready />
			</div>
		</section>

		<section class="sec">
			<SectionHead title="Start in three commands" />
			<ol class="steps" use:landOnView>
				{#each steps as step, i (step.title)}
					<li class="step">
						<div>
							<h3>
								<span class="step-mark land" style="--d: {i * 110}ms" aria-hidden="true"
									><Gauge
										fraction={(i + 1) / steps.length}
										tone={i === steps.length - 1 ? 'accent' : 'neutral'}
									/></span
								>{step.title}
							</h3>
						</div>
						<div class="cmd"><span class="text">$ {step.command}</span></div>
					</li>
				{/each}
			</ol>
		</section>

		<section class="sec">
			<SectionHead id="pricing" title="Pricing">
				Two plans. Seven days free, card at checkout.
			</SectionHead>
			<ul class="tiers tiers--two" use:landOnView>
				{#each plans as t, i (t.name)}
					<li class="tier">
						<div class="tier-top">
							<h3 class="tier-name">{t.name}</h3>
							<span class="tier-units land" style="--d: {i * 110}ms"
								><Units count={t.memory} /></span
							>
						</div>
						<p class="tier-spec">
							<span>{t.memory} GB running at once</span> · <span>{t.disk} disk</span> ·
							<span>{t.egress} egress</span>
						</p>
						<p class="tier-price">
							<span class="n">{t.price}</span>
							<span class="per">a month</span>
						</p>
						<p class="tier-cap"><b>{t.memory} GB</b> is {t.runs}</p>
					</li>
				{/each}
			</ul>
			<div class="cta-row">
				{#if authState.authenticated}
					<a href={resolve('/projects')} class="btn !px-5 !py-2.5">Open the dashboard</a>
				{:else}
					<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
						Start a free week
					</button>
					{#if seats}
						<p class="seats" data-testid="seats-line">
							{#if seats.free > 0}
								{seats.free} {seats.free === 1 ? 'seat' : 'seats'} left
							{:else}
								Full for now. {seats.waiting} waiting; join the list and you're emailed when a seat frees.
							{/if}
						</p>
					{/if}
				{/if}
			</div>
		</section>
	</main>

	<footer class="sec">
		<ul class="frieze" aria-hidden="true" use:landOnView>
			{#each frieze as [k, tone], i (i)}
				<li>
					<span class="land block h-full w-full" style="--d: {i * 60}ms"
						><Shape kind={k} {tone} /></span
					>
				</li>
			{/each}
		</ul>
		<div class="foot inset">
			<Logo size="sm" mark />
			<nav aria-label="Footer">
				<a href={resolve('/docs')}>Docs</a>
				<a href={SOURCE_URL}>GitHub</a>
				<a href={resolve('/terms')}>Terms</a>
				<a href={resolve('/privacy')}>Privacy</a>
				<a href={resolve('/refunds')}>Refunds</a>
			</nav>
		</div>
	</footer>
</div>
