<script lang="ts">
	import type { Problem } from '$lib/api/client';

	let { problem, title = 'Something went wrong' }: { problem: Problem; title?: string } = $props();
</script>

<!-- role=alert: screen readers announce the failure when it appears. -->
<div class="problem" role="alert">
	<p class="heading">{title}</p>
	{#if problem.detail}
		<p>{problem.detail}</p>
	{:else if problem.title}
		<p>{problem.title}</p>
	{/if}
	{#if problem.errors?.length}
		<ul>
			{#each problem.errors as detail, index (detail.location ?? index)}
				<li>
					{#if detail.location}<code>{detail.location}</code>{/if}
					{detail.message}
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.problem {
		background: var(--danger-soft);
		border: 1px solid var(--danger);
		border-radius: var(--radius);
		color: var(--text);
		padding: 0.75rem 1rem;
	}

	.problem p {
		margin: 0.25rem 0;
	}

	.heading {
		color: var(--danger);
		font-weight: 600;
	}

	ul {
		margin: 0.25rem 0;
		padding-left: 1.25rem;
	}
</style>
