/**
 * stores/vault.js — Reactive Svelte stores for vault secret state.
 *
 * This module is the single source of truth for the secrets list on the
 * client side. The secrets page loads data from the API into the `secrets`
 * writable store; the derived stores (`filteredSecrets`, `allTags`) then
 * recompute automatically whenever secrets, the search query, or the active
 * tag filter changes — no manual refresh needed.
 *
 * Store ownership:
 *   - `secrets`       — written only by the secrets page after API calls.
 *   - `searchQuery`   — two-way bound to the filter <input> in the secrets page.
 *   - `activeTagFilter` — set by tag pill buttons in the secrets page.
 *   - `filteredSecrets` — read-only derived view consumed by the secrets list.
 *   - `allTags`       — read-only derived set consumed by the tag pill row.
 */

import { writable, derived } from 'svelte/store';

/**
 * The canonical list of secret objects returned by GET /api/vault/secrets.
 * Each object has at least a `key` string; it may also carry `tags` (string[])
 * and `aliases` (string[]) from the vault metadata.
 *
 * Initialised as an empty array so the secrets page renders the empty-state
 * UI immediately while the async fetch is in flight.
 *
 * @type {import('svelte/store').Writable<Array<{key: string, tags?: string[], aliases?: string[]}>>}
 */
export const secrets = writable([]);

/**
 * The current value of the search filter input. Compared case-insensitively
 * against secret key names. An empty string means "no filter applied".
 *
 * @type {import('svelte/store').Writable<string>}
 */
export const searchQuery = writable('');

/**
 * The tag currently selected in the tag pill row, or null when "All" is
 * active. When set, filteredSecrets is restricted to secrets that include
 * this tag in their `tags` array.
 *
 * @type {import('svelte/store').Writable<string|null>}
 */
export const activeTagFilter = writable(null);

/**
 * Derived view of `secrets` filtered by both `searchQuery` and
 * `activeTagFilter`. Recomputes synchronously whenever any upstream store
 * changes, so the UI always reflects the latest combination of filters without
 * additional event handling.
 *
 * Filter precedence: search runs first (reduces the set), then the tag filter
 * is applied to the already-reduced set.
 *
 * @type {import('svelte/store').Readable<Array<{key: string, tags?: string[], aliases?: string[]}>>}
 */
export const filteredSecrets = derived(
	[secrets, searchQuery, activeTagFilter],
	([$secrets, $query, $tag]) => {
		let result = $secrets;

		// Apply case-insensitive substring search against the key name only.
		// Searching against values is intentionally omitted — values should
		// never appear in plaintext in client-side memory unless explicitly
		// revealed by the user.
		if ($query) {
			const q = $query.toLowerCase();
			result = result.filter((s) => s.key.toLowerCase().includes(q));
		}

		// Further narrow the result to secrets that carry the selected tag.
		// Guard against secrets without a tags field (older vault entries).
		if ($tag) {
			result = result.filter((s) => s.tags && s.tags.includes($tag));
		}

		return result;
	},
);

/**
 * Derived sorted list of every unique tag present across all secrets. Used to
 * render the tag pill row above the secrets list. Recomputes when `secrets`
 * changes (e.g. after a create or delete operation).
 *
 * Tags are collected into a Set to deduplicate, then spread into an array and
 * sorted alphabetically for stable rendering order.
 *
 * @type {import('svelte/store').Readable<string[]>}
 */
export const allTags = derived(secrets, ($secrets) => {
	const tags = new Set();
	$secrets.forEach((s) => {
		if (s.tags) s.tags.forEach((t) => tags.add(t));
	});
	return [...tags].sort();
});
