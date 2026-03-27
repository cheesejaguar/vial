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
</script>

<div class="page">
	<h1>Secret Health</h1>
	<p class="description">Track the age, rotation status, and usage of your secrets.</p>

	{#if loading}
		<p class="status">Loading...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if health}
		<div class="stats">
			<div class="stat-card">
				<div class="stat-value">{health.total_secrets}</div>
				<div class="stat-label">Total Secrets</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{health.total_projects}</div>
				<div class="stat-label">Projects</div>
			</div>
			<div class="stat-card">
				<div class="stat-value" class:warning={health.stale_count > 0}>{health.stale_count}</div>
				<div class="stat-label">Stale (90+ days)</div>
			</div>
		</div>

		{#if health.secrets && health.secrets.length > 0}
			<h2>Secret Details</h2>
			<div class="health-list">
				{#each health.secrets as s}
					<div class="health-row">
						<span class="mono key">{s.key}</span>
						<span class="age {ageClass(s.age_days)}">{s.age_days}d old</span>
						{#if s.last_rotated}
							<span class="rotated">Rotated: {new Date(s.last_rotated).toLocaleDateString()}</span>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.page { max-width: 800px; }
	h1 { color: var(--purple-light); margin-bottom: 0.5rem; }
	h2 { color: var(--text); margin: 1.5rem 0 1rem; font-size: 1.1rem; }
	.description { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.9rem; }
	.stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; margin-bottom: 2rem; }
	.stat-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1.25rem;
		text-align: center;
	}
	.stat-value { font-size: 2rem; font-weight: 700; color: var(--purple-light); }
	.stat-value.warning { color: var(--warning); }
	.stat-label { color: var(--text-muted); font-size: 0.85rem; margin-top: 0.25rem; }
	.health-list { display: flex; flex-direction: column; gap: 0.5rem; }
	.health-row {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 0.75rem 1rem;
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.key { color: var(--gold); flex: 1; }
	.age { font-size: 0.85rem; padding: 0.15rem 0.5rem; border-radius: 4px; }
	.age.ok { color: var(--success); }
	.age.warning { color: var(--warning); }
	.age.danger { color: var(--danger); }
	.rotated { color: var(--text-muted); font-size: 0.8rem; }
	.status { color: var(--text-muted); padding: 2rem; text-align: center; }
	.error { color: var(--danger); }
</style>
