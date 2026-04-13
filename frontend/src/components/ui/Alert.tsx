import type { ReactElement, ReactNode } from 'react';
import './Alert.css';

export type AlertLevel = 'info' | 'warning' | 'error';

export type AlertProps = {
  level: AlertLevel;
  title?: string;
  onDismiss?: () => void;
  children: ReactNode;
  className?: string;
};

export function Alert({ level, title, onDismiss, children, className }: AlertProps): ReactElement {
  const classes = ['ui-alert', `ui-alert--${level}`, className].filter(Boolean).join(' ');

  return (
    <div className={classes} role="alert">
      <div className="ui-alert__body">
        {title && <strong className="ui-alert__title">{title}</strong>}
        <div className="ui-alert__content">{children}</div>
      </div>
      {onDismiss && (
        <button
          type="button"
          className="ui-alert__dismiss"
          onClick={onDismiss}
          aria-label="Dismiss"
        >
          ✕
        </button>
      )}
    </div>
  );
}
