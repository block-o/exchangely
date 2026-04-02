import type { Pair } from "../../types/api";

type Props = {
  pairs: Pair[];
};

export function PairsTable({ pairs }: Props) {
  return (
    <table className="data-table">
      <thead>
        <tr>
          <th>Pair</th>
          <th>Base</th>
          <th>Quote</th>
        </tr>
      </thead>
      <tbody>
        {pairs.map((pair) => (
          <tr key={pair.symbol}>
            <td>{pair.symbol}</td>
            <td>{pair.base}</td>
            <td>{pair.quote}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
