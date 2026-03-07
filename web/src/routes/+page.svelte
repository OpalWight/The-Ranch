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
    Root
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
    <p class="empty">This folder is empty. Drop files here or use the buttons above.</p>
  {:else}
    <div class="grid">
      <!-- Directories -->
      {#each directories as dir}
        <div class="card dir-card">
          <button class="card-main" onclick={() => navigateTo(dir.id)}>
            <span class="icon folder-icon">dir</span>
            <span class="card-name">{dir.name}</span>
          </button>
          <button
            class="card-action danger"
            title="Delete folder"
            onclick={() => (confirmDelete = { type: 'dir', id: dir.id, name: dir.name })}
          >
            &times;
          </button>
        </div>
      {/each}

      <!-- Files -->
      {#each files as file}
        <div class="card file-card">
          <a class="card-main" href={downloadUrl(file.id)} download>
            <span class="icon file-icon">{fileIcon(file.mime_type)}</span>
            <span class="card-name">{file.name}</span>
            <span class="card-meta">{formatBytes(file.size_bytes)}</span>
          </a>
          <button
            class="card-action danger"
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
      <p>Delete <strong>{confirmDelete.name}</strong>?</p>
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

<!-- Activity sidebar -->
{#if events.length > 0}
  <aside class="activity">
    <h3>Recent Activity</h3>
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
    gap: 0.25rem;
    margin-bottom: 1rem;
    flex-wrap: wrap;
  }
  .crumb {
    background: none;
    color: var(--color-primary);
    padding: 0.25rem 0.5rem;
    font-size: 0.875rem;
  }
  .crumb:hover {
    text-decoration: underline;
  }
  .crumb.active {
    color: var(--color-text);
    font-weight: 600;
  }
  .sep {
    color: var(--color-text-muted);
    font-size: 0.875rem;
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
    font-size: 0.875rem;
    font-weight: 500;
  }
  .btn-primary {
    background: var(--color-primary);
    color: #fff;
  }
  .btn-primary:hover {
    background: var(--color-primary-hover);
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
  }

  /* Error */
  .error-banner {
    background: rgba(239, 68, 68, 0.15);
    color: var(--color-danger);
    padding: 0.75rem 1rem;
    border-radius: var(--radius-sm);
    margin-bottom: 1rem;
    font-size: 0.875rem;
  }

  /* Drop zone */
  .drop-zone {
    border: 2px dashed var(--color-border);
    border-radius: var(--radius);
    padding: 1.5rem;
    min-height: 300px;
    transition: border-color 0.15s, background 0.15s;
  }
  .drop-zone.drag-over {
    border-color: var(--color-primary);
    background: rgba(59, 130, 246, 0.05);
  }

  .empty {
    text-align: center;
    color: var(--color-text-muted);
    padding: 3rem 1rem;
    font-size: 0.95rem;
  }

  /* Grid */
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 0.75rem;
  }

  /* Card */
  .card {
    display: flex;
    align-items: center;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
    transition: border-color 0.15s;
  }
  .card:hover {
    border-color: var(--color-primary);
  }
  .card-main {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    flex: 1;
    padding: 0.75rem;
    text-decoration: none;
    color: inherit;
    background: none;
    border: none;
    text-align: left;
    cursor: pointer;
    min-width: 0;
  }
  .card-name {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 0.875rem;
  }
  .card-meta {
    color: var(--color-text-muted);
    font-size: 0.75rem;
    white-space: nowrap;
  }
  .card-action {
    padding: 0.75rem;
    background: none;
    color: var(--color-text-muted);
    font-size: 1.125rem;
    line-height: 1;
    border-radius: 0;
  }
  .card-action.danger:hover {
    color: var(--color-danger);
    background: rgba(239, 68, 68, 0.1);
  }

  /* Icons */
  .icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border-radius: var(--radius-sm);
    font-size: 0.625rem;
    font-weight: 700;
    text-transform: uppercase;
    flex-shrink: 0;
  }
  .folder-icon {
    background: rgba(245, 158, 11, 0.2);
    color: var(--color-warning);
  }
  .file-icon {
    background: rgba(59, 130, 246, 0.2);
    color: var(--color-primary);
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
    border-radius: var(--radius);
    padding: 1.5rem;
    min-width: 300px;
    max-width: 90vw;
  }
  .modal p {
    margin-bottom: 0.75rem;
  }
  .modal-hint {
    font-size: 0.8rem;
    color: var(--color-text-muted);
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
    padding-top: 1rem;
  }
  .activity h3 {
    font-size: 0.875rem;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }
  .activity ul {
    list-style: none;
  }
  .activity li {
    display: flex;
    gap: 0.5rem;
    font-size: 0.8rem;
    padding: 0.25rem 0;
    color: var(--color-text-muted);
  }
  .evt-type {
    color: var(--color-success);
    font-weight: 500;
    min-width: 90px;
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
    .grid {
      grid-template-columns: 1fr;
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
