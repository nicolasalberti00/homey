<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import type { Snippet } from 'svelte';

	import '../app.css';

	let { children }: { children: Snippet } = $props();

	// Literal routes keep resolve() type-checked; the nav grows with the
	// coming steps.
	const home = resolve('/');
	const rooms = resolve('/rooms');
	const settings = resolve('/settings');

	function isCurrent(href: string): boolean {
		const current = page.url.pathname;
		return href === home ? current === home : current.startsWith(href);
	}
</script>

<!-- In-page anchor, not a SvelteKit route: no resolve() needed. -->
<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
<a class="skip" href="#main">Skip to content</a>

<div class="shell">
	<aside class="sidebar">
		<p class="brand">
			<span class="logo" aria-hidden="true">▣</span>
			homey
		</p>
		<nav aria-label="Main">
			<a
				href={resolve('/')}
				class:active={isCurrent(home)}
				aria-current={isCurrent(home) ? 'page' : undefined}
			>
				Dashboard
			</a>
			<a
				href={resolve('/rooms')}
				class:active={isCurrent(rooms)}
				aria-current={isCurrent(rooms) ? 'page' : undefined}
			>
				Rooms
			</a>
			<a
				href={resolve('/settings')}
				class:active={isCurrent(settings)}
				aria-current={isCurrent(settings) ? 'page' : undefined}
			>
				Settings
			</a>
		</nav>
		<div class="footer">
			<ThemeToggle />
		</div>
	</aside>
	<main id="main" tabindex="-1">
		{@render children()}
	</main>
</div>

<style>
	.skip {
		position: absolute;
		left: -100vw;
		top: 0;
		background: var(--surface);
		padding: 0.5rem 0.75rem;
		z-index: 10;
	}

	.skip:focus {
		left: 0;
	}

	.shell {
		display: grid;
		grid-template-columns: 15rem 1fr;
		min-height: 100dvh;
	}

	.sidebar {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1rem;
		background: var(--surface);
		border-right: 1px solid var(--border);
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0;
		font-size: 1.15rem;
		font-weight: 700;
	}

	.logo {
		color: var(--accent);
	}

	nav {
		display: flex;
		flex-direction: column;
		flex: 1;
		gap: 0.25rem;
	}

	nav a {
		border-radius: 8px;
		color: var(--text);
		padding: 0.45rem 0.6rem;
		text-decoration: none;
	}

	nav a:hover {
		background: var(--surface-2);
		text-decoration: none;
	}

	nav a.active {
		background: var(--accent-soft);
		color: var(--accent);
		font-weight: 600;
	}

	main {
		max-width: 64rem;
		padding: 1.5rem;
		width: 100%;
	}

	main:focus {
		outline: none;
	}

	@media (max-width: 720px) {
		.shell {
			grid-template-columns: 1fr;
		}

		.sidebar {
			flex-direction: row;
			align-items: center;
			gap: 0.75rem;
			border-right: 0;
			border-bottom: 1px solid var(--border);
			overflow-x: auto;
		}

		nav {
			flex-direction: row;
			flex: 0 1 auto;
		}

		.footer {
			margin-left: auto;
		}

		main {
			padding: 1rem;
		}
	}
</style>
