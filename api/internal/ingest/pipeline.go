package ingest

import (
	"strings"
	"time"

	"github.com/mable/mono/api/internal/config"
	"github.com/mable/mono/pipeline"
)

// trackedTypes is the whitelist of canonical event types the API ingests.
// Anything else is dropped by the "tracked" filter stage.
var trackedTypes = map[string]bool{
	"PageView": true, "Click": true, "AddToCart": true, "Checkout": true,
	"PaymentInfoAdded": true, "Purchase": true, "Lead": true,
}

// canonicalType maps case-insensitive event-type spellings onto the canonical
// PascalCase name. Normalising before the whitelist + ChurnEnrich keeps those
// downstream stages comparing against a single canonical form.
var canonicalType = map[string]string{
	"pageview": "PageView", "click": "Click", "addtocart": "AddToCart",
	"checkout": "Checkout", "paymentinfoadded": "PaymentInfoAdded",
	"purchase": "Purchase", "lead": "Lead",
}

const (
	maxURLLen      = 2048
	maxPropsKeys   = 64
	maxPropValueLn = 1024
)

// buildPipeline constructs one windowed pipeline run. The API and the library's
// own benchmark are thus two callers of the same library: this mirrors
// benchEventPipeline, adding the consent gate, normalisation, and enrichment
// the production ingest path needs.
//
// Stage order is deliberate: consent gate first (shed disallowed data ASAP),
// normalise (so the whitelist and ChurnEnrich see canonical types), enrich,
// churn, dedupe, then Collect into SQLite.
func buildPipeline(cfg config.Config) *pipeline.Pipeline[pipeline.Event] {
	opts := []pipeline.Option{
		pipeline.WithBatchSize(cfg.PipelineBatchSize),
		pipeline.WithChannelBufferDepth(cfg.PipelineChannelBuffer),
		pipeline.WithBatchTimeout(cfg.PipelineBatchTimeout),
		pipeline.WithErrorPolicy(pipeline.SkipAndCount),
	}
	if cfg.PipelineWorkers > 0 {
		opts = append(opts, pipeline.WithWorkerCount(cfg.PipelineWorkers))
	}

	return pipeline.New[pipeline.Event](opts...).
		Filter("consent", hasConsent).
		Map("normalize", normalize).
		Filter("tracked", func(e pipeline.Event) bool { return trackedTypes[e.EventType] }).
		Map("enrich_geo", enrichGeo).
		Stage(pipeline.NewChurnEnrich()).
		Deduplicate("dedup", dedupKey, cfg.DedupCapacity)
}

// hasConsent drops events that did not carry the consent flag. The handler
// stamps consent into the in-band control property "_consent".
func hasConsent(e pipeline.Event) bool {
	return e.Properties["_consent"] == "true"
}

// normalize trims and canonicalises an event: it fixes the event type's case,
// defaults a zero timestamp to now, trims/clamps oversized string fields, and
// bounds the properties map so a hostile payload cannot blow up memory.
func normalize(e pipeline.Event) pipeline.Event {
	e.EventType = strings.TrimSpace(e.EventType)
	if c, ok := canonicalType[strings.ToLower(e.EventType)]; ok {
		e.EventType = c
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	e.URL = clamp(strings.TrimSpace(e.URL), maxURLLen)
	e.Referrer = clamp(strings.TrimSpace(e.Referrer), maxURLLen)
	e.Currency = strings.ToUpper(strings.TrimSpace(e.Currency))
	if e.Amount < 0 {
		e.Amount = 0
	}
	e.Properties = clampProps(e.Properties)
	return e
}

// enrichGeo synthesises geo/timezone and device/browser fields from the IP and
// user-agent without any external calls, writing them into Properties.
func enrichGeo(e pipeline.Event) pipeline.Event {
	if e.Properties == nil {
		e.Properties = map[string]string{}
	}
	e.Properties["geo_region"] = geoFromIP(e.IP)
	e.Properties["timezone"] = "UTC"
	e.Properties["device"] = deviceFromUA(e.UserAgent)
	e.Properties["browser"] = browserFromUA(e.UserAgent)
	return e
}

// dedupKey is the bounded-LRU dedupe key: a replayed beacon for the same
// session + type within the same minute collapses to one event.
func dedupKey(e pipeline.Event) any {
	return e.SessionID + "|" + e.EventType + "|" + e.Timestamp.UTC().Format("2006-01-02T15:04")
}

func clamp(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

func clampProps(props map[string]string) map[string]string {
	if props == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(props))
	n := 0
	for k, v := range props {
		// Always preserve internal control keys (underscore-prefixed); they do
		// not count against the public key budget.
		if strings.HasPrefix(k, "_") {
			out[k] = v
			continue
		}
		if n >= maxPropsKeys {
			continue
		}
		out[k] = clamp(v, maxPropValueLn)
		n++
	}
	return out
}

// geoFromIP returns a synthetic region bucket derived from the IP's first
// octet. A real implementation would consult a GeoIP database.
func geoFromIP(ip string) string {
	if ip == "" {
		return "unknown"
	}
	switch {
	case strings.HasPrefix(ip, "10."), strings.HasPrefix(ip, "192.168."), strings.HasPrefix(ip, "127."):
		return "local"
	default:
		return "external"
	}
}

func deviceFromUA(ua string) string {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "mobile"), strings.Contains(l, "android"), strings.Contains(l, "iphone"):
		return "mobile"
	case strings.Contains(l, "ipad"), strings.Contains(l, "tablet"):
		return "tablet"
	case ua == "":
		return "unknown"
	default:
		return "desktop"
	}
}

func browserFromUA(ua string) string {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "edg"):
		return "edge"
	case strings.Contains(l, "chrome"):
		return "chrome"
	case strings.Contains(l, "firefox"):
		return "firefox"
	case strings.Contains(l, "safari"):
		return "safari"
	default:
		return "other"
	}
}
