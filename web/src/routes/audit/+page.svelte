<script>
	import { onMount } from 'svelte';
	import { fetchAudit } from '$lib/api.js';

	let entries = $state([]);
	let loading = $state(true);
	let error = $state(null);

	onMount(async () => {
		try {
			entries = (await fetchAudit()) || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	function timeAgo(ts) {
		const diff = Date.now() - new Date(ts).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		const days = Math.floor(hrs / 24);
		return `${days}d ago`;
	}

	function eventColor(event) {
		if (event === 'remove' || event === 'lock') return 'danger';
		if (event === 'set' || event === 'pour' || event === 'unlock') return 'ok';
		return '';
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Audit Log</h1>
			<p class="page-desc">Recent vault activity</p>
		</div>
	</div>

	{#if loading}
		<div class="loading-spinner"><div class="spinner"></div><span class="loading-text">Loading...</span></div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if entries.length === 0}
		<div class="empty-state">
			<span class="empty-icon">L</span>
			<p>No audit entries yet</p>
		</div>
	{:else}
		<div class="audit-list">
			{#each entries as entry, i}
				<div class="audit-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<div class="status-dot {eventColor(entry.event)}"></div>
					<span class="event-type">{entry.event}</span>
					<span class="event-keys">
						{#if entry.keys && entry.keys.length > 0}
							{entry.keys.join(', ')}
						{/if}
					</span>
					{#if entry.detail}
						<span class="event-detail">{entry.detail}</span>
					{/if}
					<span class="event-time">{timeAgo(entry.timestamp)}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page { max-width: 800px; }
	.page-header { margin-bottom: 1.5rem; }
	h1 { font-size: 1.2rem; font-weight: 600; color: var(--text-bright); }
	.page-desc { font-size: 0.78rem; color: var(--text-muted); margin-top: 0.2rem; }

	.audit-list { display: flex; flex-direction: column; gap: 1px; }
	.audit-row {
		display: flex; align-items: center; gap: 0.75rem;
		padding: 0.55rem 1rem; font-size: 0.82rem;
	}
	.event-type {
		font-family: var(--font-mono); font-weight: 500;
		color: var(--text-bright); min-width: 60px;
	}
	.event-keys {
		font-family: var(--font-mono); color: var(--text-secondary);
		flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
	}
	.event-detail { color: var(--text-muted); font-size: 0.75rem; }
	.event-time { color: var(--text-muted); font-size: 0.75rem; white-space: nowrap; }
	.error-text { color: var(--danger); font-size: 0.85rem; }
</style>
