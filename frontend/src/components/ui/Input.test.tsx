import { render } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { Input } from './Input';

describe('Input', () => {
  it('associates label htmlFor with input id when label prop is provided', () => {
    const { container } = render(<Input label="Email" />);
    const label = container.querySelector('label')!;
    const input = container.querySelector('input')!;
    expect(label).not.toBeNull();
    expect(label.getAttribute('for')).toBe(input.id);
    expect(input.id).toBeTruthy();
  });

  it('uses a custom id for both label and input when id prop is provided', () => {
    const { container } = render(<Input label="Email" id="custom-id" />);
    const label = container.querySelector('label')!;
    const input = container.querySelector('input')!;
    expect(label.getAttribute('for')).toBe('custom-id');
    expect(input.id).toBe('custom-id');
  });

  it('does not render a label when label prop is omitted', () => {
    const { container } = render(<Input placeholder="Type here" />);
    const label = container.querySelector('label');
    expect(label).toBeNull();
  });
});
