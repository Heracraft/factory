<script lang="ts">
	import { Marked } from 'marked';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { codespan } from '$lib/codespan';
	import HeaderFrame from './HeaderFrame.svelte';

	// Inline code renders as the docs render it, so `repose-notify` stays
	// on one line here too; marked's default let it break at its hyphen.
	const marked = new Marked({ renderer: { codespan } });

	let { raw }: { raw: string } = $props();

	// content/legal/*.md files carry a small YAML-ish frontmatter block
	// (title, effective, status) that 14-security's docs use to mark drafts;
	// this is the only place that reads it, so a tiny hand-rolled parser
	// beats pulling in a YAML dependency for three known keys.
	function parseFrontmatter(text: string): { meta: Record<string, string>; body: string } {
		const match = /^---\n([\s\S]*?)\n---\n([\s\S]*)$/.exec(text);
		if (!match) return { meta: {}, body: text };
		const meta: Record<string, string> = {};
		for (const line of match[1].split('\n')) {
			const i = line.indexOf(':');
			if (i === -1) continue;
			meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
		}
		return { meta, body: match[2] };
	}

	const MONTHS = [
		'January',
		'February',
		'March',
		'April',
		'May',
		'June',
		'July',
		'August',
		'September',
		'October',
		'November',
		'December'
	];

	// "2026-09-27" as "27 September 2026", by hand rather than with
	// toLocaleDateString, so the prerendered text and the hydrated text are
	// the same string whatever the server's and the reader's locale.
	function longDate(iso: string): string {
		const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
		return m ? `${Number(m[3])} ${MONTHS[Number(m[2]) - 1]} ${m[1]}` : iso;
	}

	// A heading's id, as the docs spell theirs ("Who can see your data" is
	// who-can-see-your-data). Written here rather than imported from
	// lib/docs, which would pull every docs page into the legal bundle.
	function slug(text: string): string {
		return text
			.toLowerCase()
			.replace(/<[^>]+>/g, '')
			.replace(/&[a-z]+;|&#\d+;/g, '')
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	let { meta, body } = $derived(parseFrontmatter(raw));
	// The effective date goes straight under the policy's own title, where a
	// reader looks for it; the frontmatter held it and nothing showed it.
	// Each h2 gets an id, so the contents beside the text can link to it.
	let rendered = $derived.by(() => {
		const sections: { id: string; text: string }[] = [];
		const html = (marked.parse(body) as string)
			.replace(
				'</h1>',
				meta.effective
					? `</h1>\n<p class="effective">Effective ${longDate(meta.effective)}</p>`
					: '</h1>'
			)
			.replace(/<h2>(.*?)<\/h2>/g, (_, inner: string) => {
				const id = slug(inner);
				sections.push({ id, text: inner.replace(/<[^>]+>/g, '').replace(/&amp;/g, '&') });
				return `<h2 id="${id}">${inner}</h2>`;
			});
		return { html, sections };
	});

	const links = [
		{ href: resolve('/terms'), label: 'Terms' },
		{ href: resolve('/privacy'), label: 'Privacy' },
		{ href: resolve('/refunds'), label: 'Refunds' }
	];
</script>

<HeaderFrame home={resolve('/')} label="repose, home">
	<nav class="flex h-full items-stretch gap-4 text-sm sm:gap-6" aria-label="Policies">
		{#each links as link (link.href)}
			<!-- eslint-disable svelte/no-navigation-without-resolve -- link.href is built with resolve() in the links array above -->
			<a
				href={link.href}
				aria-current={page.url.pathname === link.href ? 'page' : undefined}
				class="-mb-px flex items-center border-b {page.url.pathname === link.href
					? 'border-ink text-ink'
					: 'border-transparent text-ink-muted hover:text-ink'}">{link.label}</a
			>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{/each}
	</nav>
</HeaderFrame>

<!-- The same column as the dashboard's PageShell, flush with the logo,
     with the text held to 33rem, about 68 characters of the body sans a
     line (the docs' prose measure): at max-w-3xl the policies ran past 90,
     which is hard to read and easy to lose your place in. The article takes the
     docs' prose styles (.doc), so inline code is a quiet chip in the body
     weight rather than bold mono in literal backticks, and a policy reads
     like the docs page that links to it. From lg up the policy's sections
     are listed beside the text, where a 33rem text alone left 450px of the
     984px column empty and the page leaning left. Beside it, 64px off,
     and not pushed to the column's right edge: there the list sat 230px
     from the text and read as a separate thing (I-393). -->
<main id="main" class="mx-auto flex max-w-5xl gap-16 px-5 pt-10 pb-24">
	<div class="max-w-[33rem] min-w-0 flex-1">
		{#if meta.status}
			<p class="banner banner--warn">Draft: {meta.status}</p>
		{/if}
		<article
			class="doc prose max-w-none [&_.effective]:mt-[-0.75em] [&_.effective]:text-sm [&_.effective]:text-ink-muted"
		>
			<!-- eslint-disable-next-line svelte/no-at-html-tags -- `raw` only ever comes from this repo's own src/content/legal/*.md via a ?raw import, never from a user or the api -->
			{@html rendered.html}
		</article>
	</div>
	{#if rendered.sections.length > 1}
		<!-- The docs sidebar's "On this page" list, in the same type and
		     the same 28px rows, sticky so it stays beside a long policy. -->
		<nav class="hidden w-56 shrink-0 lg:block" aria-label="On this page">
			<div class="sticky top-10 border-l border-rule pl-3">
				<p class="text-sm font-medium text-ink">On this page</p>
				<ul class="mt-1">
					{#each rendered.sections as h (h.id)}
						<li>
							<a
								href={`#${h.id}`}
								class="block py-1 text-compact leading-5 text-ink-muted hover:text-ink">{h.text}</a
							>
						</li>
					{/each}
				</ul>
			</div>
		</nav>
	{/if}
</main>
