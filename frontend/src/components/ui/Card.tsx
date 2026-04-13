import React from 'react';
import './Card.css';

type CardVariant = 'panel' | 'card';

type CardProps = {
  variant?: CardVariant;
  padding?: string;
} & React.HTMLAttributes<HTMLDivElement>;

export function Card({
  variant = 'panel',
  padding,
  className,
  style,
  children,
  ...rest
}: CardProps) {
  const classes = ['ui-card', `ui-card--${variant}`, className]
    .filter(Boolean)
    .join(' ');

  const mergedStyle = padding ? { ...style, padding } : style;

  return (
    <div className={classes} style={mergedStyle} {...rest}>
      {children}
    </div>
  );
}
