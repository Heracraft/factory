<!--
  A guest's state as a small square and its word. The word carries the
  state; the square is a glance cue, so it is hidden from assistive tech
  and nothing depends on its colour. data-state and data-tone let the
  stylesheet draw the square for forced colours and reduced motion without
  this component knowing how.
-->
<script lang="ts">
	import type { GuestState } from '$lib/api/types';

	let { state }: { state: GuestState } = $props();

	type Tone = 'idle' | 'good' | 'busy' | 'error';

	const tone: Record<GuestState, Tone> = {
		running: 'good',
		stopped: 'idle',
		creating: 'busy',
		building: 'busy',
		starting: 'busy',
		stopping: 'busy',
		restoring: 'busy',
		destroying: 'busy',
		destroyed: 'idle',
		error: 'error'
	};

	const dotClass: Record<Tone, string> = {
		idle: 'dot',
		good: 'dot dot--good',
		busy: 'dot dot--busy',
		error: 'dot dot--error'
	};
</script>

<span class="inline-flex items-center gap-1.5" data-state={state}>
	<span class={dotClass[tone[state]]} data-tone={tone[state]} aria-hidden="true"></span>
	<span>{state}</span>
</span>
