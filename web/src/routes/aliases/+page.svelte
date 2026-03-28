<script>
	import { onMount } from 'svelte';
	import { fetchAliases, createAlias, deleteAlias } from '$lib/api.js';

	let aliases = $state([]);
	let loading = $state(true);
	let error = $state(null);
	let showAdd = $state(false);
	let newAlias = $state('');
	let newCanonical = $state('');
	let addError = $state('');
	let toast = $state(null);

	onMount(async () => {
		try {
			aliases = (await fetchAliases()) || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	function showToast(msg) {
		toast = msg;
		setTimeout(() => (toast = null), 2500);
	}

	async function handleAdd() {
		addError = '';
		if (!newAlias.trim() || !newCanonical.trim()) {
			addError = 'Both fields required';
			return;
		}
		try {
			await createAlias(newAlias.trim(), newCanonical.trim());
			aliases = (await fetchAliases()) || [];
			showAdd = false;
			newAlias = '';
			newCanonical = '';
			showToast('Alias created');
		} catch (e) {
			addError = e.message;
		}
	}

	async function handleDelete(alias) {
		try {
			await deleteAlias(alias);
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
		<div class="empty-state">
			<span class="empty-icon">A</span>
			<p>No aliases defined</p>
			<p><code>vial label set ALIAS=KEY</code></p>
		</div>
	{:else}
		<div class="alias-list">
			{#each aliases as a, i}
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

{#if showAdd}
	<div class="modal-backdrop" onclick={() => (showAdd = false)} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
			<h3>Create Alias</h3>
			<div class="form-group">
				<label class="form-label" for="alias-name">Alias Name</label>
				<input
					id="alias-name"
					class="input input-mono"
					bind:value={newAlias}
					placeholder="OPENAI_KEY"
				/>
			</div>
			<div class="form-group">
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
		color: var(--gold);
		flex: 1;
	}
	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
