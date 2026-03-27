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
		<h1>Secrets</h1>
		<div class="search">
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
		<p class="status">Loading secrets...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if $filteredSecrets.length === 0}
		<p class="status">No secrets found.</p>
	{:else}
		<div class="secret-list">
			{#each $filteredSecrets as secret}
				<div class="secret-card">
					<div class="secret-header">
						<span class="secret-key mono">{secret.key}</span>
						<div class="secret-actions">
							<button class="btn-sm btn-danger" onclick={() => handleDelete(secret.key)}>Delete</button>
						</div>
					</div>
					<div class="secret-meta">
						{#if secret.aliases && secret.aliases.length > 0}
							<span class="meta-item">Aliases: {secret.aliases.join(', ')}</span>
						{/if}
						{#if secret.tags && secret.tags.length > 0}
							<span class="meta-item">
								{#each secret.tags as tag}
									<span class="badge">{tag}</span>
								{/each}
							</span>
						{/if}
						{#if secret.added}
							<span class="meta-item">Added: {new Date(secret.added).toLocaleDateString()}</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
		<p class="count">{$filteredSecrets.length} secret(s)</p>
	{/if}
</div>

<style>
	.page { max-width: 900px; }
	.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; }
	h1 { font-size: 1.5rem; color: var(--purple-light); }
	.search input {
		background: var(--bg-card);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 1rem;
		border-radius: 6px;
		font-size: 0.9rem;
		width: 250px;
	}
	.search input:focus { outline: none; border-color: var(--purple); }
	.tags { display: flex; gap: 0.5rem; margin-bottom: 1rem; flex-wrap: wrap; }
	.tag {
		background: var(--bg-card);
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.25rem 0.75rem;
		border-radius: 12px;
		font-size: 0.8rem;
		cursor: pointer;
	}
	.tag.active { background: var(--purple); color: white; border-color: var(--purple); }
	.secret-list { display: flex; flex-direction: column; gap: 0.75rem; }
	.secret-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1rem 1.25rem;
	}
	.secret-card:hover { border-color: var(--purple-dark); }
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
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		border: none;
		font-size: 0.8rem;
		cursor: pointer;
	}
	.btn-danger { background: var(--danger); color: var(--bg); }
	.status { color: var(--text-muted); padding: 2rem; text-align: center; }
	.error { color: var(--danger); }
	.count { color: var(--text-muted); font-size: 0.85rem; margin-top: 1rem; }
</style>
