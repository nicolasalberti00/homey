// Connection settings (Step 4.3): API base URL and bearer token, persisted
// in localStorage and read by the API client on every request.

export type Settings = {
	/** Base URL of the homey server; empty means same origin. */
	apiUrl: string;
	/** Bearer token for /api/v1 operations. */
	token: string;
};

const STORAGE_KEY = 'homey.settings';

function stored(): Settings {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return { apiUrl: '', token: '' };
		const parsed = JSON.parse(raw) as Partial<Settings>;
		return {
			apiUrl: typeof parsed.apiUrl === 'string' ? parsed.apiUrl : '',
			token: typeof parsed.token === 'string' ? parsed.token : ''
		};
	} catch {
		return { apiUrl: '', token: '' };
	}
}

let current = $state<Settings>(stored());

export function getSettings(): Settings {
	return current;
}

export function saveSettings(next: Settings): void {
	current = {
		apiUrl: next.apiUrl.trim().replace(/\/+$/, ''),
		token: next.token.trim()
	};
	localStorage.setItem(STORAGE_KEY, JSON.stringify(current));
}
