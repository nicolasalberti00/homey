<script lang="ts">
	import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import { getLastLocation, rememberLocation } from '$lib/last-location';
	import type { Snippet } from 'svelte';

	import '../app.css';

	let { children }: { children: Snippet } = $props();

	// Literal routes keep resolve() type-checked; the nav grows with the
	// coming steps.
	const home = resolve('/');
	const rooms = resolve('/rooms');
	const settings = resolve('/settings');

	type Section = {
		id: string;
		label: string;
		href: string;
		matches: (path: string) => boolean;
	};

	const sections: Section[] = [
		{ id: 'dashboard', label: 'Dashboard', href: home, matches: (path) => path === home },
		{
			id: 'rooms',
			label: 'Rooms',
			href: rooms,
			matches: (path) => path === rooms || path.startsWith(`${rooms}/`)
		},
		{
			id: 'settings',
			label: 'Settings',
			href: settings,
			matches: (path) => path === settings || path.startsWith(`${settings}/`)
		}
	];

	function sectionOf(path: string): Section | undefined {
		return sections.find((section) => section.matches(path));
	}

	function currentLocation(): string {
		return page.url.pathname + page.url.search;
	}

	// Scroll offset to apply once the page a sidebar click resumes renders,
	// keyed by the target so a superseded navigation cannot consume it.
	let pending: { path: string; y: number } | null = null;

	function rememberHere(): void {
		const section = sectionOf(page.url.pathname);
		if (section) rememberLocation(section.id, currentLocation(), window.scrollY);
	}

	function onVisibilityChange(): void {
		if (document.visibilityState === 'hidden') rememberHere();
	}

	// Keep each section's entry fresh: before leaving a page, when the tab goes
	// away (reload/close) and after a navigation that changed the page.
	beforeNavigate(rememberHere);

	afterNavigate(() => {
		const section = sectionOf(page.url.pathname);
		if (!section) return;

		if (pending) {
			const { path, y } = pending;
			pending = null;
			if (path === currentLocation()) {
				rememberLocation(section.id, path, y);
				restoreScroll(y);
				return;
			}
		}

		// Back/forward and reloads are restored by SvelteKit: keep the stored
		// position instead of overwriting it with the transient one.
		if (getLastLocation(section.id)?.path === currentLocation()) return;
		rememberLocation(section.id, currentLocation(), window.scrollY);
	});

	// A click on an inactive section resumes its last page; a click on the
	// active one goes to the root (always a one-click way out).
	function followSection(event: MouseEvent, section: Section): void {
		if (event.defaultPrevented || event.button !== 0) return;
		if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
		if (section.matches(page.url.pathname)) return;

		const stored = getLastLocation(section.id);
		if (!stored) return;

		event.preventDefault();
		pending = { path: stored.path, y: stored.y };
		// The stored path is a URL pathname, not a route id: resolve() cannot
		// rewrite it. The entry was produced by a resolve()-based href.
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		goto(stored.path).catch(() => {
			if (pending?.path === stored.path) pending = null;
		});
	}

	// The resumed page may still be loading its data: wait (briefly) until the
	// document is tall enough for the stored offset.
	function restoreScroll(y: number): void {
		const target = currentLocation();
		const deadline = performance.now() + 1000;
		const attempt = () => {
			if (currentLocation() !== target) return; // navigated away meanwhile
			const max = document.documentElement.scrollHeight - window.innerHeight;
			if (max >= y || performance.now() >= deadline) {
				window.scrollTo(0, y);
				return;
			}
			requestAnimationFrame(attempt);
		};
		requestAnimationFrame(attempt);
	}
</script>

<!-- In-page anchor, not a SvelteKit route: no resolve() needed. -->
<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
<a class="skip" href="#main">Skip to content</a>

<!-- Flush the remembered location when the tab is reloaded/closed or hidden. -->
<svelte:window onpagehide={rememberHere} />
<svelte:document onvisibilitychange={onVisibilityChange} />

<div class="shell">
	<aside class="sidebar">
		<p class="brand">
			<span class="logo" aria-hidden="true">▣</span>
			homey
		</p>
		<nav aria-label="Main">
			<!-- The hrefs are resolve() outputs kept in `sections`; the rule cannot
			     trace them through the array. -->
			<!-- eslint-disable svelte/no-navigation-without-resolve -->
			{#each sections as section (section.id)}
				{@const active = section.matches(page.url.pathname)}
				<a
					href={section.href}
					class:active
					aria-current={active ? 'page' : undefined}
					onclick={(event) => followSection(event, section)}
				>
					{section.label}
				</a>
			{/each}
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
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
		/* Keep the nav reachable while the content scrolls: sticky inside the
		   grid area, full viewport height, its own scroll if it ever overflows. */
		position: sticky;
		top: 0;
		align-self: start;
		height: 100dvh;
		overflow-y: auto;
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
			/* Sticky top bar on narrow screens: only as tall as its content. */
			height: auto;
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
