import './LogViewer.css';
import type { ReactElement } from 'react';

export type LogLine = { key: string; text: string; level?: 'ok' | 'error' };

export type LogViewerProps = {
  lines: LogLine[];
  emptyMessage?: string;
  'aria-label'?: string;
};

export function LogViewer({ lines, emptyMessage, 'aria-label': ariaLabel }: LogViewerProps): ReactElement {
  return (
    <div className="task-log-viewer" role="log" aria-label={ariaLabel}>
      {lines.length === 0 && emptyMessage ? (
        <div className="task-log-line">{emptyMessage}</div>
      ) : (
        lines.map((line) => (
          <div
            key={line.key}
            className={`task-log-line${line.level ? ` task-log-${line.level}` : ''}`}
          >
            {line.text}
          </div>
        ))
      )}
    </div>
  );
}
