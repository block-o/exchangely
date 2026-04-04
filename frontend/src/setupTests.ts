import "@testing-library/jest-dom";

function createStorage(): Storage {
  const store = new Map<string, string>();

  return {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, value);
    },
  };
}

if (typeof globalThis.localStorage === "undefined" || typeof globalThis.localStorage.clear !== "function") {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: createStorage(),
  });
}

if (typeof globalThis.sessionStorage === "undefined" || typeof globalThis.sessionStorage.clear !== "function") {
  Object.defineProperty(globalThis, "sessionStorage", {
    configurable: true,
    value: createStorage(),
  });
}
