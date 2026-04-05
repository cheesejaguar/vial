<!--
  projects/+page.svelte — Project management page.

  A "project" in Vial is a registered directory that `vial pour` writes
  secrets into (as a .env file or similar). Registering a project lets the
  matching engine know which vault keys a project needs, and lets the audit
  log record which project triggered a secret access.

  This page allows the user to:
    - View all registered projects with their filesystem path, detected env
      files, and the date secrets were last poured.
    - Register a new project directory via a modal form.
    - Unregister (remove) a project with a confirmation prompt.

  Unregistering a project does not modify or delete any .env files that were
  previously written to that directory — it only removes the project record
  from vial's configuration.
-->
<script>
	import { onMount } from 'svelte';
	import { fetchProjects, addProject, removeProject } from '$lib/api.js';

	// ---------------------------------------------------------------------------
	// Page state (Svelte 5 runes)
	// ---------------------------------------------------------------------------

	/**
	 * The list of registered project objects returned by GET /api/projects.
	 * Each object has at minimum: { name: string, path: string }
	 * Optional fields: env_files?: string[], last_poured?: string (ISO timestamp)
	 * @type {Array<{ name: string, path: string, env_files?: string[], last_poured?: string }>}
	 */
	let projects = $state([]);

	/** True while the initial projects fetch is in progress. */
	let loading = $state(true);

	/** Page-level error string shown below the header on fetch failure. */
	let error = $state(null);

	/** Controls visibility of the "Register Project" modal. */
	let showAdd = $state(false);

	/** Form field: absolute filesystem path to the project directory. */
	let newPath = $state('');

	/** Validation / server error shown inside the modal. */
	let addError = $state('');

	/**
	 * Transient success notification.
	 * @type {string | null}
	 */
	let toast = $state(null);

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------

	onMount(async () => {
		try {
			projects = (await fetchProjects()) || [];
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
	 * Shows a success toast for 2.5 s then clears it.
	 * @param {string} msg
	 */
	function showToast(msg) {
		toast = msg;
		setTimeout(() => (toast = null), 2500);
	}

	// ---------------------------------------------------------------------------
	// Handlers
	// ---------------------------------------------------------------------------

	/**
	 * Validates the path field then registers the directory with the Go server.
	 * Re-fetches the full project list after success to pick up the project name
	 * derived by the server from the directory basename.
	 */
	async function handleAdd() {
		addError = '';

		// Client-side guard: the API will also reject an empty path, but checking
		// here avoids a network round trip for the most common mistake.
		if (!newPath.trim()) {
			addError = 'Path is required';
			return;
		}

		try {
			await addProject(newPath.trim());
			// Re-fetch so the list includes the server-derived project name and any
			// detected env files from the registered directory.
			projects = (await fetchProjects()) || [];
			showAdd = false;
			newPath = '';
			showToast('Project registered');
		} catch (e) {
			addError = e.message;
		}
	}

	/**
	 * Asks the user to confirm before unregistering the project. On confirm,
	 * calls the API then optimistically removes the entry from the local list.
	 *
	 * The project is identified by name (not path) because the name is the
	 * stable identifier used in the API route: DELETE /api/projects/:name
	 *
	 * @param {string} name - The project's name (basename of its directory).
	 */
	async function handleRemove(name) {
		if (!confirm(`Unregister "${name}"?`)) return;
		try {
			await removeProject(name);
			// Optimistic update: filter locally instead of re-fetching.
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
			<!-- Count reflects the total registered projects, not a filtered view,
			     since this page has no search/filter controls. -->
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
		<!-- Empty state includes the CLI command for users who prefer the terminal. -->
		<div class="empty-state">
			<span class="empty-icon">P</span>
			<p>No projects registered</p>
			<p><code>vial shelf add .</code></p>
		</div>
	{:else}
		<div class="project-list">
			{#each projects as p, i}
				<!-- 3-column grid: project info | meta badges | remove button -->
				<div class="project-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="project-info">
						<span class="project-name">{p.name}</span>
						<!-- Full filesystem path in monospace; overflow is truncated with
						     ellipsis so long paths don't break the layout. -->
						<span class="project-path">{p.path}</span>
					</div>
					<div class="project-meta">
						<!-- env_files: detected template files in the project directory
						     (e.g. .env.example). Shown as badges so the user knows which
						     files vial will write to during a pour. -->
						{#if p.env_files && p.env_files.length > 0}
							{#each p.env_files as ef}
								<span class="badge">{ef}</span>
							{/each}
						{/if}
						<!-- last_poured: ISO timestamp of the most recent pour operation.
						     Formatted as a locale date string for readability. -->
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

<!-- Register Project modal. The directory path field accepts any absolute path;
     the server validates that the path exists and is readable. -->
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
