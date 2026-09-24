// The user docs: src/content/docs/*.md, each with a small frontmatter block
// (title, description, section, order). Bundled at build time with
// import.meta.glob, so /docs needs no server and no fetch.
import { Marked, type Tokens } from 'marked';
import { classHighlighter, highlightCode } from '@lezer/highlight';
import { nixLanguage } from '@replit/codemirror-lang-nix';

export interface DocHeading {
	depth: number;
	text: string;
	id: string;
}

export interface Doc {
	slug: string;
	title: string;
	description: string;
	section: string;
	order: number;
	body: string;
	html: string;
	headings: DocHeading[];
	/** Plain text of the page, for search snippets. */
	plain: string;
	/** The same, lowercased, for matching. */
	text: string;
}

/** The sections in the order the sidebar shows them. */
export const SECTIONS = ['Start here', 'Using repose', 'Account', 'Reference'];

const files = import.meta.glob('../content/docs/*.md', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

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

/** A heading's anchor: lowercase words joined by dashes, punctuation dropped. */
export function slugify(text: string): string {
	return text
		.toLowerCase()
		.replace(/<[^>]+>/g, '')
		.replace(/&[a-z#0-9]+;/g, '')
		.replace(/['’]/g, '')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '');
}

function plain(markdown: string): string {
	return markdown
		.replace(/```[\s\S]*?```/g, ' ')
		.replace(/`([^`]*)`/g, '$1')
		.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
		.replace(/^\s*[#>|-]+/gm, ' ')
		.replace(/[*|]/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
}

function escapeHTML(text: string): string {
	return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

/**
 * A ```nix block, highlighted with the same grammar as the dashboard's Nix
 * editor. Tokens get lezer's `tok-*` classes; layout.css colours them.
 */
export function highlightNix(code: string): string {
	let out = '';
	highlightCode(
		code,
		nixLanguage.parser.parse(code),
		classHighlighter,
		(text, classes) => {
			out += classes ? `<span class="${classes}">${escapeHTML(text)}</span>` : escapeHTML(text);
		},
		() => (out += '\n')
	);
	return out;
}

/** Heading text for the page's own list: tags dropped, entities decoded. */
function headingText(html: string): string {
	return html
		.replace(/<[^>]+>/g, '')
		.replace(/&#39;/g, "'")
		.replace(/&quot;/g, '"')
		.replace(/&lt;/g, '<')
		.replace(/&gt;/g, '>')
		.replace(/&amp;/g, '&');
}

function render(body: string): { html: string; headings: DocHeading[] } {
	const headings: DocHeading[] = [];
	const seen = new Map<string, number>();
	const marked = new Marked({
		renderer: {
			heading(
				this: { parser: { parseInline(t: Tokens.Generic[]): string } },
				token: Tokens.Heading
			) {
				const inner = this.parser.parseInline(token.tokens);
				let id = slugify(token.text);
				const n = seen.get(id) ?? 0;
				seen.set(id, n + 1);
				if (n) id = `${id}-${n}`;
				if (token.depth === 2 || token.depth === 3) {
					headings.push({ depth: token.depth, text: headingText(inner), id });
				}
				return `<h${token.depth} id="${id}"><a class="anchor" href="#${id}" aria-hidden="true" tabindex="-1">#</a>${inner}</h${token.depth}>\n`;
			},
			code(token: Tokens.Code) {
				if (token.lang !== 'nix') return false;
				return `<pre><code class="language-nix">${highlightNix(token.text)}</code></pre>\n`;
			},
			blockquote(
				this: { parser: { parse(t: Tokens.Generic[]): string } },
				token: Tokens.Blockquote
			) {
				return `<aside class="note">${this.parser.parse(token.tokens)}</aside>\n`;
			}
		}
	});
	let html = marked.parse(body) as string;
	// Wide tables scroll inside their own box, never the page.
	html = html
		.replace(/<table>/g, '<div class="table-wrap"><table>')
		.replace(/<\/table>/g, '</table></div>');
	return { html, headings };
}

export const DOCS: Doc[] = Object.entries(files)
	.map(([path, raw]) => {
		const slug = path.split('/').pop()!.replace(/\.md$/, '');
		const { meta, body } = parseFrontmatter(raw);
		const { html, headings } = render(body);
		const plainText = plain(`${meta.title ?? ''}. ${meta.description ?? ''} ${body}`);
		return {
			slug,
			title: meta.title ?? slug,
			description: meta.description ?? '',
			section: meta.section ?? 'Reference',
			order: Number(meta.order ?? 99),
			body,
			html,
			headings,
			plain: plainText,
			text: plainText.toLowerCase()
		};
	})
	.sort(
		(a, b) =>
			SECTIONS.indexOf(a.section) - SECTIONS.indexOf(b.section) ||
			a.order - b.order ||
			a.title.localeCompare(b.title)
	);

export function docBySlug(slug: string): Doc | undefined {
	return DOCS.find((d) => d.slug === slug);
}

/** The docs grouped by section, in sidebar order. */
export function docsBySection(): { section: string; docs: Doc[] }[] {
	return SECTIONS.map((section) => ({
		section,
		docs: DOCS.filter((d) => d.section === section)
	})).filter((g) => g.docs.length);
}

export interface SearchHit {
	doc: Doc;
	heading?: DocHeading;
	snippet: string;
}

/**
 * Every word of the query must appear on the page. Hits in a title rank
 * first, then in a heading, then in the text; the snippet is the text
 * around the first word's first match.
 */
export function search(query: string, limit = 8): SearchHit[] {
	const words = query.toLowerCase().split(/\s+/).filter(Boolean);
	if (!words.length) return [];
	const hits: { hit: SearchHit; score: number }[] = [];
	for (const doc of DOCS) {
		if (!words.every((w) => doc.text.includes(w))) continue;
		let score = 0;
		const title = doc.title.toLowerCase();
		for (const w of words) if (title.includes(w)) score += 10;
		const heading = doc.headings.find((h) => words.some((w) => h.text.toLowerCase().includes(w)));
		if (heading) score += 5;
		const at = doc.text.indexOf(words[0]);
		const start = Math.max(0, at - 50);
		const snippet =
			(start > 0 ? '…' : '') +
			doc.plain.slice(start, at + 90).trim() +
			(at + 90 < doc.text.length ? '…' : '');
		hits.push({ hit: { doc, heading, snippet }, score });
	}
	return hits
		.sort((a, b) => b.score - a.score)
		.slice(0, limit)
		.map((h) => h.hit);
}
