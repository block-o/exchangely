import React from 'react';
import './Badge.css';

type BadgeVariant = 'default' | 'success' | 'warning' | 'danger' | 'accent';

type BadgeProps = {
  variant?: BadgeVariant;
  children: React.ReactNode;
} & React.HTMLAttributes<HTMLSpanElement>;

export function Badge({
  variant = 'default',
  className,
  children,
  ...rest
}: BadgeProps) {
  const classes = ['ui-badge', `ui-badge--${variant}`, className]
    .filter(Boolean)
    .join(' ');

  return (
    <span className={classes} {...rest}>
      {children}
    </span>
  );
}
