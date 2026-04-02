type Props = {
  label: string;
  value: string;
};

export function StatusCard({ label, value }: Props) {
  return (
    <article className="status-card">
      <span>{label}</span>
      <strong>{value}</strong>
    </article>
  );
}
