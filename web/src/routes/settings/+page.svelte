<script>
	import { onMount } from 'svelte';
	import { fetchConfig, lockVault, authStatus } from '$lib/api.js';

	let config = $state(null);
	let status = $state(null);
	let loading = $state(true);
	let error = $state(null);

	onMount(async () => {
		try {
			const [cfg, auth] = await Promise.all([
				fetchConfig().catch(() => null),
				authStatus().catch(() => null),
			]);
			config = cfg;
			status = auth;
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function handleLock() {
		try {
			await lockVault();
			status = { locked: true };
		} catch {}
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
		<section class="section">
			<h2>Vault Status</h2>
			<div class="setting-row card">
				<div>
					<span class="setting-label">Lock State</span>
					<span class="setting-value">
						{#if status && !status.locked}
							<span class="status-dot ok" style="display: inline-block; margin-right: 0.4rem;"
							></span>Unlocked
						{:else}
							<span class="status-dot danger" style="display: inline-block; margin-right: 0.4rem;"
							></span>Locked
						{/if}
					</span>
				</div>
				{#if status && !status.locked}
					<button class="btn btn-sm" onclick={handleLock}>Lock Now</button>
				{/if}
			</div>
		</section>

		{#if config}
			<section class="section">
				<h2>Configuration</h2>
				<div class="config-list">
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
				<p class="config-hint">
					Edit <code>~/.config/vial/config.yaml</code> to change these values.
				</p>
			</section>
		{/if}

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
