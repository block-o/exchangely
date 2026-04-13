import { render, fireEvent } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect, vi } from 'vitest';
import { ToggleGroup } from './ToggleGroup';

// Feature: frontend-design-system, Property 4: ToggleGroup renders correct active state and ARIA attributes
// Feature: frontend-design-system, Property 5: ToggleGroup onChange fires with clicked segment value

const optionArb = fc.array(
  fc.record({
    value: fc.string({ minLength: 1 }).filter(s => /^[a-zA-Z0-9]+$/.test(s)),
    label: fc.string({ minLength: 1 }).filter(s => /^[a-zA-Z0-9]+$/.test(s)),
  }),
  { minLength: 1, maxLength: 5 }
).filter(arr => new Set(arr.map(o => o.value)).size === arr.length);

describe('ToggleGroup', () => {
  // **Validates: Requirements 8.1, 8.5**
  it('renders correct active state and ARIA attributes for any options and selected value', () => {
    fc.assert(
      fc.property(
        optionArb.chain(options =>
          fc.nat({ max: options.length - 1 }).map(idx => ({
            options,
            selectedValue: options[idx].value,
          }))
        ),
        ({ options, selectedValue }) => {
          const { container } = render(
            <ToggleGroup options={options} value={selectedValue} onChange={() => {}} />,
          );

          const tablist = container.querySelector('[role="tablist"]');
          expect(tablist).not.toBeNull();

          const tabs = container.querySelectorAll('[role="tab"]');
          expect(tabs.length).toBe(options.length);

          const selectedTabs = container.querySelectorAll('[aria-selected="true"]');
          expect(selectedTabs.length).toBe(1);
          expect(selectedTabs[0].textContent).toBe(
            options.find(o => o.value === selectedValue)!.label,
          );

          tabs.forEach(tab => {
            const isSelected = tab.getAttribute('aria-selected') === 'true';
            if (isSelected) {
              expect(tab.className).toContain('active');
            }
          });
        },
      ),
      { numRuns: 100 },
    );
  });

  // **Validates: Requirements 8.2**
  it('onChange fires with clicked segment value for any options and clicked segment', () => {
    fc.assert(
      fc.property(
        optionArb.chain(options =>
          fc.nat({ max: options.length - 1 }).map(clickIdx => ({
            options,
            clickIdx,
          }))
        ),
        ({ options, clickIdx }) => {
          const handleChange = vi.fn();
          const { container } = render(
            <ToggleGroup options={options} value={options[0].value} onChange={handleChange} />,
          );

          const tabs = container.querySelectorAll('[role="tab"]');
          fireEvent.click(tabs[clickIdx]);

          expect(handleChange).toHaveBeenCalledTimes(1);
          expect(handleChange).toHaveBeenCalledWith(options[clickIdx].value);
        },
      ),
      { numRuns: 100 },
    );
  });
});
