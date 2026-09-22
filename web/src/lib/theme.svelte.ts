// Theme state (Step 4.2): light, dark or system, persisted in localStorage.
// The palette follows `color-scheme` (see app.css), so "system" simply keeps
// both and lets the OS decide.

export type Theme = 'light' | 'dark' | 'system';

const STORAGE_KEY = 'homey.theme';

function stored(): Theme {
	const value = localStorage.getItem(STORAGE_KEY);
	return value === 'light' || value === 'dark' ? value : 'system';
}

let current = $state<Theme>(stored());

export function currentTheme(): Theme {
	return current;
}

export function setTheme(theme: Theme): void {
	current = theme;
	localStorage.setItem(STORAGE_KEY, theme);
	apply(theme);
}

export function cycleTheme(): void {
	setTheme(current === 'light' ? 'dark' : current === 'dark' ? 'system' : 'light');
}

function apply(theme: Theme): void {
	if (theme === 'system') {
		delete document.documentElement.dataset.theme;
	} else {
		document.documentElement.dataset.theme = theme;
	}
}
