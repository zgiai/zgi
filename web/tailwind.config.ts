import type { Config } from 'tailwindcss';

const config: Config = {
  darkMode: ['class'],
  content: [
    './pages/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './src/**/*.{js,ts,jsx,tsx,mdx}',
    './.local-overrides/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    container: {
      center: true,
      padding: '2rem',
      screens: {
        '2xl': '1400px',
      },
    },
    extend: {
      colors: {
        // Core theme colors
        border: 'var(--border)',
        input: 'var(--input)',
        ring: 'var(--ring)',
        background: 'var(--background)',
        foreground: 'var(--foreground)',

        // Brand colors
        primary: 'var(--primary)',
        'primary-foreground': 'var(--primary-foreground)',
        'primary-hover': 'var(--primary-hover)',
        'primary-active': 'var(--primary-active)',
        secondary: 'var(--secondary)',
        'secondary-foreground': 'var(--secondary-foreground)',
        'secondary-hover': 'var(--secondary-hover)',
        'secondary-active': 'var(--secondary-active)',

        // Status colors
        destructive: 'var(--destructive)',
        'destructive-foreground': 'var(--destructive-foreground)',
        warning: 'var(--warning)',
        'warning-foreground': 'var(--warning-foreground)',
        success: 'var(--success)',
        'success-foreground': 'var(--success-foreground)',
        info: 'var(--info)',
        'info-foreground': 'var(--info-foreground)',

        // Neutral colors
        muted: 'var(--muted)',
        'muted-foreground': 'var(--muted-foreground)',
        accent: 'var(--accent)',
        'accent-foreground': 'var(--accent-foreground)',

        // Surface colors
        popover: 'var(--popover)',
        'popover-foreground': 'var(--popover-foreground)',
        card: 'var(--card)',
        'card-foreground': 'var(--card-foreground)',
        'bg-canvas': 'var(--bg-canvas)',
        'bg-surface': 'var(--bg-surface)',
        'bg-subtle': 'var(--bg-subtle)',

        // Sidebar colors
        sidebar: 'var(--sidebar)',
        'sidebar-foreground': 'var(--sidebar-foreground)',
        'sidebar-primary': 'var(--sidebar-primary)',
        'sidebar-primary-foreground': 'var(--sidebar-primary-foreground)',
        'sidebar-accent': 'var(--sidebar-accent)',
        'sidebar-accent-foreground': 'var(--sidebar-accent-foreground)',
        'sidebar-border': 'var(--sidebar-border)',
        'sidebar-ring': 'var(--sidebar-ring)',

        // Border variants
        'border-strong': 'var(--border-strong)',
        'border-subtle': 'var(--border-subtle)',

        // Chart colors
        'chart-1': 'var(--chart-1)',
        'chart-2': 'var(--chart-2)',
        'chart-3': 'var(--chart-3)',
        'chart-4': 'var(--chart-4)',
        'chart-5': 'var(--chart-5)',
        
        // Custom highlight color
        highlight: 'var(--highlight)',

        // Brand accent colors (for info panels, focus states, etc.)
        'brand-subtle': 'var(--brand-subtle)',
        'brand-main': 'var(--brand-main)',
        'brand-strong': 'var(--brand-strong)',
      },

      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },

      boxShadow: {
        sm: 'var(--shadow-sm)',
        DEFAULT: 'var(--shadow-md)',
        md: 'var(--shadow-md)',
        lg: 'var(--shadow-lg)',
        xl: 'var(--shadow-xl)',
        tooltip: '0 8px 16px -4px rgba(0, 0, 0, 0.1), 0 4px 8px -2px rgba(0, 0, 0, 0.06)',
      },

      transitionDuration: {
        base: 'var(--transition-base)',
        fast: 'var(--transition-fast)',
        slow: 'var(--transition-slow)',
      },

      keyframes: {
        'accordion-down': {
          from: { height: '0' },
          to: { height: 'var(--radix-accordion-content-height)' },
        },
        'accordion-up': {
          from: { height: 'var(--radix-accordion-content-height)' },
          to: { height: '0' },
        },
        'fade-in': {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        'fade-out': {
          from: { opacity: '1' },
          to: { opacity: '0' },
        },
        'slide-in-from-top': {
          from: { transform: 'translateY(-100%)' },
          to: { transform: 'translateY(0)' },
        },
        'slide-in-from-bottom': {
          from: { transform: 'translateY(100%)' },
          to: { transform: 'translateY(0)' },
        },
        'slide-in-from-left': {
          from: { transform: 'translateX(-100%)' },
          to: { transform: 'translateX(0)' },
        },
        'slide-in-from-right': {
          from: { transform: 'translateX(100%)' },
          to: { transform: 'translateX(0)' },
        },
        'slide-out-to-bottom': {
          from: { transform: 'translateY(0)' },
          to: { transform: 'translateY(100%)' },
        },
        'slide-out-to-top': {
          from: { transform: 'translateY(0)' },
          to: { transform: 'translateY(-100%)' },
        },
        'float-in-top': {
          '0%': { transform: 'scale(0.95) translateY(12px)', opacity: '0' },
          '70%': { transform: 'scale(1.02) translateY(-2px)', opacity: '1' },
          '100%': { transform: 'scale(1) translateY(0)' },
        },
        'float-in-bottom': {
          '0%': { transform: 'scale(0.95) translateY(-12px)', opacity: '0' },
          '70%': { transform: 'scale(1.02) translateY(2px)', opacity: '1' },
          '100%': { transform: 'scale(1) translateY(0)' },
        },
        'float-in-left': {
          '0%': { transform: 'scale(0.95) translateX(12px)', opacity: '0' },
          '70%': { transform: 'scale(1.02) translateX(-2px)', opacity: '1' },
          '100%': { transform: 'scale(1) translateX(0)' },
        },
        'float-in-right': {
          '0%': { transform: 'scale(0.95) translateX(-12px)', opacity: '0' },
          '70%': { transform: 'scale(1.02) translateX(2px)', opacity: '1' },
          '100%': { transform: 'scale(1) translateX(0)' },
        },
        'float-out-top': {
          '0%': { opacity: '1', transform: 'scale(1)' },
          '100%': { opacity: '0', transform: 'scale(0.95) translateY(4px)' },
        },
        'float-out-bottom': {
          '0%': { opacity: '1', transform: 'scale(1)' },
          '100%': { opacity: '0', transform: 'scale(0.95) translateY(-4px)' },
        },
        'float-out-left': {
          '0%': { opacity: '1', transform: 'scale(1)' },
          '100%': { opacity: '0', transform: 'scale(0.95) translateX(4px)' },
        },
        'float-out-right': {
          '0%': { opacity: '1', transform: 'scale(1)' },
          '100%': { opacity: '0', transform: 'scale(0.95) translateX(-4px)' },
        },
        'pulse-fast': {
          '0%, 100%': { opacity: '1', transform: 'scale(1)' },
          '50%': { opacity: '0.4', transform: 'scale(0.8)' },
        },
        'pulse-subtle': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.6' },
        },
        'flow-gradient': {
          '0%': { backgroundPosition: '0% 50%' },
          '100%': { backgroundPosition: '200% 50%' },
        },
      },

      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
        'fade-in': 'fade-in 0.2s ease-out',
        'fade-out': 'fade-out 0.2s ease-out',
        'slide-in-from-top': 'slide-in-from-top 0.3s ease-out',
        'slide-in-from-bottom': 'slide-in-from-bottom 0.3s ease-out',
        'slide-in-from-left': 'slide-in-from-left 0.3s ease-out',
        'slide-in-from-right': 'slide-in-from-right 0.3s ease-out',
        'slide-out-to-bottom': 'slide-out-to-bottom 0.3s ease-out',
        'slide-out-to-top': 'slide-out-to-top 0.3s ease-out',
        'float-in-top': 'float-in-top 0.4s ease-out',
        'float-in-bottom': 'float-in-bottom 0.4s ease-out',
        'float-in-left': 'float-in-left 0.4s ease-out',
        'float-in-right': 'float-in-right 0.4s ease-out',
        'float-out-top': 'float-out-top 0.2s ease-in',
        'float-out-bottom': 'float-out-bottom 0.2s ease-in',
        'float-out-left': 'float-out-left 0.2s ease-in',
        'float-out-right': 'float-out-right 0.2s ease-in',
        'pulse-subtle': 'pulse-subtle 2s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'pulse-fast': 'pulse-fast 1s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'flow-gradient': 'flow-gradient 3s linear infinite',
      },
    },
  },
  plugins: [
    require('tailwindcss-animate'),
    require('@tailwindcss/container-queries'),
    // Custom plugin for theme utilities
    function ({ addUtilities }: { addUtilities: (utilities: Record<string, unknown>) => void }) {
      const newUtilities = {
        '.theme-transition': {
          transition:
            'background-color var(--transition-base), border-color var(--transition-base), color var(--transition-base)',
        },
        '.theme-focus': {
          '&:focus': {
            outline: 'none',
            'box-shadow': '0 0 0 2px var(--ring)',
          },
        },
      };
      addUtilities(newUtilities);
    },
    // Plugin to generate opacity utilities for ALL theme colors that use CSS variables (e.g., bg-primary/10, border-highlight/30)
    // eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types, @typescript-eslint/no-explicit-any
    function ({ addUtilities, e, theme }: any) {
      // Collect all color keys whose values are CSS variables: var(--token)
      const themeColors = theme('colors') as Record<string, string | Record<string, string>>;
      const colorKeys: string[] = Object.entries(themeColors)
        .filter(([_key, value]) => typeof value === 'string' && /^var\(--[A-Za-z0-9-]+\)$/.test(value))
        .map(([key]) => key);

      const opacities = [5, 10, 20, 30, 40, 50, 60, 70, 80, 90];

      const utilities: Record<string, Record<string, string | number>> = {};

      colorKeys.forEach(color => {
        opacities.forEach(opacity => {
          const mix = `color-mix(in oklch, var(--${color}) ${opacity}%, transparent)`;

          // Background utilities: bg-color/opacity
          utilities[`.${e(`bg-${color}/${opacity}`)}`] = {
            'background-color': mix,
          };

          // Border utilities: border-color/opacity
          utilities[`.${e(`border-${color}/${opacity}`)}`] = {
            'border-color': mix,
          };

          // Directional border utilities
          utilities[`.${e(`border-l-${color}/${opacity}`)}`] = {
            'border-left-color': mix,
          };
          utilities[`.${e(`border-r-${color}/${opacity}`)}`] = {
            'border-right-color': mix,
          };
          utilities[`.${e(`border-t-${color}/${opacity}`)}`] = {
            'border-top-color': mix,
          };
          utilities[`.${e(`border-b-${color}/${opacity}`)}`] = {
            'border-bottom-color': mix,
          };

          // Text color utilities: text-color/opacity
          utilities[`.${e(`text-${color}/${opacity}`)}`] = {
            'color': mix,
          };

          // Text decoration color utilities: decoration-color/opacity
          utilities[`.${e(`decoration-${color}/${opacity}`)}`] = {
            'text-decoration-color': mix,
          };

          // Shadow utilities: shadow-color/opacity
          utilities[`.${e(`shadow-${color}/${opacity}`)}`] = {
            '--tw-shadow-color': mix,
            '--tw-shadow': 'var(--tw-shadow-colored)',
            'box-shadow':
              'var(--tw-ring-offset-shadow, 0 0 #0000), var(--tw-ring-shadow, 0 0 #0000), var(--tw-shadow)',
          };

          // Ring utilities: ring-color/opacity
          utilities[`.${e(`ring-${color}/${opacity}`)}`] = {
            '--tw-ring-color': mix,
          };
        });
      });

      addUtilities(utilities);
    },
  ],
};

export default config;
