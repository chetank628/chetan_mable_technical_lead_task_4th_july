/**
 * Mable Tracker — a standalone, framework-agnostic, dependency-free web
 * tracking script. Drop it onto any site with:
 *
 *   <script src="/tracker.js"></script>
 *   <script>
 *     window.mable.init({ endpoint: "http://localhost:8080/api/events" });
 *   </script>
 *
 * It exposes `window.mable` and a `window.mableDataLayer` queue (Google-style
 * data layer) so calls made before the script loads are not lost.
 *
 * Design goals:
 *  - Never throw into the host UI. Every public call is wrapped in try/catch.
 *  - Non-blocking transport: navigator.sendBeacon, falling back to
 *    fetch(keepalive). Events are batched and flushed on a timer and on page
 *    hide / visibility change.
 *  - Resilient: events that fail to send are persisted to localStorage and
 *    replayed when the browser comes back online.
 *  - Privacy-aware: nothing is sent until consent is granted. The backend
 *    drops any event without `consent: true` anyway, but we also gate locally.
 *  - Contract-correct: matches the Go API's IngestEvent shape exactly, and
 *    coerces every `properties` value to a string (API requires
 *    map[string]string).
 *
 * Browser-vs-server data split (documented in scripts/README.md):
 *  - Collected in the browser: user agent, language, timezone, screen size,
 *    referrer, URL, session id, user id, and any event-specific properties.
 *  - Added server-side (never trust the client): client IP, server receipt
 *    time, and any geo derived from the IP.
 */
(function (window, document) {
  "use strict";

  // The canonical event types the backend ingests. Anything else is dropped by
  // the API's "tracked" filter stage, so we surface helpers only for these.
  var TRACKED_TYPES = {
    PageView: true,
    Click: true,
    AddToCart: true,
    Checkout: true,
    PaymentInfoAdded: true,
    Purchase: true,
    Lead: true,
  };

  var STORAGE_QUEUE_KEY = "mable_retry_queue";
  var SESSION_KEY = "mable_session_id";

  var config = {
    endpoint: "",
    consent: false,
    batchSize: 10, // flush when this many events are buffered
    flushIntervalMs: 5000, // ...or after this long
    maxQueueBytes: 64 * 1024, // cap persisted retry queue to avoid bloat
    debug: false,
  };

  var state = {
    initialized: false,
    userId: "",
    buffer: [], // events waiting to be sent
    flushTimer: null,
    lastPath: "", // for SPA route-change dedup
  };

  // -- small utilities -------------------------------------------------------

  function log() {
    if (!config.debug) return;
    try {
      var args = ["[mable]"].concat([].slice.call(arguments));
      console.debug.apply(console, args);
    } catch (e) {
      /* no-op */
    }
  }

  // Coerce any value to a string so `properties` satisfies the API's
  // map[string]string. Objects/arrays are JSON-stringified; null/undefined
  // become "".
  function toStringValue(v) {
    if (v === null || v === undefined) return "";
    if (typeof v === "string") return v;
    if (typeof v === "number" || typeof v === "boolean") return String(v);
    try {
      return JSON.stringify(v);
    } catch (e) {
      return String(v);
    }
  }

  function stringifyProps(props) {
    var out = {};
    if (!props || typeof props !== "object") return out;
    for (var k in props) {
      if (Object.prototype.hasOwnProperty.call(props, k)) {
        out[k] = toStringValue(props[k]);
      }
    }
    return out;
  }

  function uuid() {
    try {
      if (window.crypto && window.crypto.randomUUID) {
        return window.crypto.randomUUID();
      }
    } catch (e) {
      /* fall through */
    }
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function (c) {
      var r = (Math.random() * 16) | 0;
      var v = c === "x" ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    });
  }

  function getSessionId() {
    try {
      var id = window.sessionStorage.getItem(SESSION_KEY);
      if (!id) {
        id = uuid();
        window.sessionStorage.setItem(SESSION_KEY, id);
      }
      return id;
    } catch (e) {
      // sessionStorage can be unavailable (private mode / sandbox). Fall back
      // to an in-memory id for the page lifetime.
      if (!state._memSession) state._memSession = uuid();
      return state._memSession;
    }
  }

  // Ambient context the browser can see. The server adds IP + geo.
  function ambientContext() {
    var ctx = {};
    try {
      ctx.user_agent = navigator.userAgent || "";
    } catch (e) {}
    try {
      ctx.language = navigator.language || "";
    } catch (e) {}
    try {
      ctx.timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
    } catch (e) {}
    try {
      ctx.screen = window.screen.width + "x" + window.screen.height;
      ctx.viewport = window.innerWidth + "x" + window.innerHeight;
    } catch (e) {}
    return ctx;
  }

  // -- event construction ----------------------------------------------------

  // Build a fully-formed IngestEvent matching the Go API wire shape.
  function buildEvent(type, props) {
    var p = stringifyProps(props);
    var ctx = ambientContext();
    // Fold ambient context into properties (all strings already).
    if (ctx.timezone) p.timezone = ctx.timezone;
    if (ctx.language) p.language = ctx.language;
    if (ctx.screen) p.screen = ctx.screen;
    if (ctx.viewport) p.viewport = ctx.viewport;

    var amount = 0;
    var currency = "";
    if (props && props.amount !== undefined && props.amount !== null) {
      var n = Number(props.amount);
      if (!isNaN(n)) amount = n;
      delete p.amount; // promoted to top-level field
    }
    if (props && props.currency) {
      currency = String(props.currency);
      delete p.currency;
    }

    return {
      event_type: type,
      user_id: state.userId || "",
      session_id: getSessionId(),
      timestamp: new Date().toISOString(), // RFC3339
      url: location.href,
      referrer: document.referrer || "",
      user_agent: ctx.user_agent || "",
      amount: amount,
      currency: currency,
      properties: p,
      consent: config.consent === true,
    };
  }

  // -- transport -------------------------------------------------------------

  // Persisted retry queue (events that failed to send) ----------------------
  function loadRetryQueue() {
    try {
      var raw = window.localStorage.getItem(STORAGE_QUEUE_KEY);
      return raw ? JSON.parse(raw) : [];
    } catch (e) {
      return [];
    }
  }

  function saveRetryQueue(queue) {
    try {
      var serialized = JSON.stringify(queue);
      // Trim oldest entries if we exceed the byte cap.
      while (serialized.length > config.maxQueueBytes && queue.length > 1) {
        queue.shift();
        serialized = JSON.stringify(queue);
      }
      window.localStorage.setItem(STORAGE_QUEUE_KEY, serialized);
    } catch (e) {
      /* storage full / unavailable — drop silently, never block UI */
    }
  }

  function enqueueRetry(events) {
    var queue = loadRetryQueue();
    queue = queue.concat(events);
    saveRetryQueue(queue);
    log("queued for retry", events.length, "total", queue.length);
  }

  // Send a batch. Returns true if the transport accepted the handoff.
  // `useBeacon` is preferred during page unload.
  function sendBatch(events, useBeacon) {
    if (!config.endpoint || !events.length) return true;
    var body = JSON.stringify(events);

    // sendBeacon is fire-and-forget and survives page unload, but has a size
    // limit and no response. Use it on unload paths.
    if (useBeacon && navigator.sendBeacon) {
      try {
        var blob = new Blob([body], { type: "application/json" });
        var ok = navigator.sendBeacon(config.endpoint, blob);
        if (ok) {
          log("sent via beacon", events.length);
          return true;
        }
      } catch (e) {
        /* fall through to fetch */
      }
    }

    // fetch with keepalive lets the request outlive the page too, and gives us
    // a status code so we can distinguish retryable failures.
    try {
      fetch(config.endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: body,
        credentials: "include",
        keepalive: true,
      })
        .then(function (res) {
          if (res.status === 413 || res.status === 400) {
            // Oversized or malformed: retrying won't help, so drop it.
            log("dropped non-retryable", res.status, events.length);
            return;
          }
          if (!res.ok) {
            enqueueRetry(events); // 5xx / network-ish: retry later
            return;
          }
          log("sent via fetch", events.length);
        })
        .catch(function () {
          enqueueRetry(events); // network failure / API down
        });
      return true;
    } catch (e) {
      enqueueRetry(events);
      return false;
    }
  }

  // Replay any persisted retry queue (called on init and on 'online').
  function flushRetryQueue() {
    var queue = loadRetryQueue();
    if (!queue.length) return;
    if (!config.consent) return; // still no consent — keep holding
    log("replaying retry queue", queue.length);
    try {
      window.localStorage.removeItem(STORAGE_QUEUE_KEY);
    } catch (e) {}
    sendBatch(queue, false);
  }

  // -- buffering / flushing --------------------------------------------------

  function scheduleFlush() {
    if (state.flushTimer) return;
    state.flushTimer = window.setTimeout(function () {
      state.flushTimer = null;
      flush(false);
    }, config.flushIntervalMs);
  }

  function flush(useBeacon) {
    if (state.flushTimer) {
      window.clearTimeout(state.flushTimer);
      state.flushTimer = null;
    }
    if (!state.buffer.length) return;
    if (!config.consent) {
      // Without consent we never transmit. Hold in the persisted queue so a
      // later setConsent(true) can replay them (or they expire with storage).
      enqueueRetry(state.buffer.splice(0));
      return;
    }
    var batch = state.buffer.splice(0);
    sendBatch(batch, useBeacon === true);
  }

  function record(event) {
    state.buffer.push(event);
    if (state.buffer.length >= config.batchSize) {
      flush(false);
    } else {
      scheduleFlush();
    }
  }

  // -- public API ------------------------------------------------------------

  function track(type, props) {
    try {
      if (!state.initialized) {
        log("track() before init() — ignored", type);
        return;
      }
      if (!TRACKED_TYPES[type]) {
        log("untracked type, dropping", type);
        return;
      }
      var event = buildEvent(type, props || {});
      log("track", type, event);
      if (!config.consent) {
        // Buffer locally; will be held in the retry queue on flush.
        record(event);
        return;
      }
      record(event);
    } catch (e) {
      // Never let tracking break the host app.
      log("track error", e);
    }
  }

  function identify(userId) {
    try {
      state.userId = userId ? String(userId) : "";
      log("identify", state.userId);
    } catch (e) {}
  }

  function setConsent(granted) {
    try {
      config.consent = granted === true;
      log("setConsent", config.consent);
      if (config.consent) {
        // Consent just granted: drain anything we were holding.
        flush(false);
        flushRetryQueue();
      }
    } catch (e) {}
  }

  // -- auto-collectors -------------------------------------------------------

  function trackPageView() {
    var path = location.pathname + location.search;
    if (path === state.lastPath) return; // de-dup rapid duplicate fires
    state.lastPath = path;
    track("PageView", { path: location.pathname, title: document.title });
  }

  // Patch history so SPA route changes (pushState/replaceState) emit PageView.
  function patchHistory() {
    if (window.__mableHistoryPatched) return;
    window.__mableHistoryPatched = true;

    var wrap = function (orig) {
      return function () {
        var ret = orig.apply(this, arguments);
        try {
          window.dispatchEvent(new Event("mable:locationchange"));
        } catch (e) {}
        return ret;
      };
    };
    try {
      history.pushState = wrap(history.pushState);
      history.replaceState = wrap(history.replaceState);
    } catch (e) {}

    window.addEventListener("popstate", function () {
      trackPageView();
    });
    window.addEventListener("mable:locationchange", function () {
      trackPageView();
    });
  }

  // Delegated click tracking for elements annotated with data-track.
  // Example: <button data-track="AddToCart" data-track-id="42">Add</button>
  function patchClicks() {
    document.addEventListener(
      "click",
      function (e) {
        try {
          var el = e.target;
          while (el && el !== document.body) {
            if (el.getAttribute && el.getAttribute("data-track")) {
              var type = el.getAttribute("data-track");
              var props = {};
              for (var i = 0; i < el.attributes.length; i++) {
                var attr = el.attributes[i];
                if (attr.name.indexOf("data-track-") === 0) {
                  var key = attr.name.slice("data-track-".length);
                  props[key] = attr.value;
                }
              }
              if (!props.text) {
                props.text = (el.textContent || "").trim().slice(0, 80);
              }
              track(TRACKED_TYPES[type] ? type : "Click", props);
              break;
            }
            el = el.parentNode;
          }
        } catch (err) {
          /* never block clicks */
        }
      },
      true
    );
  }

  function bindLifecycle() {
    // Flush on tab hide / page unload using beacon so nothing is lost.
    document.addEventListener("visibilitychange", function () {
      if (document.visibilityState === "hidden") flush(true);
    });
    window.addEventListener("pagehide", function () {
      flush(true);
    });
    // Replay persisted failures when connectivity returns.
    window.addEventListener("online", function () {
      flushRetryQueue();
    });
  }

  // Convenience helpers for the canonical commerce events. Each accepts a
  // plain object; values are stringified into `properties` (amount/currency
  // are promoted to top-level fields).
  function makeHelper(type) {
    return function (props) {
      track(type, props || {});
    };
  }

  function init(opts) {
    try {
      opts = opts || {};
      if (opts.endpoint) config.endpoint = opts.endpoint;
      if (typeof opts.consent === "boolean") config.consent = opts.consent;
      if (typeof opts.batchSize === "number") config.batchSize = opts.batchSize;
      if (typeof opts.flushIntervalMs === "number")
        config.flushIntervalMs = opts.flushIntervalMs;
      if (typeof opts.debug === "boolean") config.debug = opts.debug;
      if (opts.userId) state.userId = String(opts.userId);

      if (state.initialized) {
        log("init() called twice — updating config only");
        return;
      }
      state.initialized = true;

      patchHistory();
      patchClicks();
      bindLifecycle();

      // Drain any data-layer queue calls made before load.
      drainDataLayer();

      // Initial page view + replay anything held from a previous session.
      trackPageView();
      flushRetryQueue();

      log("initialized", config);
    } catch (e) {
      log("init error", e);
    }
  }

  // The data layer lets callers push commands before the script is ready:
  //   window.mableDataLayer = window.mableDataLayer || [];
  //   window.mableDataLayer.push(["track", "Lead", { source: "hero" }]);
  function processCommand(cmd) {
    if (!cmd || !cmd.length) return;
    var name = cmd[0];
    var args = cmd.slice(1);
    if (typeof api[name] === "function") {
      api[name].apply(null, args);
    }
  }

  function drainDataLayer() {
    var dl = window.mableDataLayer || [];
    for (var i = 0; i < dl.length; i++) {
      processCommand(dl[i]);
    }
    // Replace the array's push so future pushes execute immediately.
    window.mableDataLayer = {
      push: function (cmd) {
        processCommand(cmd);
        return 0;
      },
    };
  }

  var api = {
    init: init,
    identify: identify,
    setConsent: setConsent,
    track: track,
    pageView: trackPageView,
    addToCart: makeHelper("AddToCart"),
    checkout: makeHelper("Checkout"),
    paymentInfoAdded: makeHelper("PaymentInfoAdded"),
    purchase: makeHelper("Purchase"),
    lead: makeHelper("Lead"),
    click: makeHelper("Click"),
    // Exposed for tests / introspection.
    _config: config,
    _state: state,
  };

  window.mable = api;

  // If a data layer already exists (pre-load pushes), keep it until init drains.
  window.mableDataLayer = window.mableDataLayer || [];
})(window, document);
