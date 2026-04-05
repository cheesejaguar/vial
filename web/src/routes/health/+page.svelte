<!--
  health/+page.svelte — Secret health and rotation tracking page.

  Shows summary statistics and per-secret age information to help the user
  identify credentials that are overdue for rotation. The health data is
  computed server-side by the Go backend based on each secret's creation or
  last-updated timestamp stored in the vault metadata.

  Visual design:
    - Three summary stat cards: total secrets, total projects, stale count.
    - The "stale" card turns amber when stale_count > 0 to draw attention.
    - Each secret in the detail list has:
        - A coloured status dot (green / amber / red by age threshold)
        - An age progress bar capped at 180 days (the "critical" threshold)
        - A numeric age label coloured by the same threshold classes

  Age thresholds (mirrored from the Go health logic):
    - ok      : < 90 days
    - warning : 90–179 days
    - danger  : ≥ 180 days
-->
<script>
	import { onMount } from 'svelte';
	import { fetchHealth } from '$lib/api.js';

	// ---------------------------------------------------------------------------
	// Page state (Svelte 5 runes)
	// ---------------------------------------------------------------------------

	/**
	 * The health overview object returned by GET /api/health/overview.
	 * Shape: {
	 *   total_secrets: number,
	 *   total_projects: number,
	 *   stale_count: number,
	 *   secrets: Array<{ key: string, age_days: number }>
	 * }
	 * @type {{ total_secrets: number, total_projects: number, stale_count: number, secrets: Array<{ key: string, age_days: number }> } | null}
	 */
	let health = $state(null);

	/** True while the health fetch is in progress. */
	let loading = $state(true);

	/** Error string displayed below the header on fetch failure. */
	let error = $state(null);

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------

	onMount(async () => {
		try {
			health = await fetchHealth();
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
	 * Maps a secret's age in days to a CSS status class used for the dot,
	 * progress bar fill, and age label colour. Thresholds align with common
	 * security rotation recommendations:
	 *   - 90 days: first warning (many compliance frameworks require quarterly rotation)
	 *   - 180 days: danger threshold (six months without rotation is high risk)
	 *
	 * @param {number} days - Age of the secret in days.
	 * @returns {'danger' | 'warning' | 'ok'} CSS modifier class.
	 */
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
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading...</span>
		</div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if health}
		<!-- Summary stat cards — rendered as a 3-column grid.
		     The stale card gets a warning modifier class when stale_count > 0
		     so the amber number draws the eye without needing an explicit alert. -->
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
						<!-- Status dot colour matches the age-fill and age-num classes below,
						     giving three visual cues per row for accessibility. -->
						<div class="status-dot {statusClass(s.age_days)}"></div>
						<span class="key">{s.key}</span>

						<!-- Age progress bar: width is capped at 100% so secrets older than
						     180 days show a full bar rather than overflowing their container.
						     The fill class drives the colour (green → amber → red). -->
						<div class="age-track">
							<div
								class="age-fill {statusClass(s.age_days)}"
								style="width: {Math.min((s.age_days / 180) * 100, 100)}%"
							></div>
						</div>

						<!-- Numeric age label coloured by the same threshold class so the
						     number itself communicates urgency without a tooltip. -->
						<span class="age-num {statusClass(s.age_days)}">{s.age_days}d</span>
					</div>
				{/each}
			</div>
		{/if}
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
		font-size: 1.2rem;
		font-weight: 600;
		color: var(--text-bright);
	}
	h2 {
		font-size: 0.92rem;
		font-weight: 600;
		color: var(--text-bright);
		margin: 1.5rem 0 0.75rem;
	}
	.page-desc {
		font-size: 0.78rem;
		color: var(--text-muted);
		margin-top: 0.2rem;
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.75rem;
	}
	.stat {
		padding: 1.25rem;
		text-align: center;
	}
	.stat-val {
		display: block;
		font-size: 1.75rem;
		font-weight: 700;
		color: var(--text-bright);
	}
	.stat-label {
		font-size: 0.75rem;
		color: var(--text-muted);
	}
	/* Amber number in the stale card when there are secrets overdue for rotation. */
	.stat-warn .stat-val {
		color: var(--warning);
	}

	.health-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.health-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.6rem 1rem;
	}
	.key {
		font-family: var(--font-mono);
		font-size: 0.82rem;
		color: var(--text-bright);
		flex: 1;
	}
	.age-track {
		width: 80px;
		height: 3px;
		background: var(--border);
		border-radius: 2px;
		overflow: hidden;
	}
	.age-fill {
		height: 100%;
		border-radius: 2px;
		/* CSS transition animates the bar width when the page first renders,
		   making the relative ages easier to compare at a glance. */
		transition: width 0.5s ease;
	}
	.age-fill.ok {
		background: var(--success);
	}
	.age-fill.warning {
		background: var(--warning);
	}
	.age-fill.danger {
		background: var(--danger);
	}
	.age-num {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		min-width: 35px;
		text-align: right;
	}
	.age-num.ok {
		color: var(--success);
	}
	.age-num.warning {
		color: var(--warning);
	}
	.age-num.danger {
		color: var(--danger);
	}
	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
