import { writable, derived } from 'svelte/store';

export const secrets = writable([]);
export const searchQuery = writable('');
export const activeTagFilter = writable(null);

export const filteredSecrets = derived(
	[secrets, searchQuery, activeTagFilter],
	([$secrets, $query, $tag]) => {
		let result = $secrets;
		if ($query) {
			const q = $query.toLowerCase();
			result = result.filter((s) => s.key.toLowerCase().includes(q));
		}
		if ($tag) {
			result = result.filter((s) => s.tags && s.tags.includes($tag));
		}
		return result;
	},
);

export const allTags = derived(secrets, ($secrets) => {
	const tags = new Set();
	$secrets.forEach((s) => {
		if (s.tags) s.tags.forEach((t) => tags.add(t));
	});
	return [...tags].sort();
});
