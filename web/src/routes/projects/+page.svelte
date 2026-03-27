<script>
	import { onMount } from 'svelte';
	import { fetchProjects } from '$lib/api.js';

	let projects = $state([]);
	let loading = $state(true);
	let error = $state(null);

	onMount(async () => {
		try {
			projects = await fetchProjects() || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
</script>

<div class="page">
	<h1>Projects</h1>
	<p class="description">Registered project directories for batch pour operations.</p>

	{#if loading}
		<p class="status">Loading...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if projects.length === 0}
		<p class="status">No projects registered. Use <code>vial shelf add</code> to register one.</p>
	{:else}
		<div class="project-list">
			{#each projects as p}
				<div class="project-card">
					<div class="project-name">{p.name}</div>
					<div class="project-path mono">{p.path}</div>
					{#if p.env_files && p.env_files.length > 0}
						<div class="env-files">
							{#each p.env_files as ef}
								<span class="badge">{ef}</span>
							{/each}
						</div>
					{/if}
					{#if p.last_poured}
						<div class="last-poured">Last poured: {new Date(p.last_poured).toLocaleString()}</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page { max-width: 800px; }
	h1 { color: var(--purple-light); margin-bottom: 0.5rem; }
	.description { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.9rem; }
	.project-list { display: flex; flex-direction: column; gap: 0.75rem; }
	.project-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1rem 1.25rem;
	}
	.project-name { font-weight: 600; font-size: 1rem; margin-bottom: 0.25rem; }
	.project-path { color: var(--text-muted); font-size: 0.85rem; margin-bottom: 0.5rem; }
	.env-files { display: flex; gap: 0.5rem; margin-bottom: 0.5rem; }
	.badge {
		background: var(--purple-dark);
		color: var(--purple-light);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
	}
	.last-poured { color: var(--text-muted); font-size: 0.8rem; }
	.status { color: var(--text-muted); padding: 2rem; text-align: center; }
	.error { color: var(--danger); }
	code { background: var(--bg-hover); padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.85rem; }
</style>
