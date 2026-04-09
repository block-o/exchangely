import { useEffect, useState } from "react";

export type Breakpoint = "mobile" | "tablet" | "desktop";

const MOBILE_QUERY = "(max-width: 639px)";
const TABLET_QUERY = "(min-width: 640px) and (max-width: 1023px)";

function getBreakpoint(
  mobile: MediaQueryList,
  tablet: MediaQueryList,
): Breakpoint {
  if (mobile.matches) return "mobile";
  if (tablet.matches) return "tablet";
  return "desktop";
}

export function useBreakpoint(): Breakpoint {
  const [breakpoint, setBreakpoint] = useState<Breakpoint>(() => {
    if (typeof window === "undefined" || !window.matchMedia) return "desktop";
    return getBreakpoint(
      window.matchMedia(MOBILE_QUERY),
      window.matchMedia(TABLET_QUERY),
    );
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;

    const mobileList = window.matchMedia(MOBILE_QUERY);
    const tabletList = window.matchMedia(TABLET_QUERY);

    const update = () => setBreakpoint(getBreakpoint(mobileList, tabletList));

    mobileList.addEventListener("change", update);
    tabletList.addEventListener("change", update);

    return () => {
      mobileList.removeEventListener("change", update);
      tabletList.removeEventListener("change", update);
    };
  }, []);

  return breakpoint;
}
