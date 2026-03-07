import type { FileRecord, Directory, DirectoryContents } from './types';

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

export const uploadFile = (file: File, directoryId?: string): Promise<FileRecord> => {
  const form = new FormData();
  form.append('file', file);
  if (directoryId) form.append('directory_id', directoryId);
  return request<FileRecord>('/files/upload', { method: 'POST', body: form });
};

export const deleteFile = (id: string): Promise<void> =>
  request<void>(`/files/${id}`, { method: 'DELETE' });

export const downloadUrl = (id: string): string => `${BASE}/files/${id}/download`;

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

export const updateDirectory = (id: string, data: { name?: string; parent_id?: string }): Promise<Directory> =>
  request<Directory>(`/directories/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
