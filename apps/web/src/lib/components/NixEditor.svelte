<script lang="ts">
	import { onMount } from 'svelte';
	import { StateEffect, StateField } from '@codemirror/state';
	import {
		EditorView,
		keymap,
		lineNumbers,
		highlightActiveLine,
		highlightActiveLineGutter,
		Decoration,
		type DecorationSet
	} from '@codemirror/view';
	import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
	import { syntaxHighlighting, defaultHighlightStyle, bracketMatching } from '@codemirror/language';
	import { nix } from '@replit/codemirror-lang-nix';

	let {
		value = $bindable(''),
		errorLine,
		readonly = false
	}: { value: string; errorLine?: number; readonly?: boolean } = $props();

	let container: HTMLDivElement;
	let view: EditorView | undefined;

	// A build error names a fragment.nix line (nix-build-contract.md); this
	// paints that one line so the user does not have to count.
	const setErrorLine = StateEffect.define<number | null>();
	const errorLineField = StateField.define<DecorationSet>({
		create: () => Decoration.none,
		update(deco, tr) {
			deco = deco.map(tr.changes);
			for (const e of tr.effects) {
				if (e.is(setErrorLine)) {
					if (e.value == null || e.value < 1 || e.value > tr.state.doc.lines) return Decoration.none;
					const line = tr.state.doc.line(e.value);
					return Decoration.set([Decoration.line({ class: 'cm-line-error' }).range(line.from)]);
				}
			}
			return deco;
		},
		provide: (f) => EditorView.decorations.from(f)
	});

	onMount(() => {
		view = new EditorView({
			doc: value,
			parent: container,
			extensions: [
				lineNumbers(),
				highlightActiveLine(),
				highlightActiveLineGutter(),
				history(),
				bracketMatching(),
				syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
				nix(),
				errorLineField,
				keymap.of([...defaultKeymap, ...historyKeymap]),
				EditorView.editable.of(!readonly),
				EditorView.updateListener.of((u) => {
					if (u.docChanged) value = u.state.doc.toString();
				})
			]
		});
		return () => view?.destroy();
	});

	// The parent replaced the whole document (loaded a revision, "Edit as
	// Nix" copied the generated fragment in) — not the user typing, which
	// already flows the other way via the update listener above.
	$effect(() => {
		if (view && view.state.doc.toString() !== value) {
			view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } });
		}
	});

	$effect(() => {
		view?.dispatch({ effects: setErrorLine.of(errorLine ?? null) });
	});
</script>

<div class="nix-editor h-80" bind:this={container}></div>
