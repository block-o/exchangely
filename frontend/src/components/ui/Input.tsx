import React, { useId } from 'react';
import './Input.css';

type InputProps = {
  label?: string;
} & React.InputHTMLAttributes<HTMLInputElement>;

export function Input({ label, id, className, ...rest }: InputProps) {
  const autoId = useId();
  const inputId = id ?? autoId;

  const inputClasses = ['ui-input', className].filter(Boolean).join(' ');

  return (
    <>
      {label && (
        <label htmlFor={inputId} className="ui-input-label">
          {label}
        </label>
      )}
      <input id={inputId} className={inputClasses} {...rest} />
    </>
  );
}
