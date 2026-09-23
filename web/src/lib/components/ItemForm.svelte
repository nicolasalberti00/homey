<script lang="ts" module>
	/** Editable fields shared by item creation and editing. */
	export type ItemDraft = {
		name: string;
		description: string;
		quantity: number;
		tags: string[];
		aliases: string[];
		notes: string;
	};
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { Item } from '$lib/api/client';

	// Reusable create/edit form: `initial` seeds the fields for editing, no
	// initial means an empty create form. The parent owns the API call and
	// passes `saving` back to disable the submit button.
	let {
		initial = null,
		saving = false,
		submitLabel = 'Create item',
		onSubmit,
		onCancel = undefined
	}: {
		initial?: Item | null;
		saving?: boolean;
		submitLabel?: string;
		onSubmit: (draft: ItemDraft) => void;
		onCancel?: (() => void) | undefined;
	} = $props();

	// The fields are seeded once from `initial` (`untrack`: a deliberate read of
	// the prop, the parent remounts the form when the item changes).
	let name = $state(untrack(() => initial?.name ?? ''));
	let description = $state(untrack(() => initial?.description ?? ''));
	let quantity = $state<number | undefined>(untrack(() => initial?.quantity ?? 1));
	let tags = $state(untrack(() => initial?.tags?.join(', ') ?? ''));
	let aliases = $state(untrack(() => initial?.aliases?.join(', ') ?? ''));
	let notes = $state(untrack(() => initial?.notes ?? ''));

	// The API takes arrays; the inputs are comma-separated lists.
	function parseList(value: string): string[] {
		return value
			.split(',')
			.map((entry) => entry.trim())
			.filter((entry) => entry !== '');
	}

	function submit(event: SubmitEvent) {
		event.preventDefault();
		const parsed = Number(quantity);
		onSubmit({
			name: name.trim(),
			description: description.trim(),
			quantity: Number.isFinite(parsed) ? Math.max(0, Math.round(parsed)) : 1,
			tags: parseList(tags),
			aliases: parseList(aliases),
			notes: notes.trim()
		});
	}
</script>

<form class="item-form" onsubmit={submit}>
	<div class="field">
		<label for="item-name">Name</label>
		<input id="item-name" bind:value={name} required maxlength="120" autocomplete="off" />
	</div>
	<div class="row">
		<div class="field qty">
			<label for="item-quantity">Quantity</label>
			<input
				id="item-quantity"
				type="number"
				bind:value={quantity}
				min="0"
				max="1000000"
				step="1"
				required
			/>
		</div>
		<div class="field grow">
			<label for="item-tags">Tags <span class="muted">(comma-separated)</span></label>
			<input id="item-tags" bind:value={tags} maxlength="900" autocomplete="off" />
		</div>
	</div>
	<div class="field">
		<label for="item-description">Description <span class="muted">(optional)</span></label>
		<input id="item-description" bind:value={description} maxlength="2000" autocomplete="off" />
	</div>
	<div class="field">
		<label for="item-aliases">Aliases <span class="muted">(comma-separated)</span></label>
		<input
			id="item-aliases"
			bind:value={aliases}
			maxlength="2400"
			autocomplete="off"
			aria-describedby="item-aliases-hint"
		/>
		<p id="item-aliases-hint" class="hint">Other names the item answers to, such as “giravite”.</p>
	</div>
	<div class="field">
		<label for="item-notes">Notes <span class="muted">(optional)</span></label>
		<textarea id="item-notes" bind:value={notes} maxlength="4000" rows="3"></textarea>
	</div>
	<div class="actions">
		<button class="btn primary" type="submit" disabled={saving}>
			{saving ? 'Saving…' : submitLabel}
		</button>
		{#if onCancel}
			<button class="btn" type="button" onclick={onCancel}>Cancel</button>
		{/if}
	</div>
</form>

<style>
	.field {
		margin-bottom: 0.75rem;
	}

	.row {
		display: flex;
		gap: 0.75rem;
	}

	.row .qty {
		width: 8rem;
	}

	.row .grow {
		flex: 1;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.hint {
		margin: 0.35rem 0 0;
		color: var(--muted);
		font-size: 0.85rem;
	}
</style>
