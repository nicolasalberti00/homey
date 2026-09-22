<script lang="ts">
	import { currentTheme, cycleTheme, type Theme } from '$lib/theme.svelte';

	const labels: Record<Theme, string> = { light: 'Light', dark: 'Dark', system: 'System' };
	const icons: Record<Theme, string> = { light: '☀', dark: '☾', system: '⌂' };
	const next: Record<Theme, string> = { light: 'dark', dark: 'system', system: 'light' };
</script>

<!-- The visible label is the accessible name; the title explains the action. -->
<button
	class="btn toggle"
	type="button"
	onclick={cycleTheme}
	title="Switch to {next[currentTheme()]} theme"
>
	<span aria-hidden="true">{icons[currentTheme()]}</span>
	<span>Theme: {labels[currentTheme()]}</span>
</button>

<style>
	.toggle {
		width: 100%;
		justify-content: flex-start;
	}

	@media (max-width: 720px) {
		.toggle {
			width: auto;
		}

		.toggle span:last-child {
			display: none;
		}
	}
</style>
