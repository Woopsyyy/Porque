import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60 focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-45 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        primary:
          "bg-gold text-bg font-semibold shadow-[0_8px_20px_-10px_rgb(232_185_49_/_0.7)] hover:bg-gold-bright active:translate-y-px",
        secondary:
          "border border-border bg-surface-2 text-ink hover:border-faint hover:bg-surface-2/60",
        outline:
          "border border-border text-ink hover:border-gold/50 hover:text-gold",
        ghost: "text-muted hover:bg-surface-2 hover:text-ink",
        danger:
          "border border-danger/30 bg-danger/10 text-danger hover:bg-danger/20",
      },
      size: {
        sm: "h-8 px-3 text-[0.8rem]",
        md: "h-9 px-4 text-sm",
        lg: "h-11 px-6 text-sm",
        icon: "h-9 w-9",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
    );
  },
);
Button.displayName = "Button";

export { buttonVariants };
