import type { FileRecord, Directory, DirectoryContents, StorageStats } from './types';

const BASE = '/api/v1';

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, opts);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

// Files

export const listFiles = (directoryId?: string | null): Promise<FileRecord[]> => {
  const params = new URLSearchParams();
  if (directoryId !== undefined) {
    params.set('directory_id', directoryId ?? '');
  }
  return request<FileRecord[]>(`/files?${params}`);
};

export const uploadFile = (
  file: File,
  directoryId?: string,
  onProgress?: (percent: number) => void
): Promise<FileRecord> => {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const form = new FormData();
    form.append('file', file);
    if (directoryId) form.append('directory_id', directoryId);

    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable && onProgress) {
        const percent = Math.round((e.loaded / e.total) * 100);
        onProgress(percent);
      }
    });

    xhr.addEventListener('load', () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch (e) {
          resolve(undefined as any);
        }
      } else {
        let error = `HTTP ${xhr.status}`;
        try {
          const body = JSON.parse(xhr.responseText);
          error = body.error || error;
        } catch (e) {}
        reject(new Error(error));
      }
    });

    xhr.addEventListener('error', () => reject(new Error('Network error')));
    xhr.addEventListener('abort', () => reject(new Error('Upload aborted')));

    xhr.open('POST', `${BASE}/files/upload`);
    xhr.send(form);
  });
};

export const updateFile = (id: string, data: { name?: string; directory_id?: string | null }): Promise<FileRecord> =>
  request<FileRecord>(`/files/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });

export const bulkDeleteFiles = (ids: string[]): Promise<void> =>
  request<void>('/files/bulk', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids }),
  });

export const deleteFile = (id: string): Promise<void> =>
  request<void>(`/files/${id}`, { method: 'DELETE' });

export const downloadUrl = (id: string): string => `${BASE}/files/${id}/download`;

export const thumbnailUrl = (id: string): string => `${BASE}/files/${id}/thumbnail`;

// Directories

export const listDirectories = (parentId?: string | null): Promise<Directory[]> => {
  const params = new URLSearchParams();
  if (parentId !== undefined) {
    params.set('parent_id', parentId ?? '');
  }
  return request<Directory[]>(`/directories?${params}`);
};

export const createDirectory = (name: string, parentId?: string): Promise<Directory> =>
  request<Directory>('/directories', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, parent_id: parentId || undefined }),
  });

export const getDirectoryContents = (id: string): Promise<DirectoryContents> =>
  request<DirectoryContents>(`/directories/${id}/contents`);

export const deleteDirectory = (id: string): Promise<void> =>
  request<void>(`/directories/${id}`, { method: 'DELETE' });

export const bulkDeleteDirectories = (ids: string[]): Promise<void> =>
  request<void>('/directories/bulk', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids }),
  });

export const updateDirectory = (id: string, data: { name?: string; parent_id?: string }): Promise<Directory> =>
  request<Directory>(`/directories/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });

// Storage

export const getStorageStats = (): Promise<StorageStats> =>
  request<StorageStats>('/storage/stats');
