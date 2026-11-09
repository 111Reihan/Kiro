/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        border: "hsl(var(--border))", card: "hsl(var(--card))", "card-foreground": "hsl(var(--fg))",
        muted: "hsl(var(--muted))", "muted-foreground": "hsl(var(--muted-fg))",
        foreground: "hsl(var(--fg))", accent: "hsl(var(--accent))", "accent-soft": "hsl(var(--accent-soft))",
        ring: "hsl(var(--accent))",
      },
      fontFamily: {
        sans: ["ui-sans-serif", "system-ui", "-apple-system", "Segoe UI", "Roboto", "Helvetica Neue", "Arial", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Consolas", "monospace"],
      },
      boxShadow: { soft: "0 1px 2px rgba(16,24,40,.04), 0 4px 16px -4px rgba(16,24,40,.06)" },
    },
  },
  plugins: [],
};
