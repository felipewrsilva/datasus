import { Info } from "lucide-react";

type ContextualHintProps = {
  text: string;
  className?: string;
};

/**
 * Ajuda discreta via tooltip nativo (title) e aria-label para leitores de tela.
 */
export function ContextualHint({ text, className = "" }: ContextualHintProps) {
  return (
    <button
      type="button"
      className={`inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--border)] bg-[var(--card)] text-[var(--muted)] transition hover:text-[var(--foreground)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] ${className}`}
      title={text}
      aria-label={text}
    >
      <Info className="h-3 w-3" aria-hidden />
    </button>
  );
}
