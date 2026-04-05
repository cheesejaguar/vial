<!--
  settings/+page.svelte — Settings and configuration page.

  Provides a read-only view of the vault's current state and the active
  configuration, plus a "Lock Now" button so the user can zero the in-memory
  DEK from the browser without returning to the terminal.

  Sections:
    1. Vault Status — shows whether the vault is currently unlocked, with a
       Lock Now button when it is. After locking, the local `status` state is
       updated optimistically so the button disappears without a re-fetch.
    2. Configuration — key/value pairs from ~/.config/vial/config.yaml, loaded
       via GET /api/config. All values are read-only in the dashboard; the hint
       text tells the user to edit the YAML file to make changes.
    3. About — version/product links to the website, GitHub, and security docs.

  Data loading:
    Both fetchConfig() and authStatus() are fired in parallel with Promise.all
    so the page reaches its loaded state in a single network round trip. Each
    call is individually .catch()'d to null so a single failing endpoint does
    not prevent the rest of the page from rendering.
-->
<script>
	import { onMount } from 'svelte';
	import { fetchConfig, lockVault, authStatus } from '$lib/api.js';

	// ---------------------------------------------------------------------------
	// Page state (Svelte 5 runes)
	// ---------------------------------------------------------------------------

	/**
	 * Active configuration values from ~/.config/vial/config.yaml.
	 * Shape: { vault_path?: string, session_timeout?: string,
	 *           env_example?: string, log_level?: string }
	 * Null when the /api/config endpoint is unavailable.
	 * @type {{ vault_path?: string, session_timeout?: string, env_example?: string, log_level?: string } | null}
	 */
	let config = $state(null);

	/**
	 * Current vault lock state from GET /api/auth/status.
	 * Shape: { locked: boolean }
	 * Also set optimistically to { locked: true } after a successful lock call.
	 * @type {{ locked: boolean } | null}
	 */
	let status = $state(null);

	/** True while both parallel fetches are in progress. */
	let loading = $state(true);

	/** Error string shown below the header if the combined fetch throws. */
	let error = $state(null);

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------

	onMount(async () => {
		try {
			// Fire both requests in parallel to minimise time-to-interactive.
			// Each is individually caught so a missing /config or /auth/status
			// endpoint still allows the rest of the page to render.
			const [cfg, auth] = await Promise.all([
				fetchConfig().catch(() => null),
				authStatus().catch(() => null),
			]);
			config = cfg;
			status = auth;
		} catch (e) {
			// This branch only fires if Promise.all itself throws (unexpected).
			error = e.message;
		} finally {
			loading = false;
		}
	});

	// ---------------------------------------------------------------------------
	// Handlers
	// ---------------------------------------------------------------------------

	/**
	 * Sends the lock request to the Go server, which zeroes the in-memory DEK.
	 * On success, optimistically updates the local `status` object so the "Lock
	 * Now" button disappears immediately without waiting for an authStatus re-fetch.
	 *
	 * Errors are silently swallowed because the vault may already be locked (in
	 * which case the server returns an error but the desired state is achieved).
	 */
	async function handleLock() {
		try {
			await lockVault();
			// Optimistic update: reflect the locked state locally.
			status = { locked: true };
		} catch {
			// Silently ignore — see note above.
		}
	}
</script>

<div class="page">
	<div class="page-header">
		<h1>Settings</h1>
	</div>

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading...</span>
		</div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else}
		<!-- Section 1: Vault Status -->
		<section class="section">
			<h2>Vault Status</h2>
			<div class="setting-row card">
				<div>
					<span class="setting-label">Lock State</span>
					<span class="setting-value">
						{#if status && !status.locked}
							<!-- Green dot + "Unlocked" — vault DEK is in memory, API calls
							     that require decryption will succeed. -->
							<span class="status-dot ok" style="display: inline-block; margin-right: 0.4rem;"
							></span>Unlocked
						{:else}
							<!-- Red dot + "Locked" — DEK has been zeroed. Subsequent vault
							     operations will fail until the user runs `vial uncork`. -->
							<span class="status-dot danger" style="display: inline-block; margin-right: 0.4rem;"
							></span>Locked
						{/if}
					</span>
				</div>
				<!-- Lock Now button is only shown when the vault is currently unlocked.
				     After clicking, the optimistic state update hides this button. -->
				{#if status && !status.locked}
					<button class="btn btn-sm" onclick={handleLock}>Lock Now</button>
				{/if}
			</div>
		</section>

		<!-- Section 2: Configuration — only rendered when /api/config responded. -->
		{#if config}
			<section class="section">
				<h2>Configuration</h2>
				<div class="config-list">
					<!-- Fallback values shown when config keys are absent (fresh install
					     with no config.yaml). These match the Go defaults in config.go. -->
					<div class="config-row card">
						<span class="config-key">vault_path</span>
						<span class="config-val">{config.vault_path || '~/.local/share/vial/vault.json'}</span>
					</div>
					<div class="config-row card">
						<span class="config-key">session_timeout</span>
						<span class="config-val">{config.session_timeout || '4h'}</span>
					</div>
					<div class="config-row card">
						<span class="config-key">env_example</span>
						<span class="config-val">{config.env_example || '.env.example'}</span>
					</div>
					<div class="config-row card">
						<span class="config-key">log_level</span>
						<span class="config-val">{config.log_level || 'warn'}</span>
					</div>
				</div>
				<!-- Hint text explains why there are no edit controls here. -->
				<p class="config-hint">
					Edit <code>~/.config/vial/config.yaml</code> to change these values.
				</p>
			</section>
		{/if}

		<!-- Section 3: About — product identity and external links. -->
		<section class="section">
			<h2>About</h2>
			<div class="about card">
				<p><strong>Vial</strong> &mdash; The centralized secret vault for vibe coders.</p>
				<p class="about-links">
					<a href="https://one-vial.org">Website</a>
					<span class="sep">&middot;</span>
					<a href="https://github.com/cheesejaguar/vial">GitHub</a>
					<span class="sep">&middot;</span>
					<a href="https://github.com/cheesejaguar/vial/blob/main/SECURITY.md">Security</a>
				</p>
			</div>
		</section>
	{/if}
</div>

<style>
	.page {
		max-width: 600px;
	}
	.page-header {
		margin-bottom: 1.5rem;
	}
	h1 {
		font-size: 1.2rem;
		font-weight: 600;
		color: var(--text-bright);
	}
	h2 {
		font-size: 0.88rem;
		font-weight: 600;
		color: var(--text-secondary);
		margin-bottom: 0.5rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.section {
		margin-bottom: 2rem;
	}

	.setting-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
	}
	.setting-label {
		font-size: 0.85rem;
		color: var(--text-secondary);
		display: block;
	}
	.setting-value {
		font-size: 0.85rem;
		color: var(--text-bright);
		display: flex;
		align-items: center;
	}

	.config-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.config-row {
		display: flex;
		justify-content: space-between;
		padding: 0.55rem 1rem;
	}
	.config-key {
		font-family: var(--font-mono);
		font-size: 0.82rem;
		color: var(--text-secondary);
	}
	.config-val {
		font-family: var(--font-mono);
		font-size: 0.82rem;
		color: var(--text-bright);
	}
	.config-hint {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: 0.5rem;
	}

	.about {
		padding: 1rem;
		font-size: 0.88rem;
		color: var(--text-secondary);
	}
	.about strong {
		color: var(--text-bright);
	}
	.about-links {
		margin-top: 0.5rem;
		font-size: 0.82rem;
	}
	.sep {
		color: var(--text-muted);
		margin: 0 0.25rem;
	}
	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
