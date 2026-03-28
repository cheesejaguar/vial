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

	function statusClass(days) {
		if (days > 180) return 'danger';
		if (days > 90) return 'warning';
		return 'ok';
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Health</h1>
			<p class="page-desc">Secret age and rotation tracking</p>
		</div>
	</div>

	{#if loading}
		<div class="loading-spinner"><div class="spinner"></div><span class="loading-text">Loading...</span></div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if health}
		<div class="stats">
			<div class="stat card">
				<span class="stat-val">{health.total_secrets}</span>
				<span class="stat-label">Secrets</span>
			</div>
			<div class="stat card">
				<span class="stat-val">{health.total_projects}</span>
				<span class="stat-label">Projects</span>
			</div>
			<div class="stat card" class:stat-warn={health.stale_count > 0}>
				<span class="stat-val">{health.stale_count}</span>
				<span class="stat-label">Stale (90d+)</span>
			</div>
		</div>

		{#if health.secrets && health.secrets.length > 0}
			<h2>Details</h2>
			<div class="health-list">
				{#each health.secrets as s, i}
					<div class="health-row card animate-in stagger-{Math.min(i + 1, 10)}">
						<div class="status-dot {statusClass(s.age_days)}"></div>
						<span class="key">{s.key}</span>
						<div class="age-track">
							<div class="age-fill {statusClass(s.age_days)}" style="width: {Math.min((s.age_days / 180) * 100, 100)}%"></div>
						</div>
						<span class="age-num {statusClass(s.age_days)}">{s.age_days}d</span>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.page { max-width: 800px; }
	.page-header { margin-bottom: 1.5rem; }
	h1 { font-size: 1.2rem; font-weight: 600; color: var(--text-bright); }
	h2 { font-size: 0.92rem; font-weight: 600; color: var(--text-bright); margin: 1.5rem 0 0.75rem; }
	.page-desc { font-size: 0.78rem; color: var(--text-muted); margin-top: 0.2rem; }

	.stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.75rem; }
	.stat { padding: 1.25rem; text-align: center; }
	.stat-val { display: block; font-size: 1.75rem; font-weight: 700; color: var(--text-bright); }
	.stat-label { font-size: 0.75rem; color: var(--text-muted); }
	.stat-warn .stat-val { color: var(--warning); }

	.health-list { display: flex; flex-direction: column; gap: 1px; }
	.health-row {
		display: flex; align-items: center; gap: 0.75rem;
		padding: 0.6rem 1rem;
	}
	.key { font-family: var(--font-mono); font-size: 0.82rem; color: var(--text-bright); flex: 1; }
	.age-track { width: 80px; height: 3px; background: var(--border); border-radius: 2px; overflow: hidden; }
	.age-fill { height: 100%; border-radius: 2px; transition: width 0.5s ease; }
	.age-fill.ok { background: var(--success); }
	.age-fill.warning { background: var(--warning); }
	.age-fill.danger { background: var(--danger); }
	.age-num { font-family: var(--font-mono); font-size: 0.75rem; min-width: 35px; text-align: right; }
	.age-num.ok { color: var(--success); }
	.age-num.warning { color: var(--warning); }
	.age-num.danger { color: var(--danger); }
	.error-text { color: var(--danger); font-size: 0.85rem; }
</style>
