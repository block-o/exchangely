import { fetchPairs } from "../api/pairs";
import { PairsTable } from "../components/tables/PairsTable";
import { useApi } from "../hooks/useApi";

export function PairsPage() {
  const { data, error, loading } = useApi(fetchPairs);

  return (
    <section id="pairs" className="panel">
      <div className="panel-header">
        <h2>Tracked Pairs</h2>
        <p>Configured crypto base assets crossed with the supported quote assets.</p>
      </div>
      {loading ? <p>Loading pairs...</p> : null}
      {error ? <p className="error">{error}</p> : null}
      {data ? <PairsTable pairs={data.data} /> : null}
    </section>
  );
}
