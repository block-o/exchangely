export function formatUnix(value: number) {
  return new Date(value * 1000).toLocaleString();
}

export function formatNumber(value: number) {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 2,
  }).format(value);
}
