<script>
	import '../app.css';
	let { children } = $props();
</script>

<div class="layout">
	<nav class="sidebar">
		<div class="logo">
			<span class="logo-icon">🧪</span>
			<span class="logo-text">Vial</span>
		</div>
		<ul class="nav-links">
			<li><a href="/">Secrets</a></li>
			<li><a href="/aliases">Aliases</a></li>
			<li><a href="/projects">Projects</a></li>
			<li><a href="/health">Health</a></li>
		</ul>
	</nav>
	<main class="content">
		{@render children()}
	</main>
</div>

<style>
	.layout {
		display: flex;
		min-height: 100vh;
	}
	.sidebar {
		width: 220px;
		background: var(--bg-card);
		border-right: 1px solid var(--border);
		padding: 1.5rem 1rem;
		flex-shrink: 0;
	}
	.logo {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 2rem;
		padding: 0 0.5rem;
	}
	.logo-icon { font-size: 1.5rem; }
	.logo-text {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--purple-light);
		font-family: var(--font-mono);
	}
	.nav-links {
		list-style: none;
	}
	.nav-links li { margin-bottom: 0.25rem; }
	.nav-links a {
		display: block;
		padding: 0.5rem 0.75rem;
		color: var(--text-muted);
		text-decoration: none;
		border-radius: 6px;
		font-size: 0.9rem;
		transition: all 0.15s;
	}
	.nav-links a:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.content {
		flex: 1;
		padding: 2rem;
		max-width: 1200px;
	}
</style>
