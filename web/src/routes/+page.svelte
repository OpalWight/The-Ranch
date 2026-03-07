<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    listFiles,
    listDirectories,
    getDirectoryContents,
    createDirectory,
    deleteDirectory,
    uploadFile,
    deleteFile,
    downloadUrl,
  } from '$lib/api';
  import { connectSSE } from '$lib/sse';
  import { formatBytes, formatDate, fileIcon } from '$lib/utils';
  import type { FileRecord, Directory, FileEvent } from '$lib/types';

  // State
  let currentDirId: string | null = $state(null);
  let breadcrumb: Directory[] = $state([]);
  let directories: Directory[] = $state([]);
  let files: FileRecord[] = $state([]);
  let events: FileEvent[] = $state([]);
  let loading = $state(true);
  let error: string | null = $state(null);

  // New folder
  let showNewFolder = $state(false);
  let newFolderName = $state('');
  let creatingFolder = $state(false);

  // Upload
  let uploading = $state(false);
  let dragOver = $state(false);

  // Delete confirm
  let confirmDelete: { type: 'file' | 'dir'; id: string; name: string } | null = $state(null);

  let sse: EventSource | null = null;

  async function loadContents() {
    loading = true;
    error = null;
    try {
      if (currentDirId) {
        const contents = await getDirectoryContents(currentDirId);
        directories = contents.directories;
        files = contents.files;
        breadcrumb = contents.breadcrumb ?? [];
      } else {
        const [dirs, rootFiles] = await Promise.all([
          listDirectories(null),
          listFiles(null),
        ]);
        directories = dirs;
        files = rootFiles;
        breadcrumb = [];
      }
    } catch (e: any) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function navigateTo(dirId: string | null) {
    currentDirId = dirId;
    loadContents();
  }

  async function handleCreateFolder() {
    if (!newFolderName.trim()) return;
    creatingFolder = true;
    try {
      await createDirectory(newFolderName.trim(), currentDirId ?? undefined);
      newFolderName = '';
      showNewFolder = false;
      await loadContents();
    } catch (e: any) {
      error = e.message;
    } finally {
      creatingFolder = false;
    }
  }

  async function handleUpload(fileList: FileList | null) {
    if (!fileList?.length) return;
    uploading = true;
    error = null;
    try {
      for (const f of fileList) {
        await uploadFile(f, currentDirId ?? undefined);
      }
      await loadContents();
    } catch (e: any) {
      error = e.message;
    } finally {
      uploading = false;
    }
  }

  async function handleDelete() {
    if (!confirmDelete) return;
    try {
      if (confirmDelete.type === 'file') {
        await deleteFile(confirmDelete.id);
      } else {
        await deleteDirectory(confirmDelete.id);
      }
      confirmDelete = null;
      await loadContents();
    } catch (e: any) {
      error = e.message;
      confirmDelete = null;
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragOver = false;
    handleUpload(e.dataTransfer?.files ?? null);
  }

  function onSSEEvent(evt: FileEvent) {
    events = [evt, ...events].slice(0, 50);
    loadContents();
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && showNewFolder) handleCreateFolder();
    if (e.key === 'Escape') {
      showNewFolder = false;
      confirmDelete = null;
    }
  }

  onMount(() => {
    loadContents();
    sse = connectSSE(onSSEEvent);
  });

  onDestroy(() => {
    sse?.close();
  });
</script>

<svelte:window on:keydown={handleKeydown} />

<!-- Breadcrumb -->
<nav class="breadcrumb">
  <button class="crumb" class:active={!currentDirId} onclick={() => navigateTo(null)}>
    ~
  </button>
  {#each breadcrumb as dir}
    <span class="sep">/</span>
    <button
      class="crumb"
      class:active={dir.id === currentDirId}
      onclick={() => navigateTo(dir.id)}
    >
      {dir.name}
    </button>
  {/each}
</nav>

<!-- Toolbar -->
<div class="toolbar">
  <button class="btn btn-primary" onclick={() => (showNewFolder = !showNewFolder)}>
    New Folder
  </button>
  <label class="btn btn-primary upload-btn">
    {uploading ? 'Uploading...' : 'Upload Files'}
    <input
      type="file"
      multiple
      hidden
      disabled={uploading}
      onchange={(e) => handleUpload(e.currentTarget.files)}
    />
  </label>
</div>

<!-- New folder inline input -->
{#if showNewFolder}
  <div class="new-folder">
    <input
      type="text"
      placeholder="Folder name"
      bind:value={newFolderName}
      disabled={creatingFolder}
    />
    <button class="btn btn-primary" onclick={handleCreateFolder} disabled={creatingFolder}>
      Create
    </button>
    <button class="btn btn-ghost" onclick={() => (showNewFolder = false)}>Cancel</button>
  </div>
{/if}

<!-- Error -->
{#if error}
  <div class="error-banner">{error}</div>
{/if}

<!-- Drop zone + content -->
<div
  class="drop-zone"
  class:drag-over={dragOver}
  role="region"
  aria-label="File drop zone"
  ondragover={(e) => { e.preventDefault(); dragOver = true; }}
  ondragleave={() => (dragOver = false)}
  ondrop={onDrop}
>
  {#if loading}
    <p class="empty">Loading...</p>
  {:else if directories.length === 0 && files.length === 0}
    <p class="empty">Empty directory. Drop files or use the toolbar.</p>
  {:else}
    <div class="list">
      <!-- Column header -->
      <div class="list-header">
        <span class="col-name">Name</span>
        <span class="col-size">Size</span>
        <span class="col-date">Modified</span>
        <span class="col-action"></span>
      </div>

      <!-- Directories -->
      {#each directories as dir}
        <div class="list-row">
          <button class="row-main" onclick={() => navigateTo(dir.id)}>
            <span class="icon">dir</span>
            <span class="row-name">{dir.name}</span>
          </button>
          <span class="col-size"></span>
          <span class="col-date"></span>
          <button
            class="row-action"
            title="Delete folder"
            onclick={() => (confirmDelete = { type: 'dir', id: dir.id, name: dir.name })}
          >
            &times;
          </button>
        </div>
      {/each}

      <!-- Files -->
      {#each files as file}
        <div class="list-row">
          <a class="row-main" href={downloadUrl(file.id)} download>
            <span class="icon">{fileIcon(file.mime_type)}</span>
            <span class="row-name">{file.name}</span>
          </a>
          <span class="col-size">{formatBytes(file.size_bytes)}</span>
          <span class="col-date">{formatDate(file.created_at)}</span>
          <button
            class="row-action"
            title="Delete file"
            onclick={() => (confirmDelete = { type: 'file', id: file.id, name: file.name })}
          >
            &times;
          </button>
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Delete confirm modal -->
{#if confirmDelete}
  <!-- svelte-ignore a11y_interactive_supports_focus a11y_click_events_have_key_events -->
  <div class="modal-backdrop" role="dialog" aria-modal="true" onclick={() => (confirmDelete = null)}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <p>Delete <strong class="modal-filename">{confirmDelete.name}</strong>?</p>
      {#if confirmDelete.type === 'dir'}
        <p class="modal-hint">Directory must be empty.</p>
      {/if}
      <div class="modal-actions">
        <button class="btn btn-danger" onclick={handleDelete}>Delete</button>
        <button class="btn btn-ghost" onclick={() => (confirmDelete = null)}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

<!-- Activity feed -->
{#if events.length > 0}
  <aside class="activity">
    <h3>Activity</h3>
    <ul>
      {#each events as evt}
        <li>
          <span class="evt-type">{evt.event}</span>
          <span class="evt-name">{evt.name}</span>
          <span class="evt-time">{formatDate(evt.timestamp)}</span>
        </li>
      {/each}
    </ul>
  </aside>
{/if}

<style>
  /* Breadcrumb */
  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.125rem;
    margin-bottom: 1rem;
    flex-wrap: wrap;
    font-family: var(--font-mono);
  }
  .crumb {
    background: none;
    color: var(--color-text-muted);
    padding: 0.25rem 0.375rem;
    font-size: 0.8125rem;
    font-family: var(--font-mono);
  }
  .crumb:hover {
    color: var(--color-primary);
  }
  .crumb.active {
    color: var(--color-text);
    font-weight: 400;
  }
  .sep {
    color: var(--color-text-muted);
    font-size: 0.8125rem;
  }

  /* Toolbar */
  .toolbar {
    display: flex;
    gap: 0.75rem;
    margin-bottom: 1rem;
    flex-wrap: wrap;
  }

  /* Buttons */
  .btn {
    font-size: 0.8125rem;
    font-weight: 500;
    font-family: var(--font-mono);
  }
  .btn-primary {
    background: transparent;
    color: var(--color-primary);
    border: 1px solid var(--color-primary);
  }
  .btn-primary:hover {
    background: rgba(92, 224, 216, 0.08);
  }
  .btn-danger {
    background: var(--color-danger);
    color: #fff;
  }
  .btn-danger:hover {
    background: var(--color-danger-hover);
  }
  .btn-ghost {
    background: transparent;
    color: var(--color-text-muted);
  }
  .btn-ghost:hover {
    color: var(--color-text);
    background: var(--color-surface-hover);
  }
  .upload-btn {
    cursor: pointer;
  }

  /* New folder */
  .new-folder {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
    align-items: center;
  }
  .new-folder input {
    width: 250px;
    font-family: var(--font-mono);
    font-size: 0.8125rem;
  }

  /* Error */
  .error-banner {
    border-left: 2px solid var(--color-danger);
    color: var(--color-danger);
    padding: 0.5rem 0.75rem;
    margin-bottom: 1rem;
    font-family: var(--font-mono);
    font-size: 0.8125rem;
  }

  /* Drop zone */
  .drop-zone {
    min-height: 200px;
    transition: border-color 0.1s, background 0.1s;
  }
  .drop-zone.drag-over {
    border: 1px solid var(--color-primary);
    background: rgba(92, 224, 216, 0.04);
  }

  .empty {
    text-align: center;
    color: var(--color-text-muted);
    padding: 3rem 1rem;
    font-size: 0.875rem;
    font-family: var(--font-mono);
  }

  /* List layout */
  .list {
    display: flex;
    flex-direction: column;
  }
  .list-header {
    display: flex;
    align-items: center;
    padding: 0.375rem 0;
    border-bottom: 1px solid var(--color-border);
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
  }
  .list-row {
    display: flex;
    align-items: center;
    border-bottom: 1px solid var(--color-border);
    transition: background 0.1s;
  }
  .list-row:hover {
    background: var(--color-surface);
  }

  .row-main {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex: 1;
    padding: 0.5rem 0;
    text-decoration: none;
    color: inherit;
    background: none;
    border: none;
    text-align: left;
    cursor: pointer;
    min-width: 0;
    font-family: inherit;
  }
  .row-name {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 0.8125rem;
  }
  .col-name {
    flex: 1;
  }
  .col-size {
    width: 80px;
    text-align: right;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    color: var(--color-text-muted);
    flex-shrink: 0;
  }
  .col-date {
    width: 140px;
    text-align: right;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    color: var(--color-text-muted);
    flex-shrink: 0;
  }
  .col-action {
    width: 36px;
    flex-shrink: 0;
  }

  /* Icons */
  .icon {
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    color: var(--color-text-muted);
    text-transform: uppercase;
    width: 2rem;
    text-align: center;
    flex-shrink: 0;
  }

  .row-action {
    width: 36px;
    padding: 0.5rem 0;
    background: none;
    color: var(--color-text-muted);
    font-size: 1rem;
    line-height: 1;
    text-align: center;
    opacity: 0;
    transition: opacity 0.1s, color 0.1s;
  }
  .list-row:hover .row-action {
    opacity: 1;
  }
  .row-action:hover {
    color: var(--color-danger);
  }

  /* Modal */
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }
  .modal {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    padding: 1.25rem;
    min-width: 300px;
    max-width: 90vw;
  }
  .modal p {
    margin-bottom: 0.75rem;
    font-size: 0.875rem;
  }
  .modal-filename {
    font-family: var(--font-mono);
    color: var(--color-primary);
  }
  .modal-hint {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    font-family: var(--font-mono);
  }
  .modal-actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
  }

  /* Activity */
  .activity {
    margin-top: 2rem;
    border-top: 1px solid var(--color-border);
    padding-top: 0.75rem;
  }
  .activity h3 {
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    font-weight: 400;
    margin-bottom: 0.5rem;
  }
  .activity ul {
    list-style: none;
  }
  .activity li {
    display: flex;
    gap: 0.5rem;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    padding: 0.125rem 0;
    color: var(--color-text-muted);
  }
  .evt-type {
    color: var(--color-success);
    font-weight: 400;
    min-width: 80px;
  }
  .evt-name {
    flex: 1;
    color: var(--color-text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .evt-time {
    white-space: nowrap;
  }

  @media (max-width: 600px) {
    .col-date,
    .list-header .col-date {
      display: none;
    }
    .col-size {
      width: 60px;
    }
    .new-folder {
      flex-direction: column;
      align-items: stretch;
    }
    .new-folder input {
      width: 100%;
    }
  }
</style>
