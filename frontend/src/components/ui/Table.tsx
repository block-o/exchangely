import React from 'react';
import { useBreakpoint } from '../../hooks/useBreakpoint';

type TableProps = {
  mobileRender?: () => React.ReactNode;
} & React.HTMLAttributes<HTMLDivElement>;

type TableRowProps = {
  flash?: 'up' | 'down';
} & React.HTMLAttributes<HTMLTableRowElement>;

type TableCellProps = {
  align?: 'left' | 'center' | 'right';
} & React.TdHTMLAttributes<HTMLTableCellElement>;

export function Table({
  mobileRender,
  className,
  children,
  ...rest
}: TableProps) {
  const bp = useBreakpoint();
  const isMobile = bp === 'mobile';

  if (mobileRender && isMobile) {
    return (
      <div className={['data-table-wrapper', className].filter(Boolean).join(' ')} {...rest}>
        {mobileRender()}
      </div>
    );
  }

  return (
    <div className={['data-table-wrapper', className].filter(Boolean).join(' ')} {...rest}>
      <table className="data-table">
        {children}
      </table>
    </div>
  );
}

export function TableHead({
  children,
  ...rest
}: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <thead {...rest}>{children}</thead>;
}

export function TableBody({
  children,
  ...rest
}: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody {...rest}>{children}</tbody>;
}

export function TableRow({
  flash,
  className,
  children,
  ...rest
}: TableRowProps) {
  const classes = [
    'hoverable-row',
    flash ? `flash-${flash}` : undefined,
    className,
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <tr className={classes} {...rest}>
      {children}
    </tr>
  );
}

export function TableCell({
  align = 'center',
  style,
  children,
  ...rest
}: TableCellProps) {
  return (
    <td style={{ textAlign: align, ...style }} {...rest}>
      {children}
    </td>
  );
}
