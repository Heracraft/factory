<script lang="ts">
	// Questions an agent asked with repose-ask and is waiting on (I-245):
	// answered here with a button per option or a text box, or dismissed.
	import { onMount } from 'svelte';
	import { listProjectQuestions, answerQuestion, cancelQuestion } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import { pollWhileVisible } from '$lib/poll';
	import { relativeTime } from '$lib/format';
	import type { Question } from '$lib/api/types';

	let { projectId }: { projectId: string } = $props();

	let questions = $state<Question[]>([]);
	let drafts = $state<Record<string, string>>({});
	let notes = $state<Record<string, string>>({});
	let busy = $state<string | undefined>(undefined);
	let answered = $state<string | undefined>(undefined);

	async function refresh() {
		try {
			questions = await listProjectQuestions(projectId);
		} catch (err) {
			toastApiError(err, 'Could not load questions.');
		}
	}

	onMount(() => pollWhileVisible(refresh));

	function expiresIn(iso: string): string {
		const minutes = Math.max(0, Math.round((new Date(iso).getTime() - Date.now()) / 60_000));
		if (minutes < 60) return `${minutes}m`;
		const h = Math.floor(minutes / 60);
		const m = minutes % 60;
		return m ? `${h}h ${m}m` : `${h}h`;
	}

	async function act(q: Question, answer?: string) {
		busy = q.id;
		notes = { ...notes, [q.id]: '' };
		try {
			if (answer === undefined) {
				await cancelQuestion(projectId, q.id);
				answered = `Dismissed: ${q.text.split('\n')[0]}`;
			} else {
				const res = await answerQuestion(projectId, q.id, answer);
				answered = `Answered: ${res.answer ?? answer}`;
			}
			setTimeout(() => (answered = undefined), 8_000);
			await refresh();
		} catch (err) {
			if (err instanceof ApiError && err.code === 'conflict') {
				// Answered elsewhere first (ntfy, email, the CLI): the list
				// drops it, so the reason goes in the status line.
				answered = `Not sent: ${err.message}.`;
				await refresh();
			} else if (err instanceof ApiError && err.code === 'invalid') {
				const opts = (err.detail?.options as string[] | undefined) ?? q.options;
				notes = {
					...notes,
					[q.id]: opts.length ? `The answer is one of: ${opts.join(', ')}.` : err.message
				};
			} else {
				toastApiError(err, 'Could not send the answer.');
			}
		} finally {
			busy = undefined;
		}
	}
</script>

{#if questions.length > 0 || answered}
	<div class="card mt-6" data-testid="questions">
		<h2 class="font-semibold">Questions</h2>
		{#if answered}
			<p class="mt-2 text-sm text-zinc-600 dark:text-zinc-400" role="status">{answered}</p>
		{/if}
		<ul class="mt-2">
			{#each questions as q (q.id)}
				<li class="row text-sm" data-testid="question">
					<p class="font-medium">{q.agent} asks</p>
					<p class="mt-1 whitespace-pre-wrap">{q.text}</p>
					<p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
						asked {relativeTime(q.created_at)}, expires in {expiresIn(q.expires_at)}
					</p>
					<div class="mt-2 flex flex-wrap items-center gap-2">
						{#if q.options.length > 0}
							{#each q.options as o (o)}
								<button type="button" class="btn" disabled={busy === q.id} onclick={() => act(q, o)}
									>{o}</button
								>
							{/each}
						{:else}
							<label class="sr-only" for={`answer-${q.id}`}>Your answer</label>
							<input
								id={`answer-${q.id}`}
								class="field w-72 py-1"
								placeholder="Your answer"
								bind:value={drafts[q.id]}
								onkeydown={(e) => {
									if (e.key === 'Enter' && drafts[q.id]?.trim()) void act(q, drafts[q.id]);
								}}
							/>
							<button
								type="button"
								class="btn"
								disabled={busy === q.id || !drafts[q.id]?.trim()}
								onclick={() => act(q, drafts[q.id])}>Answer</button
							>
						{/if}
						<button type="button" class="btn-ghost" disabled={busy === q.id} onclick={() => act(q)}
							>Dismiss</button
						>
					</div>
					{#if notes[q.id]}
						<p class="mt-2 text-sm text-red-700 dark:text-red-400">{notes[q.id]}</p>
					{/if}
				</li>
			{/each}
		</ul>
	</div>
{/if}
