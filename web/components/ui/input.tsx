import * as React from "react";
import { cn } from "@/lib/utils";

export function Input({ className, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      data-slot="input"
      className={cn(
        "h-12 w-full rounded-2xl border border-[var(--line)] bg-black/20 px-4 text-[15px] text-[var(--paper)] outline-none transition placeholder:text-[var(--dim)] focus:border-[var(--accent)] focus:ring-4 focus:ring-[color:color-mix(in_oklab,var(--accent)_14%,transparent)]",
        className,
      )}
      {...props}
    />
  );
}
