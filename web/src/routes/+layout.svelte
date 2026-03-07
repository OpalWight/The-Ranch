<script>
  import '../app.css';
  let { children } = $props();
</script>

<div class="app">
  <header>
    <h1>The Ranch</h1>
    <span class="subtitle">File Manager</span>
  </header>
  <main>
    {@render children()}
  </main>
</div>

<style>
  .app {
    display: flex;
    flex-direction: column;
    min-height: 100vh;
  }

  header {
    display: flex;
    align-items: baseline;
    gap: 1rem;
    padding: 1rem 1.5rem;
    background: var(--color-surface);
    border-bottom: 1px solid var(--color-border);
  }

  header h1 {
    font-size: 1.25rem;
    font-weight: 700;
  }

  .subtitle {
    color: var(--color-text-muted);
    font-size: 0.875rem;
  }

  main {
    flex: 1;
    padding: 1.5rem;
    max-width: 1200px;
    width: 100%;
    margin: 0 auto;
  }
</style>
