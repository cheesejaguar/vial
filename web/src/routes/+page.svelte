<script>
	import { onMount } from 'svelte';
	import { fetchSecrets, deleteSecret } from '$lib/api.js';
	import { secrets, searchQuery, filteredSecrets, allTags, activeTagFilter } from '$lib/stores/vault.js';

	let loading = $state(true);
	let error = $state(null);

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

	function maskValue(val) {
		if (!val || val.length <= 12) return '••••••••';
		return val.substring(0, 4) + '•••' + val.substring(val.length - 4);
	}

	async function handleDelete(key) {
		if (!confirm(`Delete "${key}" from vault?`)) return;
		try {
			await deleteSecret(key);
			secrets.update(s => s.filter(sec => sec.key !== key));
		} catch (e) {
			error = e.message;
		}
	}
</script>

<div class="page">
	<div class="header">
		<h1>🔑 Secrets</h1>
		<div class="search">
			<span class="search-icon">🔍</span>
			<input
				type="text"
				placeholder="Search keys..."
				bind:value={$searchQuery}
			/>
		</div>
	</div>

	{#if $allTags.length > 0}
		<div class="tags">
			<button
				class="tag"
				class:active={$activeTagFilter === null}
				onclick={() => activeTagFilter.set(null)}
			>All</button>
			{#each $allTags as tag}
				<button
					class="tag"
					class:active={$activeTagFilter === tag}
					onclick={() => activeTagFilter.set(tag)}
				>{tag}</button>
			{/each}
		</div>
	{/if}

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Unlocking secrets...</span>
		</div>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if $filteredSecrets.length === 0}
		<div class="empty-state animate-in">
			<span class="empty-icon">🔐</span>
			<p class="empty-text">No secrets found.</p>
			<p class="empty-hint">Add secrets with <code>vial key set NAME</code></p>
		</div>
	{:else}
		<div class="secret-list">
			{#each $filteredSecrets as secret, i}
				<div class="secret-card card-glow animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="secret-header">
						<span class="secret-key mono">{secret.key}</span>
						<div class="secret-actions">
							<button class="btn-sm btn-danger" onclick={() => handleDelete(secret.key)}>
								🗑️ Delete
							</button>
						</div>
					</div>
					<div class="secret-meta">
						{#if secret.aliases && secret.aliases.length > 0}
							<span class="meta-item">🏷️ {secret.aliases.join(', ')}</span>
						{/if}
						{#if secret.tags && secret.tags.length > 0}
							<span class="meta-item">
								{#each secret.tags as tag}
									<span class="badge">{tag}</span>
								{/each}
							</span>
						{/if}
						{#if secret.added}
							<span class="meta-item">📅 {new Date(secret.added).toLocaleDateString()}</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
		<p class="count animate-in">🔐 {$filteredSecrets.length} secret(s)</p>
	{/if}
</div>

<style>
	.page { max-width: 900px; }
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
		animation: slideUp 0.4s ease;
	}
	h1 { font-size: 1.5rem; color: var(--purple-light); }
	.search {
		position: relative;
		display: flex;
		align-items: center;
	}
	.search-icon {
		position: absolute;
		left: 0.75rem;
		font-size: 0.85rem;
		opacity: 0.5;
		pointer-events: none;
	}
	.search input {
		background: var(--bg-card);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 1rem 0.5rem 2.2rem;
		border-radius: 8px;
		font-size: 0.9rem;
		width: 260px;
		transition: all 0.25s ease;
	}
	.search input:focus {
		outline: none;
		border-color: var(--purple);
		box-shadow: var(--glow-purple);
	}
	.tags {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
		animation: fadeIn 0.4s ease;
	}
	.tag {
		background: var(--bg-card);
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.25rem 0.75rem;
		border-radius: 12px;
		font-size: 0.8rem;
		cursor: pointer;
		transition: all 0.2s ease;
	}
	.tag:hover {
		border-color: var(--purple-dark);
		color: var(--text);
	}
	.tag.active {
		background: var(--purple);
		color: white;
		border-color: var(--purple);
		box-shadow: var(--glow-purple);
	}
	.secret-list { display: flex; flex-direction: column; gap: 0.75rem; }
	.secret-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 1rem 1.25rem;
	}
	.secret-header { display: flex; justify-content: space-between; align-items: center; }
	.secret-key { font-size: 0.95rem; color: var(--gold); }
	.secret-meta { margin-top: 0.5rem; display: flex; gap: 1rem; flex-wrap: wrap; }
	.meta-item { font-size: 0.8rem; color: var(--text-muted); }
	.badge {
		background: var(--purple-dark);
		color: var(--purple-light);
		padding: 0.1rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
		margin-right: 0.25rem;
	}
	.btn-sm {
		padding: 0.3rem 0.6rem;
		border-radius: 6px;
		border: none;
		font-size: 0.78rem;
		cursor: pointer;
		transition: all 0.2s ease;
	}
	.btn-danger {
		background: transparent;
		color: var(--danger);
		border: 1px solid var(--danger);
	}
	.btn-danger:hover {
		background: var(--danger);
		color: var(--bg);
		box-shadow: var(--glow-danger);
	}
	.status { color: var(--text-muted); padding: 2rem; text-align: center; }
	.error { color: var(--danger); }
	.count { color: var(--text-muted); font-size: 0.85rem; margin-top: 1rem; }

	/* Empty state */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 4rem 2rem;
		text-align: center;
	}
	.empty-icon { font-size: 3rem; margin-bottom: 1rem; opacity: 0.7; }
	.empty-text { color: var(--text-muted); font-size: 1.1rem; margin-bottom: 0.5rem; }
	.empty-hint { color: var(--text-muted); font-size: 0.85rem; opacity: 0.7; }
	.empty-hint code {
		background: var(--bg-hover);
		padding: 0.15rem 0.4rem;
		border-radius: 3px;
		font-size: 0.8rem;
	}
</style>
