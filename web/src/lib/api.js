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
		throw new Error(`API error ${res.status}: ${text}`);
	}

	return res.json();
}

export async function fetchSecrets() {
	return apiFetch('/vault/secrets');
}

export async function fetchSecret(key) {
	return apiFetch(`/vault/secrets/${encodeURIComponent(key)}`);
}

export async function deleteSecret(key) {
	return apiFetch(`/vault/secrets/${encodeURIComponent(key)}`, { method: 'DELETE' });
}

export async function fetchAliases() {
	return apiFetch('/aliases');
}

export async function fetchProjects() {
	return apiFetch('/projects');
}

export async function fetchHealth() {
	return apiFetch('/health/overview');
}

export async function unlockVault(password) {
	return apiFetch('/auth/unlock', {
		method: 'POST',
		body: JSON.stringify({ password }),
	});
}

export async function lockVault() {
	return apiFetch('/auth/lock', { method: 'POST' });
}
