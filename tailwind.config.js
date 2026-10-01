/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./**/*.templ", // <-- Critical for Templ templates
    "./**/*.go",
    "./**/*.html",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}