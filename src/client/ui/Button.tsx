import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';

export type ButtonVariant =
  'primary' | 'outline' | 'secondary' | 'ghost' | 'destructive' | 'warning';
export type ButtonSize =
  | 'xs'
  | 'sm'
  | 'md'
  | 'lg'
  | 'input'
  | 'content'
  | 'icon-sm'
  | 'icon-md'
  | 'icon-lg';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  leading?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      className = '',
      variant = 'outline',
      size = 'md',
      leading,
      type = 'button',
      children,
      ...props
    },
    ref,
  ) {
    return (
      <button
        ref={ref}
        type={type}
        data-ui="button"
        data-variant={variant}
        data-size={size}
        className={`ui-button ${className}`.trim()}
        {...props}
      >
        {leading}
        {children}
      </button>
    );
  },
);

export function IconButton({
  label,
  children,
  size = 'icon-md',
  ...props
}: Omit<ButtonProps, 'aria-label' | 'size'> & {
  label: string;
  children: ReactNode;
  size?: Extract<ButtonSize, 'icon-sm' | 'icon-md' | 'icon-lg'>;
}) {
  return (
    <Button aria-label={label} title={label} size={size} {...props}>
      {children}
    </Button>
  );
}
