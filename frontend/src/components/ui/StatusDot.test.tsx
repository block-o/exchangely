import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { StatusDot } from './StatusDot';

// Feature: frontend-design-system, Property 11: StatusDot status produces correct CSS class

const statuses = ['live', 'offline'] as const;

describe('StatusDot', () => {
  // **Validates: Requirements 9.1**
  it('renders correct CSS class for any status value', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...statuses),
        (status) => {
          const { container } = render(<StatusDot status={status} />);
          const el = container.firstElementChild!;

          expect(el.className).toContain('market-stream-status');

          if (status === 'live') {
            expect(el.className).toContain('is-live');
          } else {
            expect(el.className).not.toContain('is-live');
          }
        },
      ),
      { numRuns: 100 },
    );
  });
});
