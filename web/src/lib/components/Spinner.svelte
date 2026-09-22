<script lang="ts">
	let { label = 'Loading…' }: { label?: string } = $props();
</script>

<p class="loading" role="status">
	<span class="dot" aria-hidden="true"></span>
	{label}
</p>

<style>
	.loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--muted);
	}

	.dot {
		width: 0.7rem;
		height: 0.7rem;
		border-radius: 50%;
		border: 2px solid var(--border);
		border-top-color: var(--accent);
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(1turn);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dot {
			animation: none;
		}
	}
</style>
