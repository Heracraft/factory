// The debugging port the a11y run's Chromium listens on and the Lighthouse
// CLI attaches to. One fixed port is safe because the a11y config runs one
// worker; it is here rather than in the config so the spec can import it
// without importing the config.
export const CDP_PORT = 9333;
