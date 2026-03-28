<script>
	import { onMount } from 'svelte';
	import { fetchAliases } from '$lib/api.js';

	let aliases = $state([]);
	let loading = $state(true);
	let error = $state(null);

	onMount(async () => {
		try {
			aliases = await fetchAliases() || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
</script>

<div class="page">
	<div class="page-header animate-in">
		<h1>🏷️ Aliases</h1>
		<p class="description">Alias mappings allow variant key names to resolve to the same vault secret.</p>
	</div>

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading aliases...</span>
		</div>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if aliases.length === 0}
		<div class="empty-state animate-in">
			<span class="empty-icon">🏷️</span>
			<p class="empty-text">No aliases defined.</p>
			<p class="empty-hint">Use <code>vial label set ALIAS=KEY</code> to create one.</p>
		</div>
	{:else}
		<div class="alias-list">
			{#each aliases as a, i}
				<div class="alias-row card-glow animate-in stagger-{Math.min(i + 1, 10)}">
					<span class="mono alias-from">{a.alias}</span>
					<span class="arrow">→</span>
					<span class="mono alias-to">{a.canonical}</span>
				</div>
			{/each}
		</div>
		<p class="count animate-in">🏷️ {aliases.length} alias(es)</p>
	{/if}
</div>

<style>
	.page { max-width: 700px; }
	.page-header { margin-bottom: 1.5rem; }
	h1 { color: var(--purple-light); margin-bottom: 0.5rem; }
	.description { color: var(--text-muted); font-size: 0.9rem; }
	.alias-list { display: flex; flex-direction: column; gap: 0.5rem; }
	.alias-row {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.75rem 1rem;
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.alias-from { color: var(--text-muted); }
	.arrow {
		color: var(--gold);
		font-size: 1.1rem;
		transition: transform 0.3s ease;
	}
	.alias-row:hover .arrow {
		transform: translateX(3px);
		color: var(--gold-light);
	}
	.alias-to { color: var(--gold); }
	.count { color: var(--text-muted); font-size: 0.85rem; margin-top: 1rem; }
	.status { color: var(--text-muted); padding: 2rem; text-align: center; }
	.error { color: var(--danger); }

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
