<!--
  +page.svelte — Secrets page (dashboard home).

  This is the primary view of the Vial dashboard. It shows all secrets stored
  in the encrypted vault and allows the user to:
    - Search secrets by key name (live filter via the vault store's searchQuery)
    - Filter by tag (tag pills backed by the vault store's activeTagFilter)
    - Reveal / hide individual secret values (fetches plaintext on demand)
    - Add new secrets via a modal form
    - Delete existing secrets with a confirmation prompt

  Secret values are NEVER stored in client memory in bulk — the `secrets` store
  holds only key names and metadata. Plaintext values are fetched one at a time
  via revealSecret() and held in `revealedKeys` (a local $state map) only for
  the duration of the current page session. Hiding a secret nulls its entry
  in that map, allowing the value to be garbage-collected.

  The masked display uses bullet characters (•) rather than CSS
  `visibility: hidden` so screen readers do not accidentally read aloud a real
  value from the DOM.
-->
<script>
	import { onMount } from 'svelte';
	import { fetchSecrets, deleteSecret, createSecret, revealSecret } from '$lib/api.js';
	import {
		secrets,
		searchQuery,
		filteredSecrets,
		allTags,
		activeTagFilter,
	} from '$lib/stores/vault.js';

	// ---------------------------------------------------------------------------
	// Page state (Svelte 5 runes)
	// ---------------------------------------------------------------------------

	/** True while the initial secrets fetch is in progress. */
	let loading = $state(true);

	/** Holds the error message string when the page-level fetch fails. */
	let error = $state(null);

	/** Controls visibility of the "Add Secret" modal. */
	let showAdd = $state(false);

	/** Form field: new secret key name. */
	let newKey = $state('');

	/** Form field: new secret value. Bound to a password input so it is
	 *  not visible in the DOM as plaintext. */
	let newValue = $state('');

	/** Validation / server error message shown inside the Add Secret modal. */
	let addError = $state('');

	/**
	 * Map of key → plaintext value for secrets the user has chosen to reveal.
	 * Entries are nulled (not deleted) on hide so that Svelte's reactivity
	 * still detects the change. Using a plain object rather than a Map because
	 * Svelte's fine-grained reactivity tracks object property access.
	 *
	 * @type {{ [key: string]: string | null }}
	 */
	let revealedKeys = $state({});

	/**
	 * Transient notification shown in the bottom-right corner after mutations.
	 * Automatically cleared after 2.5 s via setTimeout.
	 * @type {{ msg: string, type: 'success' | 'error' } | null}
	 */
	let toast = $state(null);

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------

	onMount(async () => {
		try {
			const data = await fetchSecrets();
			// Populate the shared vault store so derived stores (filteredSecrets,
			// allTags) immediately recompute from the loaded data.
			secrets.set(/** @type {any} */ (data) || []);
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
	 * Displays a transient toast notification then clears it after 2.5 s.
	 * @param {string} msg - Human-readable message.
	 * @param {'success' | 'error'} [type='success'] - Controls toast colour.
	 */
	function showToast(msg, type = 'success') {
		toast = { msg, type };
		setTimeout(() => (toast = null), 2500);
	}

	// ---------------------------------------------------------------------------
	// Handlers
	// ---------------------------------------------------------------------------

	/**
	 * Validates the Add Secret form fields, submits to the API, re-fetches the
	 * full secret list to pick up server-side metadata (tags, aliases), then
	 * closes the modal and resets the form.
	 *
	 * Re-fetching after create (rather than optimistically pushing to the store)
	 * ensures any server-side tagging or aliasing is reflected immediately.
	 */
	async function handleAdd() {
		addError = '';

		// Client-side validation before the network round trip.
		if (!newKey.trim()) {
			addError = 'Key name is required';
			return;
		}
		if (!newValue.trim()) {
			addError = 'Value is required';
			return;
		}

		try {
			await createSecret(newKey.trim(), newValue.trim());
			// Re-fetch so the list includes server-generated metadata.
			const data = await fetchSecrets();
			secrets.set(/** @type {any} */ (data) || []);
			showAdd = false;
			// Capture the trimmed key before resetting state for the toast message.
			newKey = '';
			newValue = '';
			showToast(`${newKey.trim()} added`);
		} catch (e) {
			addError = e.message;
		}
	}

	/**
	 * Asks for confirmation then deletes the secret. On success, optimistically
	 * removes the entry from the store to avoid a full re-fetch round trip.
	 *
	 * @param {string} key - The vault key name to delete.
	 */
	async function handleDelete(key) {
		if (!confirm(`Delete "${key}"?`)) return;
		try {
			await deleteSecret(key);
			// Optimistic update: filter out the deleted key immediately so the
			// UI feels instantaneous without waiting for a server round trip.
			secrets.update((s) => s.filter((sec) => sec.key !== key));
			showToast(`${key} deleted`);
		} catch (e) {
			error = e.message;
		}
	}

	/**
	 * Toggles the plaintext reveal state for a single secret. If the key is
	 * already revealed, nulls the cached value (hides it). Otherwise, fetches
	 * the plaintext from the server and stores it in `revealedKeys`.
	 *
	 * A spread is used to update revealedKeys (`{ ...revealedKeys, [key]: val }`)
	 * because Svelte 5's fine-grained reactivity requires a new object reference
	 * to detect property-level changes on a $state object.
	 *
	 * @param {string} key - Vault key name.
	 */
	async function handleReveal(key) {
		if (revealedKeys[key]) {
			// Already revealed — null out to hide. Spread creates a new reference
			// so Svelte detects the change.
			revealedKeys = { ...revealedKeys, [key]: null };
			return;
		}
		try {
			const data = /** @type {any} */ (await revealSecret(key));
			revealedKeys = { ...revealedKeys, [key]: data.value };
		} catch (e) {
			showToast(`Failed to reveal: ${e.message}`, 'error');
		}
	}

	/**
	 * Returns a masked representation of a secret value for display. Short or
	 * absent values get a fixed-length bullet string so no information about
	 * value length is leaked. Longer values show the first and last 4 characters
	 * to help users identify the secret without exposing the full value.
	 *
	 * The bullet character (U+2022) is used rather than '*' to clearly signal
	 * that this is a masked placeholder, not the actual value.
	 *
	 * @param {string | null | undefined} val - The plaintext value (not used
	 *   here since we intentionally never store values in the list, but the
	 *   parameter is kept for consistency with potential future use).
	 * @returns {string} Masked display string.
	 */
	function mask(val) {
		if (!val || val.length <= 12) return '\u2022'.repeat(12);
		return val.substring(0, 4) + '\u2022'.repeat(6) + val.substring(val.length - 4);
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Secrets</h1>
			<!-- Reactive count pulled from the filtered store, not the full list,
			     so it accurately reflects the current search/tag filter state. -->
			<p class="page-desc">
				{$filteredSecrets.length} key{$filteredSecrets.length !== 1 ? 's' : ''} in vault
			</p>
		</div>
		<div class="header-actions">
			<!-- Two-way bound to the searchQuery store; any change triggers
			     filteredSecrets to recompute via the derived store. -->
			<input
				class="input"
				type="text"
				placeholder="Filter..."
				bind:value={$searchQuery}
				style="width: 180px;"
			/>
			<button class="btn btn-primary" onclick={() => (showAdd = true)}>Add Secret</button>
		</div>
	</div>

	<!-- Tag filter pills — only rendered when at least one secret has tags.
	     "All" button clears the activeTagFilter; individual tag buttons set it. -->
	{#if $allTags.length > 0}
		<div class="tags">
			<button
				class="tag"
				class:active={$activeTagFilter === null}
				onclick={() => activeTagFilter.set(null)}>All</button
			>
			{#each $allTags as tag}
				<button
					class="tag"
					class:active={$activeTagFilter === tag}
					onclick={() => activeTagFilter.set(tag)}>{tag}</button
				>
			{/each}
		</div>
	{/if}

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading...</span>
		</div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if $filteredSecrets.length === 0}
		<!-- Empty state: shown both when the vault has no secrets AND when the
		     current filter produces zero results. The CLI hint helps new users. -->
		<div class="empty-state">
			<span class="empty-icon">K</span>
			<p>No secrets stored</p>
			<p><code>vial key set NAME</code></p>
		</div>
	{:else}
		<div class="secret-list">
			{#each $filteredSecrets as secret, i}
				<!-- stagger-N classes apply CSS animation-delay stepping so rows
				     cascade in rather than all appearing at once. Capped at 10
				     to avoid generating unbounded class names. -->
				<div class="secret-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="secret-main">
						<span class="secret-key">{secret.key}</span>
						<!-- Alias badges: secondary key names that resolve to this secret
						     via the matcher's alias tier. -->
						{#if secret.aliases && secret.aliases.length > 0}
							{#each secret.aliases as a}
								<span class="badge">{a}</span>
							{/each}
						{/if}
						<!-- Tag badges: shown in gold to visually distinguish from aliases. -->
						{#if secret.tags && secret.tags.length > 0}
							{#each secret.tags as tag}
								<span class="badge badge-gold">{tag}</span>
							{/each}
						{/if}
					</div>
					<div class="secret-value">
						{#if revealedKeys[secret.key]}
							<!-- Plaintext value: highlighted in gold background to draw
							     attention to the fact that a real value is visible. -->
							<code class="revealed">{revealedKeys[secret.key]}</code>
						{:else}
							<!-- Masked placeholder: passes empty string to mask() which
							     always returns a fixed-length bullet string when no value
							     is provided, preventing any length inference. -->
							<span class="masked">{mask('')}</span>
						{/if}
					</div>
					<div class="secret-actions">
						<button class="btn btn-sm" onclick={() => handleReveal(secret.key)}>
							{revealedKeys[secret.key] ? 'Hide' : 'Reveal'}
						</button>
						<button class="btn btn-sm btn-danger" onclick={() => handleDelete(secret.key)}>
							Delete
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Add Secret modal — rendered outside the page div so it sits above all
     other content in the stacking context. Clicking the backdrop closes
     the modal; clicking inside the dialog stops propagation to prevent
     that same click from triggering the backdrop handler. -->
{#if showAdd}
	<div class="modal-backdrop" onclick={() => (showAdd = false)} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
			<h3>Add Secret</h3>
			<div class="form-group">
				<label class="form-label" for="new-key">Key Name</label>
				<input
					id="new-key"
					class="input input-mono"
					bind:value={newKey}
					placeholder="OPENAI_API_KEY"
				/>
			</div>
			<div class="form-group">
				<label class="form-label" for="new-value">Value</label>
				<!-- type="password" prevents the browser from displaying the value
				     in plaintext and disables autofill suggestions for this field. -->
				<input
					id="new-value"
					class="input input-mono"
					type="password"
					bind:value={newValue}
					placeholder="sk-..."
				/>
			</div>
			{#if addError}
				<p class="error-text" style="font-size: 0.82rem; margin-top: 0.5rem;">{addError}</p>
			{/if}
			<div class="modal-actions">
				<button class="btn" onclick={() => (showAdd = false)}>Cancel</button>
				<button class="btn btn-primary" onclick={handleAdd}>Add</button>
			</div>
		</div>
	</div>
{/if}

<!-- Toast notification: floats in the bottom-right corner (positioned by
     global app.css rules). The type class ('success' or 'error') drives the
     background colour. Automatically disappears after 2.5 s. -->
{#if toast}
	<div class="toast {toast.type}">{toast.msg}</div>
{/if}

<style>
	.page {
		max-width: 800px;
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
	.header-actions {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.tags {
		display: flex;
		gap: 0.35rem;
		margin-bottom: 1.25rem;
		flex-wrap: wrap;
	}
	.tag {
		font-size: 0.75rem;
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		background: var(--bg-raised);
		border: 1px solid var(--border);
		color: var(--text-secondary);
		cursor: pointer;
		transition: all var(--transition);
	}
	.tag:hover {
		color: var(--text);
		border-color: var(--text-muted);
	}
	.tag.active {
		background: var(--purple-muted);
		border-color: var(--purple-dark);
		color: var(--purple-light);
	}

	.secret-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.secret-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1rem;
	}
	.secret-main {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}
	.secret-key {
		font-family: var(--font-mono);
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--text-bright);
		white-space: nowrap;
	}
	.secret-value {
		font-family: var(--font-mono);
		font-size: 0.78rem;
		color: var(--text-muted);
	}
	.revealed {
		font-size: 0.78rem;
		background: var(--gold-muted);
		word-break: break-all;
	}
	.masked {
		letter-spacing: 0.1em;
	}
	.secret-actions {
		display: flex;
		gap: 0.35rem;
	}

	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
