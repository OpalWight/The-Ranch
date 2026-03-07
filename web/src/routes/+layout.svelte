<script>
  import '../app.css';
  let { children } = $props();
</script>

<div class="app">
  <header>
    <h1>the ranch</h1>
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
    align-items: center;
    padding: 0.75rem 1.5rem;
    border-bottom: 1px solid var(--color-border);
  }

  header h1 {
    font-family: var(--font-mono);
    font-size: 0.875rem;
    font-weight: 400;
    letter-spacing: 0.02em;
  }

  main {
    flex: 1;
    padding: 1.5rem;
    max-width: 1200px;
    width: 100%;
    margin: 0 auto;
  }
</style>
