import { cloneElement } from "react";
import type { ReactElement, ReactNode } from "react";

type ContextualHintProps = {
  text: string;
  children: ReactElement<{ className?: string; title?: string; "aria-label"?: string; children?: ReactNode }>;
  className?: string;
  ariaLabel?: string;
};

/**
 * Wrapper de dica contextual sem ícone visual.
 * Fase 2: evoluir para tooltip acessível dedicado em touch/mobile.
 */
export function ContextualHint({ text, children, className = "", ariaLabel }: ContextualHintProps) {
  const mergedClassName = [children.props.className, className].filter(Boolean).join(" ");
  return cloneElement(children, {
    title: text,
    "aria-label": ariaLabel ?? children.props["aria-label"],
    className: mergedClassName || undefined,
  });
}
