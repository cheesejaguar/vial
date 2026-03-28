<script>
	import { onMount } from 'svelte';
	import { fetchProjects } from '$lib/api.js';

	let projects = $state([]);
	let loading = $state(true);
	let error = $state(null);

	onMount(async () => {
		try {
			projects = (await fetchProjects()) || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
</script>

<div class="page">
	<div class="page-header animate-in">
		<h1>📁 Projects</h1>
		<p class="description">Registered project directories for batch pour operations.</p>
	</div>

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading projects...</span>
		</div>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if projects.length === 0}
		<div class="empty-state animate-in">
			<span class="empty-icon">📁</span>
			<p class="empty-text">No projects registered.</p>
			<p class="empty-hint">Use <code>vial shelf add</code> to register one.</p>
		</div>
	{:else}
		<div class="project-list">
			{#each projects as p, i}
				<div class="project-card card-glow animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="project-header">
						<span class="project-icon">📦</span>
						<div class="project-name">{p.name}</div>
					</div>
					<div class="project-path mono">{p.path}</div>
					{#if p.env_files && p.env_files.length > 0}
						<div class="env-files">
							{#each p.env_files as ef}
								<span class="badge">📄 {ef}</span>
							{/each}
						</div>
					{/if}
					{#if p.last_poured}
						<div class="last-poured">
							🫗 Last poured: {new Date(p.last_poured).toLocaleString()}
						</div>
					{/if}
				</div>
			{/each}
		</div>
		<p class="count animate-in">📁 {projects.length} project(s) registered</p>
	{/if}
</div>

<style>
	.page {
		max-width: 800px;
	}
	.page-header {
		margin-bottom: 1.5rem;
	}
	h1 {
		color: var(--purple-light);
		margin-bottom: 0.5rem;
	}
	.description {
		color: var(--text-muted);
		font-size: 0.9rem;
	}
	.project-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.project-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 1rem 1.25rem;
	}
	.project-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.25rem;
	}
	.project-icon {
		font-size: 1rem;
	}
	.project-name {
		font-weight: 600;
		font-size: 1rem;
	}
	.project-path {
		color: var(--text-muted);
		font-size: 0.82rem;
		margin-bottom: 0.5rem;
	}
	.env-files {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
		flex-wrap: wrap;
	}
	.badge {
		background: var(--purple-dark);
		color: var(--purple-light);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
	}
	.last-poured {
		color: var(--text-muted);
		font-size: 0.8rem;
	}
	.count {
		color: var(--text-muted);
		font-size: 0.85rem;
		margin-top: 1rem;
	}
	.status {
		color: var(--text-muted);
		padding: 2rem;
		text-align: center;
	}
	.error {
		color: var(--danger);
	}

	/* Empty state */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 4rem 2rem;
		text-align: center;
	}
	.empty-icon {
		font-size: 3rem;
		margin-bottom: 1rem;
		opacity: 0.7;
	}
	.empty-text {
		color: var(--text-muted);
		font-size: 1.1rem;
		margin-bottom: 0.5rem;
	}
	.empty-hint {
		color: var(--text-muted);
		font-size: 0.85rem;
		opacity: 0.7;
	}
	.empty-hint code {
		background: var(--bg-hover);
		padding: 0.15rem 0.4rem;
		border-radius: 3px;
		font-size: 0.8rem;
	}
</style>
