<!--
  +layout.svelte — Root application shell for the Vial dashboard SPA.

  Responsibilities:
    1. Auth bootstrap: reads the one-time Bearer token from the URL fragment
       (#token=<hex>) on first load, persists it to sessionStorage via
       setToken(), then immediately removes the fragment from the browser
       history so the token never appears in back/forward navigation or
       server logs.
    2. Auth gate: renders child routes only when a token is present. When
       unauthenticated (token missing or cleared after 401), shows a minimal
       "Not Connected" prompt that tells the user to run `vial dashboard`.
    3. Navigation: renders the persistent left sidebar with links to every
       page. Active state is calculated per-link against the current pathname.
    4. Lock action: the sidebar Lock button calls the /auth/lock API endpoint
       which zeroes the in-memory DEK on the Go server, then silently swallows
       the error because the vault may already be locked.

  Auth flow detail:
    The Go server appends #token=<hex> to the URL it opens in the browser.
    HTTP fragments are client-only — they are never transmitted in the request
    URI — so the token is not exposed in any network log. The layout strips the
    fragment via history.replaceState() immediately after reading it, so the
    clean URL is what the user sees and copies.
-->
<script>
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { setToken, hasToken, lockVault } from '$lib/api.js';

	/** Slot content: the matched child route component. */
	let { children } = $props();

	/**
	 * Whether the user has a valid session token in sessionStorage.
	 * Starts false and is set synchronously in onMount after the fragment check,
	 * so there is a single render cycle with the auth gate before the dashboard
	 * becomes visible — this prevents a flash of authenticated content.
	 */
	let authenticated = $state(false);

	onMount(() => {
		const hash = window.location.hash;

		// If the Go server appended the token as a URL fragment, extract and
		// persist it, then rewrite the URL to the clean pathname. Using
		// history.replaceState (not assign/href) avoids adding a new history
		// entry, so the Back button behaves naturally.
		if (hash.startsWith('#token=')) {
			setToken(hash.substring(7)); // substring(7) skips the '#token=' prefix
			history.replaceState(null, '', window.location.pathname);
		}

		// Determine auth state after potentially storing the new token above.
		authenticated = hasToken();
	});

	/**
	 * Navigation items rendered in the sidebar. The icon character doubles as
	 * a keyboard hint rendered in a small monospace badge beside each label.
	 */
	const nav = [
		{ href: '/', label: 'Secrets', icon: 'K' },
		{ href: '/projects', label: 'Projects', icon: 'P' },
		{ href: '/aliases', label: 'Aliases', icon: 'A' },
		{ href: '/health', label: 'Health', icon: 'H' },
		{ href: '/audit', label: 'Audit', icon: 'L' },
		{ href: '/settings', label: 'Settings', icon: 'S' },
	];

	/**
	 * Returns true when the given nav href should be considered active for the
	 * current pathname. The root '/' route requires an exact match to avoid
	 * marking every route as active (all paths start with '/').
	 *
	 * @param {string} href — The nav item's href.
	 * @param {string} pathname — The current URL pathname from $page.url.
	 * @returns {boolean}
	 */
	function isActive(href, pathname) {
		if (href === '/') return pathname === '/';
		return pathname.startsWith(href);
	}

	/**
	 * Sends the lock request to the Go server and intentionally swallows any
	 * error. The vault may already be locked (e.g. the user locked it from the
	 * CLI), in which case the API returns an error — but the desired end state
	 * (locked vault) is already achieved, so surfacing an error would be noise.
	 */
	async function handleLock() {
		try {
			await lockVault();
		} catch {
			// Silently ignore — see note above.
		}
	}
</script>

<div class="shell">
	<aside class="sidebar">
		<!-- Brand mark: links back to the secrets (home) page -->
		<a href="/" class="brand">
			<span class="brand-mark">V</span>
			<span class="brand-text">vial</span>
		</a>

		<nav class="nav">
			{#each nav as item}
				<!-- isActive() drives the .active CSS class, which highlights the
				     current page and tints the keyboard-hint badge purple. -->
				<a href={item.href} class="nav-item" class:active={isActive(item.href, $page.url.pathname)}>
					<span class="nav-key">{item.icon}</span>
					<span>{item.label}</span>
				</a>
			{/each}
		</nav>

		<!-- Lock button sits at the very bottom of the sidebar, separated from
		     navigation by a border, so it's clearly a destructive action. -->
		<div class="sidebar-bottom">
			<button class="nav-item lock-btn" onclick={handleLock}>
				<span class="nav-key">L</span>
				<span>Lock</span>
			</button>
		</div>
	</aside>

	<main class="main page-enter">
		{#if authenticated}
			<!-- Render the matched child route (secrets, projects, aliases, etc.) -->
			{@render children()}
		{:else}
			<!-- Auth gate: shown when no token exists in sessionStorage.
			     Instructs the user to run the CLI command that opens the
			     dashboard with a fresh token in the URL fragment. -->
			<div class="auth-gate">
				<div class="auth-lock">
					<svg
						width="32"
						height="32"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.5"
						stroke-linecap="round"
						stroke-linejoin="round"
						style="color: var(--text-muted)"
					>
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
