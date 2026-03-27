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
	<h1>Aliases</h1>
	<p class="description">Alias mappings allow variant key names to resolve to the same vault secret.</p>

	{#if loading}
		<p class="status">Loading...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if aliases.length === 0}
		<p class="status">No aliases defined. Use <code>vial label set ALIAS=KEY</code> to create one.</p>
	{:else}
		<div class="alias-list">
			{#each aliases as a}
				<div class="alias-row">
					<span class="mono alias-from">{a.alias}</span>
					<span class="arrow">→</span>
					<span class="mono alias-to">{a.canonical}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page { max-width: 700px; }
	h1 { color: var(--purple-light); margin-bottom: 0.5rem; }
	.description { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.9rem; }
	.alias-list { display: flex; flex-direction: column; gap: 0.5rem; }
	.alias-row {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 0.75rem 1rem;
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.alias-from { color: var(--text-muted); }
	.arrow { color: var(--gold); }
	.alias-to { color: var(--gold); }
	.status { color: var(--text-muted); padding: 2rem; text-align: center; }
	.error { color: var(--danger); }
	code { background: var(--bg-hover); padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.85rem; }
</style>
