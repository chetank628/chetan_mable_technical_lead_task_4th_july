// Thin, typed wrapper around the global `window.mable` tracker loaded from
// /tracker.js. The app never imports the tracker source directly — it calls the
// global, exactly as any third-party site would. This keeps the tracker truly
// framework-agnostic and loadable via <script src>.

export type TrackedType =
  | "PageView"
  | "Click"
  | "AddToCart"
  | "Checkout"
  | "PaymentInfoAdded"
  | "Purchase"
  | "Lead";

export interface TrackerProps {
  [key: string]: unknown;
  amount?: number;
  currency?: string;
}

export interface MableTracker {
  init(opts: {
    endpoint: string;
    consent?: boolean;
    batchSize?: number;
    flushIntervalMs?: number;
    debug?: boolean;
    userId?: string;
  }): void;
  identify(userId: string): void;
  setConsent(granted: boolean): void;
  track(type: TrackedType, props?: TrackerProps): void;
  pageView(): void;
  addToCart(props?: TrackerProps): void;
  checkout(props?: TrackerProps): void;
  paymentInfoAdded(props?: TrackerProps): void;
  purchase(props?: TrackerProps): void;
  lead(props?: TrackerProps): void;
  click(props?: TrackerProps): void;
}

declare global {
  interface Window {
    mable?: MableTracker;
    mableDataLayer?: unknown[] | { push: (cmd: unknown[]) => number };
  }
}

const ENDPOINT =
  import.meta.env.VITE_TRACKER_ENDPOINT ||
  "http://localhost:8080/api/events";

// Commands issued before /tracker.js has executed are queued here and flushed
// (in order, directly onto window.mable) as soon as it is available. We do NOT
// use the raw window.mableDataLayer array: the tracker only drains that array
// from inside init(), so pushing init() onto it would deadlock.
const pending: unknown[][] = [];
let ready = false;
let flushScheduled = false;

function flushPending() {
  if (!ready) return;
  const m = window.mable as unknown as Record<string, (...a: unknown[]) => void>;
  while (pending.length) {
    const [name, ...args] = pending.shift() as [string, ...unknown[]];
    try {
      m[name]?.(...args);
    } catch {
      /* never throw into the host UI */
    }
  }
}

function ensureReady(): boolean {
  if (ready) return true;
  if (typeof window !== "undefined" && window.mable) {
    ready = true;
    flushPending();
    return true;
  }
  // /tracker.js not parsed yet — retry shortly until it is.
  if (typeof window !== "undefined" && !flushScheduled) {
    flushScheduled = true;
    const tick = () => {
      flushScheduled = false;
      if (!ensureReady() && pending.length) {
        flushScheduled = true;
        window.setTimeout(tick, 50);
      }
    };
    window.setTimeout(tick, 50);
  }
  return false;
}

// Invoke a tracker method directly if loaded, else queue it for replay.
function call(name: string, ...args: unknown[]) {
  if (typeof window === "undefined") return;
  if (ensureReady()) {
    const m = window.mable as unknown as Record<
      string,
      (...a: unknown[]) => void
    >;
    try {
      m[name]?.(...args);
    } catch {
      /* no-op */
    }
  } else {
    pending.push([name, ...args]);
  }
}

let initialized = false;

export const tracker = {
  init(consent: boolean) {
    if (initialized) {
      this.setConsent(consent);
      return;
    }
    initialized = true;
    call("init", {
      endpoint: ENDPOINT,
      consent,
      debug: import.meta.env.DEV,
    });
  },

  identify(userId: string) {
    call("identify", userId);
  },

  setConsent(granted: boolean) {
    call("setConsent", granted);
  },

  track(type: TrackedType, props?: TrackerProps) {
    call("track", type, props || {});
  },

  pageView() {
    call("pageView");
  },

  addToCart(props: TrackerProps) {
    call("addToCart", props);
  },

  checkout(props: TrackerProps) {
    call("checkout", props);
  },

  paymentInfoAdded(props: TrackerProps) {
    call("paymentInfoAdded", props);
  },

  purchase(props: TrackerProps) {
    call("purchase", props);
  },

  lead(props: TrackerProps) {
    call("lead", props);
  },

  click(props: TrackerProps) {
    call("click", props);
  },
};
