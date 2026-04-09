import { useEffect, useRef, useCallback, type RefObject } from "react";
import { sections } from "../../app/router";
import { API_BASE_URL } from "../../api/client";

export interface NavigationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  activeHash: string;
  onNavigate: (hash: string) => void;
  /** Ref to the hamburger button for focus restoration on close */
  hamburgerRef?: RefObject<HTMLButtonElement | null>;
}

function getApiDocsUrl() {
  try {
    const apiUrl = new URL(API_BASE_URL);
    return new URL("/swagger", apiUrl.origin).toString();
  } catch {
    return "/swagger";
  }
}

export function NavigationDrawer({
  isOpen,
  onClose,
  activeHash,
  onNavigate,
  hamburgerRef,
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

    // Store the previously focused element
    previousFocusRef.current = document.activeElement;

    // Focus the drawer itself initially
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

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
    };
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

  const apiDocsUrl = getApiDocsUrl();

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
        {sections.map((section) => {
          const hash = `#${section.id}`;
          return (
            <a
              key={section.id}
              href={hash}
              className={`nav-item${activeHash === hash ? " active" : ""}`}
              onClick={(e) => {
                e.preventDefault();
                handleNavClick(hash);
              }}
            >
              {section.label}
            </a>
          );
        })}

        {/* API Docs external link */}
        <a
          className="nav-item external-nav-item"
          href={apiDocsUrl}
          target="_blank"
          rel="noreferrer"
        >
          API Docs
        </a>

        {/* Spacer */}
        <div style={{ flex: 1 }} />

        {/* Bottom actions: GitHub + Settings */}
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <a
            className="icon-link"
            href="https://github.com/block-o/exchangely"
            target="_blank"
            rel="noreferrer"
            title="GitHub"
            aria-label="GitHub project"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="currentColor"
              aria-hidden="true"
            >
              <path d="M12 1.5a10.5 10.5 0 0 0-3.32 20.46c.53.1.72-.23.72-.51v-1.98c-2.94.64-3.56-1.25-3.56-1.25-.48-1.22-1.18-1.54-1.18-1.54-.96-.65.07-.64.07-.64 1.06.07 1.62 1.08 1.62 1.08.94 1.61 2.47 1.14 3.07.87.1-.68.37-1.14.67-1.4-2.35-.27-4.82-1.17-4.82-5.22 0-1.15.41-2.08 1.08-2.82-.11-.27-.47-1.37.1-2.86 0 0 .88-.28 2.89 1.08a10 10 0 0 1 5.26 0c2.01-1.36 2.89-1.08 2.89-1.08.57 1.49.21 2.59.1 2.86.67.74 1.08 1.67 1.08 2.82 0 4.06-2.47 4.94-4.83 5.21.38.33.72.98.72 1.98v2.93c0 .28.19.62.73.51A10.5 10.5 0 0 0 12 1.5Z" />
            </svg>
          </a>
        </div>
      </div>
    </>
  );
}
