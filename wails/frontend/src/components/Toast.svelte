<script lang="ts">
  // Tiny, centered notification pinned to the bottom of the viewport.
  // App.svelte owns the timeout and the null/string state; this component
  // just renders when `message` is non-null.
  export let message: string | null = null;
</script>

{#if message}
  <div class="toast" role="status">{message}</div>
{/if}

<style>
  .toast {
    position: fixed;
    bottom: 12px;
    left: 50%;
    transform: translateX(-50%);
    background: var(--text-strong);
    color: var(--panel-bg-2);
    padding: 8px 16px;
    border-radius: 20px;
    font-size: 0.9rem;
  }
</style>
