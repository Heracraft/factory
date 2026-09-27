<script lang="ts">
	import './landing.css';
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { signIn } from '$lib/auth.svelte';
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
	const tiers: {
		name: string;
		vcpu: number;
		ram: string;
		disk: string;
		hour: string;
		cap: string;
	}[] = [
		{ name: 'small', vcpu: 2, ram: '4 GB', disk: '20 GB', hour: '$0.07', cap: '$49' },
		{ name: 'large', vcpu: 4, ram: '8 GB', disk: '40 GB', hour: '$0.14', cap: '$99' },
		{ name: 'xl', vcpu: 8, ram: '16 GB', disk: '80 GB', hour: '$0.28', cap: '$199' }
	];
	// Mostly grey, with a spot of blue every few shapes, the way the
	// pictures use colour.
	const frieze: [Kind, Tone][] = [
		['sun', 'neutral'],
		['moon', 'neutral'],
		['asterisk', 'neutral'],
		['pinwheel', 'accent'],
		['arch', 'neutral'],
		['ring', 'accent'],
		['star', 'accent'],
		['halves', 'neutral'],
		['leaf', 'neutral'],
		['sphere', 'neutral'],
		['pill', 'neutral']
	];
</script>

<svelte:head>
	<title>repose: let your agents run with full permissions</title>
	<meta
		name="description"
		content="A disposable dev machine per project with your code, tools and secrets on it in 15 seconds, so coding agents can run with full permissions and your laptop stays out of reach."
	/>
</svelte:head>

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
			<button type="button" class="btn-quiet !py-1.5" disabled={signingIn} onclick={onSignIn}
				>Sign in</button
			>
		</nav>
	</header>

	<main>
		<section class="sec">
			<div class="hero-grid inset">
				<h1 class="hero-h">
					<span class="line">Let your agents run</span>
					<span class="line">with <span class="bar">full permissions</span></span>
				</h1>
				<div class="hero-row">
					<p class="lead">
						One command puts your work on a machine of its own. The agent can wreck it, and a
						snapshot puts it back.
					</p>
					<div class="hero-ctas">
						<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
							Sign in with GitHub
						</button>
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
			</div>
			<div class="landing-stage">
				<Hero />
			</div>
		</section>

		<section class="sec">
			<SectionHead index="01" label="Run" title="Your working state, in one command">
				Run <code>repose run</code> in any checkout and your cloud machine picks up where your laptop
				is, down to the uncommitted edits.
			</SectionHead>
			<div class="landing-stage">
				<OneCommand animated />
			</div>
		</section>

		<section class="sec">
			<SectionHead index="02" label="Machine" id="features" title="On every machine" />
			<ul class="cells">
				<li class="cell">
					<ComesBack />
					<h3>Let it break the whole machine</h3>
					<p>
						Snapshots hold the whole disk: databases, installed tools, logins and uncommitted work.
						Restore one and the machine is back in minutes.
					</p>
				</li>
				<li class="cell">
					<Localhost />
					<h3>Your dev server on your localhost</h3>
					<p>
						While you're attached, every port the machine listens on is on your laptop's localhost,
						so cookies and OAuth redirects work as they do locally.
					</p>
				</li>
				<li class="cell">
					<Browser />
					<h3>Watch the agent use the browser</h3>
					<p>
						The agent drives Chromium on the machine and reads the console. <code
							>repose open --desktop</code
						> shows you the same window, and you can take over.
					</p>
				</li>
				<li class="cell">
					<Editor />
					<h3>Open it in your editor</h3>
					<p>
						Each machine is an SSH host named after your repo, so Neovim runs right on it and VS
						Code, Cursor or Zed connect over SSH.
					</p>
				</li>
			</ul>
		</section>

		<section class="sec">
			<SectionHead
				index="03"
				label="Toolchain"
				title="Five agents and a full toolchain on first boot"
			>
				The agent has sudo to install anything else, and <code>repose config add</code> keeps it on every
				rebuild.
			</SectionHead>
			<div class="landing-stage">
				<Ready />
			</div>
		</section>

		<section class="sec">
			<SectionHead index="04" label="Start" title="Start in three commands" />
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
							<p>{step.text}</p>
						</div>
						<div class="cmd"><span class="text">$ {step.command}</span></div>
					</li>
				{/each}
			</ol>
		</section>

		<section class="sec">
			<SectionHead index="05" label="Pricing" id="pricing" title="Pricing">
				Per hour while a machine runs, capped each month.
			</SectionHead>
			<ul class="tiers" use:landOnView>
				{#each tiers as t, i (t.name)}
					<li class="tier">
						<div class="tier-top">
							<h3 class="tier-name">{t.name}</h3>
							<span class="tier-units land" style="--d: {i * 110}ms"><Units count={t.vcpu} /></span>
						</div>
						<p class="tier-spec">{t.vcpu} vCPU · {t.ram} memory · {t.disk} disk</p>
						<p class="tier-price">
							<span class="n">{t.hour}</span>
							<span class="per">per hour</span>
						</p>
						<p class="tier-cap">Capped at <b>{t.cap}</b> a month</p>
					</li>
				{/each}
			</ul>
			<div class="cta-row">
				<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
					Start with GitHub
				</button>
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
			</nav>
		</div>
	</footer>
</div>
