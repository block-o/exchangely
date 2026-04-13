import { render, fireEvent } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect, vi } from 'vitest';
import { Button } from './Button';

// Feature: frontend-design-system, Property 1: Button variant and size produce correct CSS classes

const variants = ['primary', 'secondary', 'danger', 'ghost', 'icon'] as const;
const sizes = ['sm', 'md', 'lg'] as const;

describe('Button', () => {
  it('renders correct CSS classes for any variant and size combination', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...variants),
        fc.constantFrom(...sizes),
        (variant, size) => {
          const { container } = render(
            <Button variant={variant} size={size}>
              Test
            </Button>,
          );
          const el = container.firstElementChild!;
          expect(el.className).toContain('ui-btn');
          expect(el.className).toContain(`ui-btn--${variant}`);
          expect(el.className).toContain(`ui-btn--${size}`);
        },
      ),
      { numRuns: 100 },
    );
  });

  describe('disabled state', () => {
    it('does not invoke onClick when disabled', () => {
      const handleClick = vi.fn();
      const { getByRole } = render(
        <Button disabled onClick={handleClick}>
          Disabled
        </Button>,
      );
      fireEvent.click(getByRole('button'));
      expect(handleClick).not.toHaveBeenCalled();
    });

    it('applies the ui-btn class and disabled attribute for reduced opacity styling', () => {
      const { getByRole } = render(<Button disabled>Disabled</Button>);
      const btn = getByRole('button');
      expect(btn).toBeDisabled();
      expect(btn.className).toContain('ui-btn');
    });
  });
});
