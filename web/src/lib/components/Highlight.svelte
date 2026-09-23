<script lang="ts">
	import { highlightSegments } from '$lib/search';

	let { text, terms = [] }: { text: string; terms?: string[] } = $props();

	const segments = $derived(highlightSegments(text, terms));
</script>

{#each segments as segment, index (index)}{#if segment.match}<mark>{segment.text}</mark
		>{:else}{segment.text}{/if}{/each}
