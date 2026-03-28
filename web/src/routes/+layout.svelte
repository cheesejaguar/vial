<script>
	import '../app.css';
	import { page } from '$app/stores';
	let { children } = $props();

	const navItems = [
		{ href: '/', label: 'Secrets', icon: '🔑' },
		{ href: '/aliases', label: 'Aliases', icon: '🏷️' },
		{ href: '/projects', label: 'Projects', icon: '📁' },
		{ href: '/health', label: 'Health', icon: '💊' },
	];

	function isActive(href, pathname) {
		if (href === '/') return pathname === '/';
		return pathname.startsWith(href);
	}
</script>

<div class="layout">
	<nav class="sidebar">
		<div class="logo">
			<span class="logo-icon">🧪</span>
			<span class="logo-text">Vial</span>
		</div>
		<ul class="nav-links">
			{#each navItems as item}
				<li>
					<a href={item.href} class:active={isActive(item.href, $page.url.pathname)}>
						<span class="nav-icon">{item.icon}</span>
						<span class="nav-label">{item.label}</span>
					</a>
				</li>
			{/each}
		</ul>
		<div class="sidebar-footer">
			<div class="sidebar-tagline">Encrypted secret vault</div>
		</div>
	</nav>
	<main class="content page-enter">
		{@render children()}
	</main>
</div>

<style>
	.layout {
		display: flex;
		min-height: 100vh;
	}
	.sidebar {
		width: 230px;
		background: linear-gradient(180deg, var(--bg-card) 0%, #151520 100%);
		border-right: 1px solid var(--border);
		padding: 1.5rem 1rem;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		position: sticky;
		top: 0;
		height: 100vh;
	}
	.logo {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin-bottom: 2rem;
		padding: 0.5rem 0.75rem;
		animation: slideInLeft 0.5s ease;
	}
	.logo-icon {
		font-size: 1.6rem;
		animation: pulse 3s ease-in-out infinite;
	}
	.logo-text {
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--purple-light);
		font-family: var(--font-mono);
		letter-spacing: 0.5px;
	}
	.nav-links {
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.nav-links a {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		padding: 0.55rem 0.75rem;
		color: var(--text-muted);
		text-decoration: none;
		border-radius: 8px;
		font-size: 0.9rem;
		transition: all 0.2s ease;
		position: relative;
	}
	.nav-links a:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.nav-links a.active {
		background: rgba(107, 70, 193, 0.15);
		color: var(--purple-light);
		font-weight: 600;
		box-shadow: inset 3px 0 0 var(--purple);
	}
	.nav-icon {
		font-size: 1rem;
		width: 1.4rem;
		text-align: center;
	}
	.nav-label {
		flex: 1;
	}
	.sidebar-footer {
		margin-top: auto;
		padding: 1rem 0.75rem 0;
		border-top: 1px solid var(--border);
	}
	.sidebar-tagline {
		font-size: 0.7rem;
		color: var(--text-muted);
		opacity: 0.5;
		text-align: center;
	}
	.content {
		flex: 1;
		padding: 2rem;
		max-width: 1200px;
	}
</style>
