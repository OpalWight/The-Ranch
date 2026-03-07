import type { FileEvent } from './types';

export function connectSSE(onEvent: (event: FileEvent) => void): EventSource {
  const source = new EventSource('/api/v1/events/stream');

  source.addEventListener('file_changed', (e: MessageEvent) => {
    try {
      const data: FileEvent = JSON.parse(e.data);
      onEvent(data);
    } catch (err) {
      console.error('Failed to parse SSE event:', err);
    }
  });

  source.onerror = () => {
    console.warn('SSE connection lost, will auto-reconnect...');
  };

  return source;
}
