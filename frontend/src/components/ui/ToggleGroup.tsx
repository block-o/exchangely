import './ToggleGroup.css';
import React from 'react';

type ToggleOption = { value: string; label: string };

type ToggleGroupProps = {
  options: ToggleOption[];
  value: string;
  onChange: (value: string) => void;
} & Omit<React.HTMLAttributes<HTMLDivElement>, 'onChange'>;

export function ToggleGroup({
  options,
  value,
  onChange,
  className,
  ...rest
}: ToggleGroupProps) {
  const classes = ['toggle-group', className].filter(Boolean).join(' ');

  return (
    <div className={classes} role="tablist" {...rest}>
      {options.map((option) => (
        <button
          key={option.value}
          role="tab"
          aria-selected={option.value === value}
          className={option.value === value ? 'active' : undefined}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
