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

	let loading = $state(true);
	let error = $state(null);
	let showAdd = $state(false);
	let newKey = $state('');
	let newValue = $state('');
	let addError = $state('');
	let revealedKeys = $state({});
	let toast = $state(null);

	onMount(async () => {
		try {
			const data = await fetchSecrets();
			secrets.set(data || []);
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	function showToast(msg, type = 'success') {
		toast = { msg, type };
		setTimeout(() => (toast = null), 2500);
	}

	async function handleAdd() {
		addError = '';
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
			const data = await fetchSecrets();
			secrets.set(data || []);
			showAdd = false;
			newKey = '';
			newValue = '';
			showToast(`${newKey.trim()} added`);
		} catch (e) {
			addError = e.message;
		}
	}

	async function handleDelete(key) {
		if (!confirm(`Delete "${key}"?`)) return;
		try {
			await deleteSecret(key);
			secrets.update((s) => s.filter((sec) => sec.key !== key));
			showToast(`${key} deleted`);
		} catch (e) {
			error = e.message;
		}
	}

	async function handleReveal(key) {
		if (revealedKeys[key]) {
			revealedKeys = { ...revealedKeys, [key]: null };
			return;
		}
		try {
			const data = await revealSecret(key);
			revealedKeys = { ...revealedKeys, [key]: data.value };
		} catch (e) {
			showToast(`Failed to reveal: ${e.message}`, 'error');
		}
	}

	function mask(val) {
		if (!val || val.length <= 12) return '\u2022'.repeat(12);
		return val.substring(0, 4) + '\u2022'.repeat(6) + val.substring(val.length - 4);
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Secrets</h1>
			<p class="page-desc">
				{$filteredSecrets.length} key{$filteredSecrets.length !== 1 ? 's' : ''} in vault
			</p>
		</div>
		<div class="header-actions">
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
		<div class="empty-state">
			<span class="empty-icon">K</span>
			<p>No secrets stored</p>
			<p><code>vial key set NAME</code></p>
		</div>
	{:else}
		<div class="secret-list">
			{#each $filteredSecrets as secret, i}
				<div class="secret-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="secret-main">
						<span class="secret-key">{secret.key}</span>
						{#if secret.aliases && secret.aliases.length > 0}
							{#each secret.aliases as a}
								<span class="badge">{a}</span>
							{/each}
						{/if}
						{#if secret.tags && secret.tags.length > 0}
							{#each secret.tags as tag}
								<span class="badge badge-gold">{tag}</span>
							{/each}
						{/if}
					</div>
					<div class="secret-value">
						{#if revealedKeys[secret.key]}
							<code class="revealed">{revealedKeys[secret.key]}</code>
						{:else}
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
