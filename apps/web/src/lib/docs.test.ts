import { describe, expect, it } from 'vitest';
import {
	DOCS,
	SECTIONS,
	blockLines,
	docBySlug,
	highlightNix,
	movedDoc,
	renderShell,
	search,
	slugify
} from './docs';

// Every /docs link in the docs names a page that exists and, when it has a
// #fragment, a heading on that page: a renamed heading otherwise breaks
// links silently.
describe('user docs', () => {
	it('has an overview and puts every page in a known section', () => {
		expect(docBySlug('index')).toBeDefined();
		for (const d of DOCS) {
			expect(SECTIONS, d.slug).toContain(d.section);
			expect(d.title, d.slug).not.toBe(d.slug);
			expect(d.description, d.slug).not.toBe('');
		}
	});

	it('links only to pages and headings that exist', () => {
		const broken: string[] = [];
		for (const d of DOCS) {
			for (const [, slug, anchor] of d.body.matchAll(
				/\]\(\/docs(?:\/([a-z0-9-]+))?(?:#([a-z0-9-]+))?\)/g
			)) {
				const target = docBySlug(slug ?? 'index');
				if (!target) {
					broken.push(`${d.slug} -> /docs/${slug}`);
					continue;
				}
				if (anchor && !target.headings.some((h) => h.id === anchor)) {
					broken.push(`${d.slug} -> /docs/${slug ?? ''}#${anchor}`);
				}
			}
		}
		expect(broken).toEqual([]);
	});

	it('uses no em dashes', () => {
		for (const d of DOCS) expect(d.body.includes('—'), d.slug).toBe(false);
	});

	// Billing is not enforced yet: the docs must not send anyone to add a
	// card first, and must not carry release caveats that went stale.
	it('promises no card step and has no stale release notes', () => {
		for (const d of DOCS) {
			expect(d.text.includes('add a card'), d.slug).toBe(false);
			expect(d.text.includes('not in a release yet'), d.slug).toBe(false);
		}
	});

	it('gives every page its own title and a place in its section', () => {
		const titles = new Set(DOCS.map((d) => d.title));
		expect(titles.size).toBe(DOCS.length);
		const places = new Set(DOCS.map((d) => `${d.section}/${d.order}`));
		expect(places.size).toBe(DOCS.length);
	});

	it('slugs headings the way links spell them', () => {
		expect(slugify("What you'll get")).toBe('what-youll-get');
		expect(slugify('`repose open`')).toBe('repose-open');
		expect(slugify('Things that don’t work yet')).toBe('things-that-dont-work-yet');
	});

	it('finds pages by words in them', () => {
		expect(search('ntfy topic')[0]?.doc.slug).toBe('notifications');
		expect(search('')).toEqual([]);
		expect(search('zzzz-not-a-word')).toEqual([]);
		// A line commented out in a page is not searchable.
		expect(search('live product for other people')).toEqual([]);
	});

	it('sends a moved page on to its new slug', () => {
		expect(movedDoc('tutorial-your-chrome')).toBe('your-chrome');
		expect(docBySlug(movedDoc('tutorial-your-chrome') ?? '')).toBeDefined();
		expect(movedDoc('your-chrome')).toBeUndefined();
	});

	// The grammar's bare `parser` export has no highlight tags (they live on
	// nixLanguage), which rendered every block as plain text.
	it('highlights nix blocks', () => {
		const out = highlightNix('{ pkgs, ... }:\n{\n  # a\n  x = with pkgs; [ "s" true 1 ];\n}\n');
		for (const tok of ['tok-comment', 'tok-keyword', 'tok-string', 'tok-bool', 'tok-number']) {
			expect(out, tok).toContain(`class="${tok}"`);
		}
		expect(highlightNix('x = "<b>";')).toContain('&lt;b&gt;');
		const config = docBySlug('config')!;
		expect(config.html).toContain('<code class="language-nix"><span');
	});

	it('highlights shell, toml and json blocks and gives each a copy button', () => {
		const cli = docBySlug('cli')!.html;
		expect(cli).toContain('<code class="language-shell">');
		expect(cli).toContain('<code class="language-toml"><span');
		expect(docBySlug('agents')!.html).toContain('<code class="language-json">');
		expect(cli).toContain('<button type="button" class="copy"');
		// Program output is ```text: plain, nothing to copy.
		const index = docBySlug('index')!.html;
		expect(index).toContain('<code class="language-text">');
		expect(index.match(/class="copy"/g)?.length).toBe(
			index.match(/<code class="language-(?!text)/g)?.length
		);
	});

	it('copies only the commands of a block with prompts', () => {
		const { html, copy } = renderShell(
			'$ repose ls\nPROJECT  STATE\n\n$ repose stop "a"\nStopped a.'
		);
		expect(copy).toBe('repose ls\nrepose stop "a"');
		expect(html).toContain('<span class="prompt">$ </span>');
		expect(html).toContain('<span class="output">Stopped a.</span>');
		expect(renderShell('repose run # go').copy).toBe('repose run # go');
	});

	it('puts each line of a block in its own box, hung past its indent', () => {
		expect(blockLines('a\n  b\n\nc\n')).toBe(
			'<span class="line" style="--hang:4ch">a</span>' +
				'<span class="line" style="--hang:6ch">  b</span>' +
				'<span class="line" style="--hang:4ch"></span>' +
				'<span class="line" style="--hang:4ch">c</span>'
		);
		const git = docBySlug('tutorial-git')!.html;
		expect(git).toContain('<code class="language-text"><span class="line"');
		expect(git).not.toMatch(/<code class="language-\w+">(?!<span class="line")/);
	});
});
