const BASE = '';

function getToken() {
	return sessionStorage.getItem('vial_token') || '';
}

export function setToken(token) {
	sessionStorage.setItem('vial_token', token);
}

export function clearToken() {
	sessionStorage.removeItem('vial_token');
}

export function hasToken() {
	return !!getToken();
}

export async function apiFetch(path, options = {}) {
	const token = getToken();
	const res = await fetch(`${BASE}/api${path}`, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${token}`,
			...options.headers,
		},
	});

	if (res.status === 401) {
		clearToken();
		throw new Error('Not authenticated — run "vial dashboard" from your terminal');
	}

	if (!res.ok) {
		const text = await res.text();
		throw new Error(`${res.status}: ${text.trim()}`);
	}

	return res.json();
}

// Vault secrets
export const fetchSecrets = () => apiFetch('/vault/secrets');
export const fetchSecret = (key) => apiFetch(`/vault/secrets/${encodeURIComponent(key)}`);
export const revealSecret = (key) => apiFetch(`/vault/secrets/${encodeURIComponent(key)}?reveal=true`);
export const deleteSecret = (key) =>
	apiFetch(`/vault/secrets/${encodeURIComponent(key)}`, { method: 'DELETE' });
export const createSecret = (key, value) =>
	apiFetch('/vault/secrets', {
		method: 'POST',
		body: JSON.stringify({ key, value }),
	});

// Aliases
export const fetchAliases = () => apiFetch('/aliases');
export const createAlias = (alias, canonical) =>
	apiFetch('/aliases', {
		method: 'POST',
		body: JSON.stringify({ alias, canonical }),
	});
export const deleteAlias = (alias) =>
	apiFetch(`/aliases/${encodeURIComponent(alias)}`, { method: 'DELETE' });

// Projects
export const fetchProjects = () => apiFetch('/projects');
export const addProject = (path) =>
	apiFetch('/projects', {
		method: 'POST',
		body: JSON.stringify({ path }),
	});
export const removeProject = (name) =>
	apiFetch(`/projects/${encodeURIComponent(name)}`, { method: 'DELETE' });

// Health & audit
export const fetchHealth = () => apiFetch('/health/overview');
export const fetchAudit = () => apiFetch('/audit');

// Config
export const fetchConfig = () => apiFetch('/config');

// Auth
export const lockVault = () => apiFetch('/auth/lock', { method: 'POST' });
export const authStatus = () => apiFetch('/auth/status');
