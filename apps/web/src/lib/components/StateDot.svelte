<script lang="ts">
	import type { GuestState } from '$lib/api/types';

	let { state }: { state: GuestState } = $props();

	const dotClass: Record<GuestState, string> = {
		running: 'dot--good',
		stopped: 'dot',
		creating: 'dot--busy',
		building: 'dot--busy',
		starting: 'dot--busy',
		stopping: 'dot--busy',
		restoring: 'dot--busy',
		destroying: 'dot--busy',
		destroyed: 'dot',
		error: 'dot--error'
	};
</script>

<span class="inline-flex items-center gap-1.5">
	<span class="dot {dotClass[state]}"></span>
	<span>{state}</span>
</span>
