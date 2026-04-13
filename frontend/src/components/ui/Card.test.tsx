import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { Card } from './Card';

// Feature: frontend-design-system, Property 3: Card variant produces correct CSS class with className forwarding

const variants = ['panel', 'card'] as const;

describe('Card', () => {
  // **Validates: Requirements 4.1, 4.5**
  it('renders correct variant CSS class and forwards className for any variant and className combination', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...variants),
        fc.string().filter((s) => /^[a-zA-Z0-9]+$/.test(s) && s.length > 0),
        (variant, extraClass) => {
          const { container } = render(
            <Card variant={variant} className={extraClass}>
              Content
            </Card>,
          );
          const el = container.firstElementChild!;
          expect(el.className).toContain('ui-card');
          expect(el.className).toContain(`ui-card--${variant}`);
          expect(el.className).toContain(extraClass);
        },
      ),
      { numRuns: 100 },
    );
  });

  it('defaults to panel variant when variant prop is omitted', () => {
    const { container } = render(<Card>Content</Card>);
    const el = container.firstElementChild!;
    expect(el.className).toContain('ui-card--panel');
  });
});
