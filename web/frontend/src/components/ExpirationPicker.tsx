const OPTIONS = [
  { value: "5m", label: "5m" },
  { value: "10m", label: "10m" },
  { value: "15m", label: "15m" },
  { value: "1h", label: "1h" },
  { value: "4h", label: "4h" },
  { value: "12h", label: "12h" },
  { value: "1d", label: "1d" },
  { value: "3d", label: "3d" },
  { value: "7d", label: "7d" },
] as const;

interface ExpirationPickerProps {
  value: string;
  onChange: (value: string) => void;
}

export default function ExpirationPicker({ value, onChange }: ExpirationPickerProps) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {OPTIONS.map((opt) => (
        <button
          key={opt.value}
          type="button"
          onClick={() => onChange(opt.value)}
          className={`px-3 py-1.5 rounded-md text-xs font-semibold tracking-wide transition-all duration-150 focus:outline-none focus:ring-2 focus:ring-amber-400/30 ${
            value === opt.value
              ? "border border-transparent bg-amber-400 text-zinc-900"
              : "border border-zinc-200 dark:border-zinc-500/50 text-zinc-600 dark:text-zinc-100 hover:border-zinc-400 dark:hover:border-zinc-500 hover:text-zinc-800 dark:hover:text-white"
          }`}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
