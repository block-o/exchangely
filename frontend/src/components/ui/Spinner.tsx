import './Spinner.css';

type SpinnerSize = 'sm' | 'md' | 'lg';

type SpinnerProps = {
  size?: SpinnerSize;
  label?: string;
};

const sizeMap: Record<SpinnerSize, number> = {
  sm: 20,
  md: 32,
  lg: 48,
};

export function Spinner({ size = 'md', label }: SpinnerProps) {
  const px = sizeMap[size];

  return (
    <div className="auth-loading-spinner">
      <div
        className="auth-spinner"
        style={{ width: px, height: px }}
        role="status"
        aria-label={label ?? 'Loading'}
      />
      {label && <span>{label}</span>}
    </div>
  );
}
