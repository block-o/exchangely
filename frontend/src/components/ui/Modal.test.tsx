import { render, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { Modal } from './Modal';

describe('Modal', () => {
  it('invokes onClose when the backdrop is clicked', () => {
    const handleClose = vi.fn();
    const { container } = render(
      <Modal onClose={handleClose}>
        <p>Content</p>
      </Modal>,
    );
    const backdrop = container.querySelector('.modal-backdrop')!;
    fireEvent.click(backdrop);
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  it('invokes onClose when the Escape key is pressed', () => {
    const handleClose = vi.fn();
    render(
      <Modal onClose={handleClose}>
        <p>Content</p>
      </Modal>,
    );
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  it('sets role="dialog" on the dialog element', () => {
    const handleClose = vi.fn();
    const { container } = render(
      <Modal onClose={handleClose} title="Test Dialog">
        <p>Content</p>
      </Modal>,
    );
    const dialog = container.querySelector('[role="dialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog!.getAttribute('aria-label')).toBe('Test Dialog');
  });
});
