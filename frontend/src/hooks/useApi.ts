import { DependencyList, useEffect, useState } from "react";

export function useApi<T>(loader: () => Promise<T>, deps: DependencyList = []) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;

    loader()
      .then((result) => {
        if (!active) {
          return;
        }
        setData(result);
        setLoading(false);
      })
      .catch((err: Error) => {
        if (!active) {
          return;
        }
        setError(err.message);
        setLoading(false);
      });

    return () => {
      active = false;
    };
  }, deps);

  return { data, error, loading };
}
