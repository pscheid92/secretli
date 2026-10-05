import type { ReactNode, Ref } from "react";

interface PageTitleProps {
  children: ReactNode;
  /** The one line under the title, in the muted colour. */
  lead?: ReactNode;
  /** Lets a page move focus to its title, as the result page does. */
  ref?: Ref<HTMLHeadingElement>;
  tabIndex?: number;
}

/**
 * The one title a screen has, ending in the amber full stop. The stop is drawn
 * with CSS, so it is neither read out nor part of the heading's text.
 */
export default function PageTitle({ children, lead, ref, tabIndex }: PageTitleProps) {
  return (
    <div className="flex flex-col gap-3">
      <h1
        ref={ref}
        tabIndex={tabIndex}
        className="text-balance text-[clamp(2.125rem,4.4vw,3rem)] font-semibold leading-[1.04] tracking-[-0.04em] text-ink after:text-accent after:content-['.'] focus:outline-none"
      >
        {children}
      </h1>
      {lead && <p className="max-w-[34em] text-pretty text-lead text-muted">{lead}</p>}
    </div>
  );
}
