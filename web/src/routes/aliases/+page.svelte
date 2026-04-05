<!--
  aliases/+page.svelte — Alias management page.

  Aliases let users define alternate key names that the Vial matching engine
  resolves to canonical vault keys. For example, OPENAI_KEY → OPENAI_API_KEY.
  This is the matcher's Tier 3 (alias tier): it runs after exact and normalize
  matching, so aliases only fire when the requested key name doesn't already
  directly match a vault entry.

  This page provides:
    - A list view of all defined alias mappings (alias → canonical)
    - An "Add Alias" modal form to create new mappings
    - A per-row Remove button to delete mappings

  Data is fetched on mount and re-fetched after create operations to reflect
  any server-side normalisation of the submitted names. Delete operations use
  optimistic local state updates for instant UI response.
-->
<script>
	import { onMount } from 'svelte';
	import { fetchAliases, createAlias, deleteAlias } from '$lib/api.js';

	// ---------------------------------------------------------------------------
	// Page state (Svelte 5 runes)
	// ---------------------------------------------------------------------------

	/**
	 * The list of alias objects returned by GET /api/aliases.
	 * Each object has at minimum: { alias: string, canonical: string }.
	 * @type {Array<{ alias: string, canonical: string }>}
	 */
	let aliases = $state([]);

	/** True while the initial aliases fetch is in progress. */
	let loading = $state(true);

	/** Page-level error string displayed below the header on fetch failure. */
	let error = $state(null);

	/** Controls visibility of the "Create Alias" modal. */
	let showAdd = $state(false);

	/** Form field: the alias key name (the variant that will be requested). */
	let newAlias = $state('');

	/** Form field: the canonical vault key the alias should resolve to. */
	let newCanonical = $state('');

	/** Validation / server error shown inside the modal. */
	let addError = $state('');

	/**
	 * Transient success message shown after create/delete operations.
	 * @type {string | null}
	 */
	let toast = $state(null);

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------

	onMount(async () => {
		try {
			aliases = /** @type {any} */ (await fetchAliases()) || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------

	/**
	 * Shows a success toast for 2.5 s then clears it.
	 * @param {string} msg
	 */
	function showToast(msg) {
		toast = msg;
		setTimeout(() => (toast = null), 2500);
	}

	// ---------------------------------------------------------------------------
	// Handlers
	// ---------------------------------------------------------------------------

	/**
	 * Validates both alias form fields then submits to the API. On success,
	 * re-fetches the full alias list (to capture any server normalisation),
	 * closes the modal, and resets the form fields.
	 */
	async function handleAdd() {
		addError = '';

		// Both fields are required — the API also enforces this, but validating
		// client-side avoids an unnecessary round trip.
		if (!newAlias.trim() || !newCanonical.trim()) {
			addError = 'Both fields required';
			return;
		}

		try {
			await createAlias(newAlias.trim(), newCanonical.trim());
			// Re-fetch rather than optimistically pushing to the array, so the
			// list reflects any canonical key normalisation done server-side.
			aliases = /** @type {any} */ (await fetchAliases()) || [];
			showAdd = false;
			newAlias = '';
			newCanonical = '';
			showToast('Alias created');
		} catch (e) {
			addError = e.message;
		}
	}

	/**
	 * Deletes an alias by its alias name (the variant, not the canonical key).
	 * Uses an optimistic local filter so the row disappears immediately without
	 * waiting for a re-fetch.
	 *
	 * @param {string} alias - The alias key name to remove.
	 */
	async function handleDelete(alias) {
		try {
			await deleteAlias(alias);
			// Optimistic update: filter locally to avoid a full re-fetch round trip.
			aliases = aliases.filter((a) => a.alias !== alias);
			showToast(`${alias} removed`);
		} catch (e) {
			error = e.message;
		}
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Aliases</h1>
			<p class="page-desc">Map variant key names to canonical vault keys</p>
		</div>
		<button class="btn btn-primary" onclick={() => (showAdd = true)}>Add Alias</button>
	</div>

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading...</span>
		</div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if aliases.length === 0}
		<!-- Empty state: includes the CLI command the user can run to create their
		     first alias outside the dashboard. -->
		<div class="empty-state">
			<span class="empty-icon">A</span>
			<p>No aliases defined</p>
			<p><code>vial label set ALIAS=KEY</code></p>
		</div>
	{:else}
		<div class="alias-list">
			{#each aliases as a, i}
				<!-- Arrow (→) between alias and canonical visually reinforces the
				     directional mapping: requests for .alias resolve to .canonical. -->
				<div class="alias-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<span class="alias-from">{a.alias}</span>
					<span class="arrow">&rarr;</span>
					<span class="alias-to">{a.canonical}</span>
					<button class="btn btn-sm btn-danger" onclick={() => handleDelete(a.alias)}>Remove</button
					>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Create Alias modal. Backdrop click closes; inner dialog click stops
     propagation to prevent the backdrop handler from firing. -->
{#if showAdd}
	<div class="modal-backdrop" onclick={() => (showAdd = false)} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
			<h3>Create Alias</h3>
			<div class="form-group">
				<!-- "Alias Name" is the variant key that tools or .env.example files
				     will request (e.g. OPENAI_KEY). -->
				<label class="form-label" for="alias-name">Alias Name</label>
				<input
					id="alias-name"
					class="input input-mono"
					bind:value={newAlias}
					placeholder="OPENAI_KEY"
				/>
			</div>
			<div class="form-group">
				<!-- "Maps To" is the canonical key that exists in the vault.
				     The server will reject it if this key does not exist. -->
				<label class="form-label" for="canonical-key">Maps To (vault key)</label>
				<input
					id="canonical-key"
					class="input input-mono"
					bind:value={newCanonical}
					placeholder="OPENAI_API_KEY"
				/>
			</div>
			{#if addError}
				<p class="error-text" style="font-size: 0.82rem;">{addError}</p>
			{/if}
			<div class="modal-actions">
				<button class="btn" onclick={() => (showAdd = false)}>Cancel</button>
				<button class="btn btn-primary" onclick={handleAdd}>Create</button>
			</div>
		</div>
	</div>
{/if}

<!-- Success toast — only 'success' type needed on this page since errors are
     surfaced inline rather than via the toast mechanism. -->
{#if toast}
	<div class="toast success">{toast}</div>
{/if}

<style>
	.page {
		max-width: 700px;
	}
	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 1.5rem;
	}
	h1 {
		font-size: 1.2rem;
		font-weight: 600;
		color: var(--text-bright);
	}
	.page-desc {
		font-size: 0.78rem;
		color: var(--text-muted);
		margin-top: 0.2rem;
	}

	.alias-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.alias-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.65rem 1rem;
	}
	.alias-from {
		font-family: var(--font-mono);
		font-size: 0.82rem;
		color: var(--text-secondary);
	}
	.arrow {
		color: var(--text-muted);
		font-size: 0.85rem;
	}
	.alias-to {
		font-family: var(--font-mono);
		font-size: 0.82rem;
		/* Gold colour distinguishes the canonical target from the alias source,
		   matching the visual language used for tags elsewhere in the dashboard. */
		color: var(--gold);
		flex: 1;
	}
	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
