export interface FileRecord {
  id: string;
  name: string;
  size_bytes: number;
  mime_type: string;
  checksum: string;
  storage_key?: string;
  directory_id?: string;
  status: string;
  thumbnail_key?: string;
  processed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Directory {
  id: string;
  name: string;
  parent_id?: string;
  created_at: string;
  updated_at: string;
}

export interface DirectoryContents {
  directory?: Directory;
  directories: Directory[];
  files: FileRecord[];
  breadcrumb: Directory[];
}

export interface StorageStats {
  file_count: number;
  total_bytes: number;
}

export interface FileEvent {
  event: string;
  file_id: string;
  name: string;
  timestamp: string;
}
