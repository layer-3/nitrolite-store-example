/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ink: '#0d0d0d',
        paper: '#fffef7',
        yellow: {
          brand: '#fcd000',
          surface: '#fff7c7',
          line: '#d4b21d',
        },
      },
      boxShadow: {
        card: '0 18px 60px rgba(0, 0, 0, 0.10)',
        glow: '0 0 0 1px rgba(252, 208, 0, 0.35), 0 18px 50px rgba(252, 208, 0, 0.16)',
      },
      backgroundImage: {
        'hero-grid':
          'linear-gradient(rgba(13, 13, 13, 0.055) 1px, transparent 1px), linear-gradient(90deg, rgba(13, 13, 13, 0.055) 1px, transparent 1px)',
      },
    },
  },
  plugins: [],
}
