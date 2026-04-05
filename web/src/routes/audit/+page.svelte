<!--
  audit/+page.svelte — Audit Log page.

  Displays a time-ordered list of vault operations recorded by the Go server.
  The audit log captures events such as: secret creation (set/pour), deletion
  (remove), vault lock/unlock, and secret reveal calls. Each entry includes the
  event type, the affected key names, optional detail text, and a timestamp.

  This page is read-only — the audit log cannot be modified or cleared from
  the dashboard.

  Visual design:
    - Destructive events (remove, lock) are marked with a red status dot.
    - Constructive events (set, pour, unlock) are marked with a green dot.
    - All other event types use a neutral (no colour class) dot.
    - Timestamps are displayed as human-relative strings ("just now", "3m ago")
      so the user can quickly gauge recency without parsing absolute datetimes.
-->
<script>
	import { onMount } from 'svelte';
	import { fetchAudit } from '$lib/api.js';

	// ---------------------------------------------------------------------------
	// Page state (Svelte 5 runes)
	// ---------------------------------------------------------------------------

	/**
	 * The ordered list of audit entries returned by GET /api/audit.
	 * Each entry has: { event: string, keys?: string[], detail?: string, timestamp: string }
	 * The timestamp is an ISO 8601 string produced by the Go server.
	 * @type {Array<{ event: string, keys?: string[], detail?: string, timestamp: string }>}
	 */
	let entries = $state([]);

	/** True while the audit fetch is in progress. */
	let loading = $state(true);

	/** Error string shown below the header if the fetch fails. */
	let error = $state(null);

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------

	onMount(async () => {
		try {
			entries = (await fetchAudit()) || [];
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
	 * Converts an ISO 8601 timestamp into a human-relative string.
	 * The granularity steps from "just now" → minutes → hours → days, which
	 * matches the typical audit use case where recent activity matters most.
	 *
	 * @param {string} ts - ISO 8601 timestamp string from the server.
	 * @returns {string} Human-relative string, e.g. "just now", "3m ago", "2d ago".
	 */
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

	/**
	 * Maps an audit event type to a CSS class name used to colour the status
	 * dot at the start of each row. The goal is fast visual scanning: red for
	 * destructive actions, green for constructive ones, neutral otherwise.
	 *
	 * @param {string} event - The event type string from the audit entry.
	 * @returns {'danger' | 'ok' | ''} CSS modifier class.
	 */
	function eventColor(event) {
		// 'remove' and 'lock' are treated as high-visibility events because they
		// either destroy data or reduce vault accessibility.
		if (event === 'remove' || event === 'lock') return 'danger';
		// 'set', 'pour', and 'unlock' are constructive — data was added or the
		// vault was made accessible.
		if (event === 'set' || event === 'pour' || event === 'unlock') return 'ok';
		// 'reveal', 'get', and any future event types fall through to neutral.
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
		<div class="loading-spinner">
			<div class="spinner"></div>
			<span class="loading-text">Loading...</span>
		</div>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if entries.length === 0}
		<!-- Empty state: shown when the vault has recorded no events yet. -->
		<div class="empty-state">
			<span class="empty-icon">L</span>
			<p>No audit entries yet</p>
		</div>
	{:else}
		<div class="audit-list">
			{#each entries as entry, i}
				<div class="audit-row card animate-in stagger-{Math.min(i + 1, 10)}">
					<!-- Coloured dot gives instant visual classification of the event. -->
					<div class="status-dot {eventColor(entry.event)}"></div>

					<!-- Event type in monospace so alignment is consistent across rows. -->
					<span class="event-type">{entry.event}</span>

					<!-- Comma-joined list of affected key names. The flex+ellipsis
					     combination truncates gracefully when many keys are affected. -->
					<span class="event-keys">
						{#if entry.keys && entry.keys.length > 0}
							{entry.keys.join(', ')}
						{/if}
					</span>

					<!-- Optional free-text detail (e.g. project name, matcher tier used). -->
					{#if entry.detail}
						<span class="event-detail">{entry.detail}</span>
					{/if}

					<!-- Relative timestamp; whitespace: nowrap prevents it from wrapping
					     mid-string on narrow viewports. -->
					<span class="event-time">{timeAgo(entry.timestamp)}</span>
				</div>
			{/each}
		</div>
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
	.page-desc {
		font-size: 0.78rem;
		color: var(--text-muted);
		margin-top: 0.2rem;
	}

	.audit-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.audit-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.55rem 1rem;
		font-size: 0.82rem;
	}
	.event-type {
		font-family: var(--font-mono);
		font-weight: 500;
		color: var(--text-bright);
		min-width: 60px;
	}
	.event-keys {
		font-family: var(--font-mono);
		color: var(--text-secondary);
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.event-detail {
		color: var(--text-muted);
		font-size: 0.75rem;
	}
	.event-time {
		color: var(--text-muted);
		font-size: 0.75rem;
		white-space: nowrap;
	}
	.error-text {
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
