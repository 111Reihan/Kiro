// Adapted from ravikatiyar162/card-10 on 21st.dev: spring count-up, labelled region.
// Changes: optional suffix instead of a fixed "%", a plain caption instead of a trend
// arrow, and no animation when the user prefers reduced motion.
import * as React from "react";
import { animate, motion, useReducedMotion, useSpring, useTransform } from "framer-motion";
import { cn } from "@/lib/utils";

interface StatCardProps extends React.HTMLAttributes<HTMLDivElement> {
  title: string;
  value: number;
  suffix?: string;
  caption: string;
  icon: React.ReactNode;
  tone?: string; // tailwind classes for the icon chip
}

export function StatCard({ title, value, suffix, caption, icon, tone, className, ...props }: StatCardProps) {
  const reduce = useReducedMotion();
  const mv = useSpring(reduce ? value : 0, { damping: 100, stiffness: 100 });
  const shown = useTransform(mv, (v) => Math.round(v).toLocaleString());
  React.useEffect(() => {
    if (reduce) { mv.set(value); return; }
    const c = animate(mv, value, { duration: 1.2, ease: "easeOut" });
    return c.stop;
  }, [value, mv, reduce]);
  return (
    <div
      role="region"
      aria-label={`${title}: ${value}${suffix ?? ""}. ${caption}`}
      className={cn("flex flex-col gap-1 rounded-xl border bg-card p-5 shadow-soft", className)}
      {...props}
    >
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium text-muted-foreground">{title}</p>
        <span className={cn("grid h-8 w-8 place-items-center rounded-lg", tone)} aria-hidden="true">{icon}</span>
      </div>
      <div className="flex items-baseline gap-1">
        <motion.span className="text-4xl font-semibold tracking-tight tabular-nums">{shown}</motion.span>
        {suffix && <span className="text-xl font-medium text-muted-foreground">{suffix}</span>}
      </div>
      <p className="text-sm text-muted-foreground">{caption}</p>
    </div>
  );
}
