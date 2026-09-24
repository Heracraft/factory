import { describe, expect, it } from 'vitest';
import { DOCS, SECTIONS, docBySlug, highlightNix, search, slugify } from './docs';

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
});
