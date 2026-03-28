<script>
	import { onMount } from 'svelte';
	import { fetchProjects, addProject, removeProject } from '$lib/api.js';

	let projects = $state([]);
	let loading = $state(true);
	let error = $state(null);
	let showAdd = $state(false);
	let newPath = $state('');
	let addError = $state('');
	let toast = $state(null);

	onMount(async () => {
		try {
			projects = (await fetchProjects()) || [];
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
		if (!newPath.trim()) {
			addError = 'Path is required';
			return;
		}
		try {
			await addProject(newPath.trim());
			projects = (await fetchProjects()) || [];
			showAdd = false;
			newPath = '';
			showToast('Project registered');
		} catch (e) {
			addError = e.message;
		}
	}

	async function handleRemove(name) {
		if (!confirm(`Unregister "${name}"?`)) return;
		try {
			await removeProject(name);
			projects = projects.filter((p) => p.name !== name);
			showToast(`${name} removed`);
		} catch (e) {
			error = e.message;
		}
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Projects</h1>
			<p class="page-desc">
				{projects.length} registered project{projects.length !== 1 ? 's' : ''}
			</p>
		</div>
		<button class="btn btn-primary" onclick={() => (showAdd = true)}>Add Project</button>
	</div>

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading...</span>
		</div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if projects.length === 0}
		<div class="empty-state">
			<span class="empty-icon">P</span>
			<p>No projects registered</p>
			<p><code>vial shelf add .</code></p>
		</div>
	{:else}
		<div class="project-list">
			{#each projects as p, i}
				<div class="project-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="project-info">
						<span class="project-name">{p.name}</span>
						<span class="project-path">{p.path}</span>
					</div>
					<div class="project-meta">
						{#if p.env_files && p.env_files.length > 0}
							{#each p.env_files as ef}
								<span class="badge">{ef}</span>
							{/each}
						{/if}
						{#if p.last_poured}
							<span class="meta-text">poured {new Date(p.last_poured).toLocaleDateString()}</span>
						{/if}
					</div>
					<button class="btn btn-sm btn-danger" onclick={() => handleRemove(p.name)}>Remove</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

{#if showAdd}
	<div class="modal-backdrop" onclick={() => (showAdd = false)} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
			<h3>Register Project</h3>
			<div class="form-group">
				<label class="form-label" for="proj-path">Directory Path</label>
				<input
					id="proj-path"
					class="input input-mono"
					bind:value={newPath}
					placeholder="/Users/you/projects/my-app"
				/>
			</div>
			{#if addError}
				<p class="error-text" style="font-size: 0.82rem;">{addError}</p>
			{/if}
			<div class="modal-actions">
				<button class="btn" onclick={() => (showAdd = false)}>Cancel</button>
				<button class="btn btn-primary" onclick={handleAdd}>Register</button>
			</div>
		</div>
	</div>
{/if}

{#if toast}
	<div class="toast success">{toast}</div>
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

	.project-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.project-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1rem;
	}
	.project-info {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
	}
	.project-name {
		font-weight: 600;
		font-size: 0.88rem;
		color: var(--text-bright);
	}
	.project-path {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.project-meta {
		display: flex;
		gap: 0.35rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.meta-text {
		font-size: 0.72rem;
		color: var(--text-muted);
	}
	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
