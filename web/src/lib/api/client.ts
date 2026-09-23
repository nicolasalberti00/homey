// Typed API client (Step 4.3).
//
// Request and response types come from the generated OpenAPI schema
// (`schema.d.ts`, regenerated with `npm run api:generate` and verified in
// CI), so the UI contract cannot drift from the Go server's.

import { getSettings, type Settings } from '$lib/settings.svelte';
import type { components } from './schema';

/** ErrorDetail mirrors the RFC 9457 field-level details the API returns. */
export type ErrorDetail = components['schemas']['ErrorDetail'];

/** Problem is the part of the RFC 9457 problem document the UI renders. */
export type Problem = {
	title: string;
	status: number;
	detail?: string;
	errors?: ErrorDetail[] | null;
	/** Seconds to wait before retrying, from Retry-After on 429 responses. */
	retryAfter?: number;
};

export type Room = components['schemas']['RoomResponse'];
export type Container = components['schemas']['ContainerResponse'];
export type ContainerDetail = components['schemas']['ContainerDetail'];
export type Item = components['schemas']['ItemResponse'];
export type SearchMatch = components['schemas']['SearchMatch'];
export type SearchResult = components['schemas']['SearchResult'];
export type LocationRef = components['schemas']['LocationRef'];

/** ApiError carries the RFC 9457 problem document of a failed request. */
export class ApiError extends Error {
	readonly status: number;
	readonly problem: Problem;

	constructor(status: number, problem: Problem) {
		super(problem.detail ?? problem.title ?? `HTTP ${status}`);
		this.name = 'ApiError';
		this.status = status;
		this.problem = problem;
	}
}

/** Normalizes any thrown value into the problem document the UI renders. */
export function toProblem(error: unknown, title: string): Problem {
	return error instanceof ApiError ? error.problem : { title, status: 0, detail: String(error) };
}

type FetchOptions = {
	method?: string;
	body?: unknown;
	/** Overrides the stored settings; used by "test connection" flows. */
	settings?: Settings;
	/** Aborts an in-flight request, so a newer one can replace it. */
	signal?: AbortSignal;
};

export async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
	const settings = options.settings ?? getSettings();
	const headers = new Headers({ Accept: 'application/json' });
	if (options.body !== undefined) headers.set('Content-Type', 'application/json');
	if (settings.token !== '') headers.set('Authorization', `Bearer ${settings.token}`);

	const response = await fetch(`${settings.apiUrl}${path}`, {
		method: options.method ?? 'GET',
		headers,
		signal: options.signal,
		body: options.body === undefined ? undefined : JSON.stringify(options.body)
	});
	if (!response.ok) {
		throw new ApiError(response.status, await readProblem(response));
	}
	if (response.status === 204) {
		return undefined as T;
	}
	return (await response.json()) as T;
}

async function readProblem(response: Response): Promise<Problem> {
	// Both rate limiters answer 429 with Retry-After (delta-seconds); keep the
	// wait so the UI can say when the request may be retried.
	const retryAfter = parseRetryAfter(response.headers.get('Retry-After'));
	try {
		const problem = (await response.json()) as Partial<Problem>;
		return {
			title: problem.title ?? response.statusText,
			status: problem.status ?? response.status,
			detail: problem.detail,
			errors: problem.errors,
			retryAfter
		};
	} catch {
		return { title: response.statusText, status: response.status, retryAfter };
	}
}

/** parseRetryAfter reads the delta-seconds form; HTTP-date values are ignored. */
function parseRetryAfter(header: string | null): number | undefined {
	if (header === null) return undefined;
	const seconds = Number.parseInt(header, 10);
	return Number.isFinite(seconds) && seconds > 0 ? seconds : undefined;
}

/** Verifies reachability and credentials; returns a human-readable summary. */
export async function testConnection(settings: Settings): Promise<string> {
	const health = await fetch(`${settings.apiUrl}/healthz`);
	if (!health.ok) {
		throw new Error(`Health check failed: HTTP ${health.status}`);
	}
	const rooms = await apiFetch<Room[] | null>('/api/v1/rooms', { settings });
	const count = rooms?.length ?? 0;
	return `Connected — ${count} room${count === 1 ? '' : 's'} visible.`;
}
