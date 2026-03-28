<script>
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { setToken, hasToken, lockVault } from '$lib/api.js';

	let { children } = $props();
	let authenticated = $state(false);

	onMount(() => {
		const hash = window.location.hash;
		if (hash.startsWith('#token=')) {
			setToken(hash.substring(7));
			history.replaceState(null, '', window.location.pathname);
		}
		authenticated = hasToken();
	});

	const nav = [
		{ href: '/', label: 'Secrets', icon: 'K' },
		{ href: '/projects', label: 'Projects', icon: 'P' },
		{ href: '/aliases', label: 'Aliases', icon: 'A' },
		{ href: '/health', label: 'Health', icon: 'H' },
		{ href: '/audit', label: 'Audit', icon: 'L' },
		{ href: '/settings', label: 'Settings', icon: 'S' },
	];

	function isActive(href, pathname) {
		if (href === '/') return pathname === '/';
		return pathname.startsWith(href);
	}

	async function handleLock() {
		try {
			await lockVault();
		} catch {}
	}
</script>

<div class="shell">
	<aside class="sidebar">
		<a href="/" class="brand">
			<span class="brand-mark">V</span>
			<span class="brand-text">vial</span>
		</a>

		<nav class="nav">
			{#each nav as item}
				<a
					href={item.href}
					class="nav-item"
					class:active={isActive(item.href, $page.url.pathname)}
				>
					<span class="nav-key">{item.icon}</span>
					<span>{item.label}</span>
				</a>
			{/each}
		</nav>

		<div class="sidebar-bottom">
			<button class="nav-item lock-btn" onclick={handleLock}>
				<span class="nav-key">L</span>
				<span>Lock</span>
			</button>
		</div>
	</aside>

	<main class="main page-enter">
		{#if authenticated}
			{@render children()}
		{:else}
			<div class="auth-gate">
				<div class="auth-lock">
					<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--text-muted)">
						<rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
						<path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
					</svg>
				</div>
				<h2>Not Connected</h2>
				<p>Start the dashboard from your terminal:</p>
				<code>vial dashboard</code>
			</div>
		{/if}
	</main>
</div>

<style>
	.shell {
		display: flex;
		min-height: 100vh;
	}

	.sidebar {
		width: 180px;
		background: var(--bg-surface);
		border-right: 1px solid var(--border);
		padding: 1.25rem 0.75rem;
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		position: sticky;
		top: 0;
		height: 100vh;
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.25rem 0.5rem;
		margin-bottom: 1.5rem;
		text-decoration: none;
	}

	.brand-mark {
		width: 24px;
		height: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--purple);
		color: #fff;
		border-radius: 6px;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 700;
	}

	.brand-text {
		font-family: var(--font-mono);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--text-bright);
		letter-spacing: -0.02em;
	}

	.nav {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.nav-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.5rem;
		font-size: 0.82rem;
		color: var(--text-secondary);
		text-decoration: none;
		border-radius: 6px;
		transition: all var(--transition);
		border: none;
		background: none;
		cursor: pointer;
		width: 100%;
		font-family: var(--font-sans);
	}

	.nav-item:hover {
		color: var(--text);
		background: var(--bg-hover);
	}

	.nav-item.active {
		color: var(--text-bright);
		background: var(--bg-raised);
	}

	.nav-key {
		width: 18px;
		height: 18px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		color: var(--text-muted);
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 4px;
		flex-shrink: 0;
	}

	.active .nav-key {
		background: var(--purple-muted);
		border-color: var(--purple-dark);
		color: var(--purple-light);
	}

	.sidebar-bottom {
		margin-top: auto;
		padding-top: 0.75rem;
		border-top: 1px solid var(--border);
	}

	.lock-btn {
		color: var(--text-muted);
	}

	.main {
		flex: 1;
		padding: 2rem 2.5rem;
		max-width: 960px;
	}

	/* Auth gate */
	.auth-gate {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 50vh;
		text-align: center;
		gap: 0.75rem;
	}

	.auth-gate h2 {
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--text-bright);
	}

	.auth-gate p {
		color: var(--text-secondary);
		font-size: 0.88rem;
	}
</style>
