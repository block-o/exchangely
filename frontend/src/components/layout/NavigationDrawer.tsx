import './NavigationDrawer.css';
import { useEffect, useRef, useCallback, type RefObject } from "react";

export interface NavigationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  activeHash: string;
  onNavigate: (hash: string) => void;
  /** Ref to the hamburger button for focus restoration on close */
  hamburgerRef?: RefObject<HTMLButtonElement | null>;
  /** Filtered nav items based on auth state */
  navItems: { id: string; label: string }[];
}

export function NavigationDrawer({
  isOpen,
  onClose,
  activeHash,
  onNavigate,
  hamburgerRef,
  navItems,
}: NavigationDrawerProps) {
  const drawerRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<Element | null>(null);

  const handleNavClick = useCallback(
    (hash: string) => {
      onNavigate(hash);
      onClose();
    },
    [onNavigate, onClose],
  );

  // Focus trap + Escape key handling
  useEffect(() => {
    if (!isOpen) return;

    previousFocusRef.current = document.activeElement;

    const drawer = drawerRef.current;
    if (drawer) {
      drawer.focus();
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
        return;
      }

      if (e.key === "Tab" && drawer) {
        const focusable = drawer.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
        );
        if (focusable.length === 0) return;

        const first = focusable[0];
        const last = focusable[focusable.length - 1];

        if (e.shiftKey) {
          if (document.activeElement === first) {
            e.preventDefault();
            last.focus();
          }
        } else {
          if (document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  // Restore focus to hamburger button on close
  useEffect(() => {
    if (!isOpen && previousFocusRef.current) {
      if (hamburgerRef?.current) {
        hamburgerRef.current.focus();
      }
      previousFocusRef.current = null;
    }
  }, [isOpen, hamburgerRef]);

  return (
    <>
      {/* Backdrop */}
      <div
        className={`nav-drawer-backdrop${isOpen ? " open" : ""}`}
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Drawer */}
      <div
        ref={drawerRef}
        id="nav-drawer"
        className={`nav-drawer${isOpen ? " open" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label="Navigation menu"
        tabIndex={-1}
      >
        {/* Close button */}
        <button
          className="icon-btn"
          onClick={onClose}
          aria-label="Close navigation menu"
          style={{ alignSelf: "flex-end", marginBottom: 8 }}
        >
          ✕
        </button>

        {/* Section links */}
        {navItems.map((item) => {
          const hash = `#${item.id}`;
          return (
            <a
              key={item.id}
              href={hash}
              className={`nav-item${activeHash === hash ? " active" : ""}`}
              onClick={(e) => {
                e.preventDefault();
                handleNavClick(hash);
              }}
            >
              {item.label}
            </a>
          );
        })}
      </div>
    </>
  );
}
