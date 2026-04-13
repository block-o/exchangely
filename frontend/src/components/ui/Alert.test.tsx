import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { Alert } from './Alert';

// Feature: frontend-design-system, Property 7: Alert level produces correct CSS class

const levels = ['info', 'warning', 'error'] as const;

describe('Alert', () => {
  // **Validates: Requirements 12.1, 12.8**
  it('renders correct role and level-specific CSS class for any valid level', () => {
    fc.assert(
      fc.property(fc.constantFrom(...levels), (level) => {
        const { container } = render(
          <Alert level={level}>Test message</Alert>,
        );

        const alertEl = container.querySelector('[role="alert"]');
        expect(alertEl).not.toBeNull();
        expect(alertEl!.className).toContain(`ui-alert--${level}`);
      }),
      { numRuns: 100 },
    );
  });
});
