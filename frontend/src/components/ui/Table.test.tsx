import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { TableRow } from './Table';

// Feature: frontend-design-system, Property 10: TableRow flash prop produces correct CSS class

describe('TableRow', () => {
  it('applies the correct flash CSS class for any flash value', () => {
    fc.assert(
      fc.property(
        fc.constantFrom('up' as const, 'down' as const, undefined),
        (flash) => {
          const { container } = render(
            <table>
              <tbody>
                <TableRow flash={flash}>
                  <td>cell</td>
                </TableRow>
              </tbody>
            </table>,
          );
          const tr = container.querySelector('tr')!;

          if (flash === 'up') {
            expect(tr.className).toContain('flash-up');
            expect(tr.className).not.toContain('flash-down');
          } else if (flash === 'down') {
            expect(tr.className).toContain('flash-down');
            expect(tr.className).not.toContain('flash-up');
          } else {
            expect(tr.className).not.toContain('flash-up');
            expect(tr.className).not.toContain('flash-down');
          }
        },
      ),
      { numRuns: 100 },
    );
  });
});
