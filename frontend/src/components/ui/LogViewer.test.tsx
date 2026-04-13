import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { LogViewer } from './LogViewer';

// Feature: frontend-design-system, Property 6: LogViewer renders lines with correct level classes

const logLineArb = fc.record({
  key: fc.string({ minLength: 1 }).filter(s => /^[a-zA-Z0-9]+$/.test(s)),
  text: fc.string({ minLength: 1 }),
  level: fc.constantFrom('ok' as const, 'error' as const),
});

describe('LogViewer', () => {
  // **Validates: Requirements 11.2**
  it('renders lines with correct level classes for any array of log lines', () => {
    fc.assert(
      fc.property(
        fc.array(logLineArb, { minLength: 1, maxLength: 10 }).filter(
          arr => new Set(arr.map(l => l.key)).size === arr.length,
        ),
        (lines) => {
          const { container } = render(<LogViewer lines={lines} />);

          const lineElements = container.querySelectorAll('.task-log-line');
          expect(lineElements.length).toBe(lines.length);

          lineElements.forEach((el, i) => {
            const expectedClass = `task-log-${lines[i].level}`;
            expect(el.className).toContain(expectedClass);
          });
        },
      ),
      { numRuns: 100 },
    );
  });
});
