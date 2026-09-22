<script lang="ts">
	let { title, hint = '' }: { title: string; hint?: string } = $props();
</script>

<div class="empty">
	<p class="title">{title}</p>
	{#if hint}
		<p class="hint">{hint}</p>
	{/if}
</div>

<style>
	.empty {
		border: 1px dashed var(--border);
		border-radius: var(--radius);
		padding: 1.5rem;
		text-align: center;
	}

	.title {
		margin: 0;
		font-weight: 600;
	}

	.hint {
		margin: 0.35rem 0 0;
		color: var(--muted);
	}
</style>
