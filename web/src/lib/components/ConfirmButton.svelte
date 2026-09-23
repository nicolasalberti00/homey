<script lang="ts">
	// Two-step destructive action: the first activation arms it, the second
	// performs it. Accessible by keyboard; the armed state is announced.
	let {
		label,
		confirmLabel = 'Confirm?',
		onConfirm,
		disabled = false
	}: {
		label: string;
		confirmLabel?: string;
		onConfirm: () => void;
		disabled?: boolean;
	} = $props();

	let armed = $state(false);

	function activate() {
		if (armed) {
			armed = false;
			onConfirm();
		} else {
			armed = true;
		}
	}
</script>

<button
	class="btn danger"
	type="button"
	onclick={activate}
	onblur={() => (armed = false)}
	{disabled}
	aria-label={armed ? `${confirmLabel} ${label}` : label}
>
	{armed ? confirmLabel : label}
</button>

{#if armed}
	<span class="armed" role="status">Click again to confirm.</span>
{/if}

<style>
	.armed {
		margin-left: 0.5rem;
		color: var(--muted);
		font-size: 0.9rem;
	}
</style>
