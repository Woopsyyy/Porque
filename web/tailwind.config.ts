import type { Config } from "tailwindcss";

// "Royal Control Room" — dark cool-slate with a single crown-gold accent.
// Colors are driven by CSS variables (see src/index.css) so a future light
// theme is a token swap.
const config: Config = {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "rgb(var(--bg) / <alpha-value>)",
        surface: "rgb(var(--surface) / <alpha-value>)",
        "surface-2": "rgb(var(--surface-2) / <alpha-value>)",
        border: "rgb(var(--border) / <alpha-value>)",
        ink: "rgb(var(--ink) / <alpha-value>)",
        muted: "rgb(var(--muted) / <alpha-value>)",
        faint: "rgb(var(--faint) / <alpha-value>)",
        gold: "rgb(var(--gold) / <alpha-value>)",
        "gold-bright": "rgb(var(--gold-bright) / <alpha-value>)",
        "gold-deep": "rgb(var(--gold-deep) / <alpha-value>)",
        running: "rgb(var(--running) / <alpha-value>)",
        warn: "rgb(var(--warn) / <alpha-value>)",
        danger: "rgb(var(--danger) / <alpha-value>)",
        idle: "rgb(var(--idle) / <alpha-value>)",
      },
      fontFamily: {
        display: ['"Bricolage Grotesque"', "ui-sans-serif", "sans-serif"],
        sans: ['"Hanken Grotesk"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"JetBrains Mono"', "ui-monospace", "monospace"],
      },
      borderRadius: {
        lg: "0.7rem",
        md: "0.5rem",
        sm: "0.35rem",
      },
      boxShadow: {
        glow: "0 0 0 1px rgb(var(--gold) / 0.25), 0 0 28px -6px rgb(var(--gold) / 0.45)",
        panel: "0 1px 0 0 rgb(255 255 255 / 0.03) inset, 0 18px 40px -24px rgb(0 0 0 / 0.8)",
      },
      keyframes: {
        "fade-up": {
          from: { opacity: "0", transform: "translateY(8px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        pulsedot: {
          "0%, 100%": { opacity: "1" },
          "50%": { opacity: "0.35" },
        },
        sheen: {
          "0%": { backgroundPosition: "-200% 0" },
          "100%": { backgroundPosition: "200% 0" },
        },
      },
      animation: {
        "fade-up": "fade-up 0.5s cubic-bezier(0.16,1,0.3,1) both",
        pulsedot: "pulsedot 1.8s ease-in-out infinite",
        sheen: "sheen 2.5s linear infinite",
      },
    },
  },
  plugins: [require("tailwindcss-animate")],
};

export default config;
