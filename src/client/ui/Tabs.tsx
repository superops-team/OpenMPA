import type { ReactNode } from 'react';

export interface TabOption<T extends string> {
  id: T;
  label: string;
  icon?: ReactNode;
}

export function Tabs<T extends string>({
  label,
  group,
  value,
  options,
  onChange,
}: {
  label: string;
  group: string;
  value: T;
  options: readonly TabOption<T>[];
  onChange: (value: T) => void;
}) {
  return (
    <div className="ui-tabs" role="tablist" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          role="tab"
          id={`${group}-${option.id}-tab`}
          aria-controls={`${group}-${option.id}-panel`}
          aria-selected={value === option.id}
          tabIndex={value === option.id ? 0 : -1}
          onClick={() => onChange(option.id)}
          onKeyDown={(event) => {
            if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key))
              return;
            event.preventDefault();
            const current = options.findIndex((item) => item.id === option.id);
            const next =
              event.key === 'Home'
                ? 0
                : event.key === 'End'
                  ? options.length - 1
                  : (current +
                      (event.key === 'ArrowRight' ? 1 : -1) +
                      options.length) %
                    options.length;
            const id = options[next].id;
            onChange(id);
            requestAnimationFrame(() =>
              document.getElementById(`${group}-${id}-tab`)?.focus(),
            );
          }}
        >
          {option.icon}
          <span>{option.label}</span>
        </button>
      ))}
    </div>
  );
}

export function TabPanel({
  group,
  id,
  active,
  children,
}: {
  group: string;
  id: string;
  active: boolean;
  children: ReactNode;
}) {
  if (!active) return null;
  return (
    <section
      className="ui-tab-panel"
      role="tabpanel"
      id={`${group}-${id}-panel`}
      aria-labelledby={`${group}-${id}-tab`}
      tabIndex={0}
    >
      {children}
    </section>
  );
}
