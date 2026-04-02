import type { PropsWithChildren } from "react";
import { sections } from "../../app/router";

export function AppShell({ children }: PropsWithChildren) {
  return (
    <div className="app-shell">
      <header className="hero">
        <div>
          <p className="eyebrow">Exchangely</p>
          <h1>Historical crypto market data, structured for retrieval.</h1>
        </div>
        <nav className="top-nav">
          {sections.map((section) => (
            <a key={section.id} href={`#${section.id}`}>
              {section.label}
            </a>
          ))}
        </nav>
      </header>
      <main className="content">{children}</main>
    </div>
  );
}
