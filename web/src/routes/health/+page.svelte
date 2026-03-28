<script>
	import { onMount } from 'svelte';
	import { fetchHealth } from '$lib/api.js';

	let health = $state(null);
	let loading = $state(true);
	let error = $state(null);

	onMount(async () => {
		try {
			health = await fetchHealth();
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	function ageClass(days) {
		if (days > 180) return 'danger';
		if (days > 90) return 'warning';
		return 'ok';
	}

	function ageEmoji(days) {
		if (days > 180) return '🔴';
		if (days > 90) return '🟡';
		return '🟢';
	}
</script>

<div class="page">
	<div class="page-header animate-in">
		<h1>💊 Secret Health</h1>
		<p class="description">Track the age, rotation status, and usage of your secrets.</p>
	</div>

	{#if loading}
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Checking vitals...</span>
		</div>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if health}
		<div class="stats">
			<div class="stat-card animate-in stagger-1">
				<div class="stat-icon">🔐</div>
				<div class="stat-value">{health.total_secrets}</div>
				<div class="stat-label">Total Secrets</div>
			</div>
			<div class="stat-card animate-in stagger-2">
				<div class="stat-icon">📁</div>
				<div class="stat-value">{health.total_projects}</div>
				<div class="stat-label">Projects</div>
			</div>
			<div class="stat-card animate-in stagger-3" class:stat-warning={health.stale_count > 0}>
				<div class="stat-icon">{health.stale_count > 0 ? '⚠️' : '✅'}</div>
				<div class="stat-value" class:warning={health.stale_count > 0}>{health.stale_count}</div>
				<div class="stat-label">Stale (90+ days)</div>
			</div>
		</div>

		{#if health.secrets && health.secrets.length > 0}
			<h2 class="animate-in stagger-4">Secret Details</h2>
			<div class="health-list">
				{#each health.secrets as s, i}
					<div class="health-row card-glow animate-in stagger-{Math.min(i + 5, 10)}">
						<span class="age-indicator">{ageEmoji(s.age_days)}</span>
						<span class="mono key">{s.key}</span>
						<div class="age-bar-wrapper">
							<div
								class="age-bar {ageClass(s.age_days)}"
								style="width: {Math.min((s.age_days / 180) * 100, 100)}%"
							></div>
						</div>
						<span class="age {ageClass(s.age_days)}">{s.age_days}d</span>
						{#if s.last_rotated}
							<span class="rotated">🔄 {new Date(s.last_rotated).toLocaleDateString()}</span>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.page {
		max-width: 850px;
	}
	.page-header {
		margin-bottom: 1.5rem;
	}
	h1 {
		color: var(--purple-light);
		margin-bottom: 0.5rem;
	}
	h2 {
		color: var(--text);
		margin: 1.5rem 0 1rem;
		font-size: 1.1rem;
	}
	.description {
		color: var(--text-muted);
		font-size: 0.9rem;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
		margin-bottom: 2rem;
	}
	.stat-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 1.25rem;
		text-align: center;
		transition: all 0.25s ease;
	}
	.stat-card:hover {
		border-color: var(--purple-dark);
		box-shadow: var(--glow-purple);
		transform: translateY(-2px);
	}
	.stat-warning {
		border-color: rgba(246, 173, 85, 0.3);
	}
	.stat-warning:hover {
		box-shadow: 0 0 20px rgba(246, 173, 85, 0.2);
	}
	.stat-icon {
		font-size: 1.5rem;
		margin-bottom: 0.5rem;
	}
	.stat-value {
		font-size: 2rem;
		font-weight: 700;
		color: var(--purple-light);
	}
	.stat-value.warning {
		color: var(--warning);
	}
	.stat-label {
		color: var(--text-muted);
		font-size: 0.85rem;
		margin-top: 0.25rem;
	}
	.health-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.health-row {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.75rem 1rem;
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.age-indicator {
		font-size: 0.85rem;
	}
	.key {
		color: var(--gold);
		flex: 1;
		font-size: 0.9rem;
	}
	.age-bar-wrapper {
		width: 80px;
		height: 4px;
		background: var(--border);
		border-radius: 2px;
		overflow: hidden;
	}
	.age-bar {
		height: 100%;
		border-radius: 2px;
		transition: width 0.6s ease;
	}
	.age-bar.ok {
		background: var(--success);
	}
	.age-bar.warning {
		background: var(--warning);
	}
	.age-bar.danger {
		background: var(--danger);
	}
	.age {
		font-size: 0.85rem;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		min-width: 45px;
		text-align: right;
	}
	.age.ok {
		color: var(--success);
	}
	.age.warning {
		color: var(--warning);
	}
	.age.danger {
		color: var(--danger);
	}
	.rotated {
		color: var(--text-muted);
		font-size: 0.78rem;
	}
	.status {
		color: var(--text-muted);
		padding: 2rem;
		text-align: center;
	}
	.error {
		color: var(--danger);
	}
</style>
