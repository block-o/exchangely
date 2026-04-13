import './StatusDot.css';

type StatusDotProps = {
  status: 'live' | 'offline';
  label?: string;
};

export function StatusDot({ status, label }: StatusDotProps) {
  const isLive = status === 'live';

  return (
    <span className={`market-stream-status${isLive ? ' is-live' : ''}`}>
      <span className="market-stream-dot" />
      {label && <span>{label}</span>}
    </span>
  );
}
