import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const badgeVariants = cva("status-badge", {
  variants: {
    tone: {
      info: "",
      success: "",
      warning: "",
      danger: "",
      neutral: "",
    },
  },
  defaultVariants: {
    tone: "neutral",
  },
});

export function Badge({
  className,
  tone,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  return <span data-tone={tone ?? "neutral"} className={cn(badgeVariants({ tone }), className)} {...props} />;
}
