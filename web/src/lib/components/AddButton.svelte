<script lang="ts">
	// Small toggle next to a section title that reveals an inline form. The
	// form stays out of the way until it is asked for, and the button works
	// with keyboard and touch (unlike a hover-only affordance).
	let {
		label,
		expanded = false,
		controls,
		onToggle
	}: {
		label: string;
		expanded?: boolean;
		controls: string;
		onToggle: () => void;
	} = $props();
</script>

<button
	type="button"
	class="add"
	aria-label={label}
	title={label}
	aria-expanded={expanded}
	aria-controls={controls}
	onclick={onToggle}
>
	<span aria-hidden="true">{expanded ? '−' : '+'}</span>
</button>

<style>
	.add {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1.8rem;
		height: 1.8rem;
		padding: 0;
		font: inherit;
		font-size: 1.05rem;
		line-height: 1;
		color: var(--text);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 999px;
		cursor: pointer;
	}

	.add:hover {
		color: var(--accent);
		background: var(--accent-soft);
		border-color: var(--accent);
	}
</style>
