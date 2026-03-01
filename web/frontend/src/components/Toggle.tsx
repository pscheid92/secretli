interface ToggleProps {
  checked: boolean;
  onChange: () => void;
  label: string;
  description?: string;
}

export default function Toggle({ checked, onChange, label, description }: ToggleProps) {
  return (
    <div
      role="switch"
      aria-checked={checked}
      tabIndex={0}
      onClick={onChange}
      onKeyDown={(e) => { if (e.key === " " || e.key === "Enter") { e.preventDefault(); onChange(); } }}
      className="flex items-center justify-between gap-4 cursor-pointer select-none"
    >
      <div>
        <div className="text-sm text-zinc-800 dark:text-zinc-100">{label}</div>
        {description && (
          <div className="text-xs text-zinc-600 dark:text-zinc-100 mt-0.5">{description}</div>
        )}
      </div>
      <span
        aria-hidden
        className={`relative overflow-hidden flex-shrink-0 w-10 h-5 rounded-full transition-colors duration-200 ${
          checked ? "bg-amber-400" : "bg-zinc-200 dark:bg-zinc-700"
        }`}
      >
        <span
          className={`absolute left-0 top-0.5 w-4 h-4 rounded-full bg-white shadow-sm transition-transform duration-200 ${
            checked ? "translate-x-[22px]" : "translate-x-0.5"
          }`}
        />
      </span>
    </div>
  );
}
