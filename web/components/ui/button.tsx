import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-semibold transition-[color,background,transform,border-color] outline-none disabled:pointer-events-none disabled:opacity-45 focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--ink)] active:translate-y-px",
  {
    variants: {
      variant: {
        default: "bg-[var(--signal)] text-[var(--ink)] hover:bg-[var(--signal-bright)]",
        secondary: "border border-[var(--line)] bg-[var(--panel)] text-[var(--paper)] hover:border-[var(--muted)] hover:bg-[var(--panel-raised)]",
        ghost: "text-[var(--muted)] hover:bg-white/5 hover:text-[var(--paper)]",
        danger: "bg-[var(--danger)] text-white hover:brightness-110",
      },
      size: {
        default: "h-10 px-5",
        sm: "h-8 px-3 text-xs",
        lg: "h-12 px-7 text-base",
        icon: "size-10 p-0",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<"button"> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : "button";
  return <Comp data-slot="button" className={cn(buttonVariants({ variant, size, className }))} {...props} />;
}
