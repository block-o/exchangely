import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { Badge } from './Badge';

// Feature: frontend-design-system, Property 2: Badge variant produces correct CSS class

const variants = ['default', 'success', 'warning', 'danger', 'accent'] as const;

describe('Badge', () => {
  // **Validates: Requirements 3.1**
  it('renders correct CSS class for any variant', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...variants),
        (variant) => {
          const { container } = render(
            <Badge variant={variant}>Test</Badge>,
          );
          const el = container.firstElementChild!;
          expect(el.tagName).toBe('SPAN');
          expect(el.className).toContain('ui-badge');
          expect(el.className).toContain(`ui-badge--${variant}`);
        },
      ),
      { numRuns: 100 },
    );
  });
});
