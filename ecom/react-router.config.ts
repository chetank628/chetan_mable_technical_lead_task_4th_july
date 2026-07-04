import type { Config } from "@react-router/dev/config";

// SPA mode: no server-side rendering. The app is built to static assets and
// served on :5173, which matches the API's CORS allow-list.
export default {
  ssr: false,
} satisfies Config;
