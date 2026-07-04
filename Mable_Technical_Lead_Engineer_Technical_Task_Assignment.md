# Mable Technical Lead Engineer - Technical Task Assignment

## General Information
* **Title:** Mable Technical Lead Engineer - Technical Task
* **Deadline for Submission:** EOD (Sunday 05.07.2026, 11:59 p.m. IST), a good week from the date you receive this task.
* **Official Start Date:** The official start date of the assignment is agreed upon in advance.
* **Prerequisites:** Clear any doubts with Horst Wenske (horst@mable.ai).

---

## Task Overview
For this task, you will build and deploy a small full-stack system and a reusable Go library, benchmark the library, and—as the Technical Lead we are hiring—document the decisions and trade-offs behind it.

### Suggested Stack
*(You can make your own tech stack decisions)*

* **Frontend (FE):**
  * React Router 7 (Remix) & React 19
  * Zustand (for FE State Management)
  * JavaScript for Browser APIs
  * Tailwind CSS (use React Icons and other free graphics for ease)
  * Vite (for bundling)
  * PNPM (for package management)
* **Backend (BE):**
  * Go
  * Gin Gonic API Framework
  * An analytics database (e.g., Clickhouse)
* **Deployment:**
  * Cloudflare Workers or Pages (Pages is fine for the Remix frontend)
  * API deployment platform of your choice (e.g., Render, Fly.io, Railway); the Go API may be deployed separately and is not required to run on Cloudflare Workers.
* **Observability:**
  * Grafana

---

## Detailed Task List

### 1. AI Coding Strategy
Frame this as a Technical Lead would for the team. Cover the following dimensions:
* **Scope of usage:** What AI coding would you let an engineer do unsupervised?
* **Guardrails:** What must go through review, and how do you keep the quality bar?
* **Tooling:** What tools will be utilized?

### 2. Full-Stack Data Pipeline & Reusable Library

#### A. Demo E-Commerce Application (Remix SPA)
Build a demo e-commerce application (can be a Single Page Application using a random free API to populate with dummy products) in Remix.
* **UI/UX Requirements:** Create a basic, but functional UI/UX for:
  * Sign Up and Login Page
  * Product Library
  * Cart Operations
  * Checkout
* **Styling:** Style using Tailwind CSS.
* **Flow:** Simulate checkout flow, and return the user to the product library when successful.
* **Authentication:** Use the Go API to serve a simple sign-up/login flow using JWT-based authentication, with the token stored in an `HttpOnly` cookie.
* **Frontend Best Practices:**
  * Apply accessibility guidelines (semantic HTML, labels, keyboard navigation).
  * Implement form validation with clear error messaging.
  * Provide explicit loading, empty, and error states for every data-driven view.
  * Keep tracking calls non-blocking (they must never block or break the UI).
  * Do not store sensitive auth state (e.g., JWTs) in client-accessible storage like `localStorage`.
* **Complex Workflow:** Include at least one moderately complex UI workflow (e.g., a multi-step checkout, offline/retry behavior for failed requests, or session restoration after a page refresh).
* **Frontend Observability:** Add React error boundaries, client-side error tracking, structured logging, and write a short note on your monitoring strategy for the FE.
* **Testing Strategy:** Describe (and where practical, implement) a frontend testing strategy across unit, integration, and end-to-end (E2E) levels, stating what should and should not be tested. A basic E2E test with a tool such as Playwright is a plus.
* **Codebase Scaling:** In `DESIGN.md`, explain how you would scale the frontend codebase for multiple engineers and future features (structure, state management, conventions, code ownership).

#### B. Tracking Script
Implement a tracking script on the demo e-commerce application.
* **Technology:** Use the Standard Web APIs to build the tracking logic and data layer.
* **Event Tracking:** Ensure that your script can track user events like:
  * Clicks
  * Page Views
  * Add To Cart
  * Checkout
  * Payment Info Added
  * Purchase
  * Lead (Email Form Submission)
  * *[OPTIONAL]* Implement any other events that you think are valuable for user tracking.
* **Data Parameter Tracking:** Track user data parameters such as:
  * User Agent
  * IP Address
  * CartData and other details from the Checkout Form
  * Userdata
  * Location
  * Timezone
  * Details from the logged-in user session

#### C. Backend API Service (Go & Gin)
Build a Go API using the Gin Gonic framework to process and persist tracking data.
* **Ingestion:** Receive events from the tracking script.
* **Pipeline Integration:** Process received events through the generic pipeline library before persistence. Every event the API ingests must flow through a `Pipeline[Event]` featuring at minimum validation/normalization and enrichment stages. Only the final pipeline output is written to the analytics DB. The library must be a working part of the ingest path, not a standalone demo.
* **Authentication Service:** Serve basic authentication (signup and login) via JWT Tokens issued in an `HttpOnly` cookie. (State your CORS and cross-site cookie strategy, since the SPA and API are on different origins).
* **Health Check & Metrics:** Serve a lightweight liveness/readiness check at a standard endpoint (`/health`) returning a `200` status + minimal JSON. If you expose Prometheus-style metrics, serve them separately at `/metrics`.
* **Analytics Storage:** Dump tracked event metadata to the analytics DB and calculate statistics such as:
  * Average event capture time
  * Average event parameters
  * Events tracked over time
  * Event counts over time for each event type
  * *[OPTIONAL]* Any other stats you can think of.

#### D. Observability & Visualization (Grafana)
* *[OPTIONAL]* Create a visualization dashboard in Grafana to visualize the different analytics data crunched in the previous step.
* **Reproducibility:** Make the dashboard reproducible (a runnable `docker compose up` plus screenshots is sufficient; a publicly accessible link like Grafana Cloud is a nice-to-have but not required).
* **Views:** Create a standard view featuring the 3 most important statistics you choose in relevant graphs.

#### E. Deployment & Walkthrough
* Deploy your applications and record a demo video walkthrough demonstrating the functionality of the data pipeline on deployed versions of the individual applications.
* *[OPTIONAL]* Profile your apps (BE and FE) and send in benchmarks using industry-standard best practices.

---

## 3. Generic, Concurrent Data-Pipeline Library (Go)
Build a generic, concurrent data-pipeline library in Go as a systems-design deliverable. The tracking API must consume this pipeline to process events on the way to the analytics DB, meaning the per-stage metadata comes from that same in-API pipeline run. Treat the standalone benchmark and the in-API integration as two callers of one library.

### Type & Component Requirements
* **Generics:** A pipeline is generic over a single element type—`Pipeline[T]`—and is composed of stages.
* **Supported Stage Types:**
  * **`Map[T]`**: Transforms a `T` into a `T` (`func(T) T`).
  * **`Filter[T]`**: Drops events for which a predicate is false (`func(T) bool`).
  * **`Generate[T]`**: A 1→N stage: given one `T`, emits the original plus zero-or-more newly produced `T` downstream.
  * **`If[T]`**: Routes each `T` into one of two sub-pipelines (both `Pipeline[T]`), merging their outputs back into the downstream `T` stream.
  * **`Reduce[T, R]`**: Terminates a pipeline by folding a stream of `T` into a single (or keyed) `R` sink. *Note: Reduce is a sink and is not chainable into further T stages. State whether your reduce is per-batch or global, and why.*
  * **`Collect[T]`**: Drains the stream into a caller-provided, bounded sink (document its back-pressure behavior).
* **Typing Model:** Decide the typing model and document the trade-off. A homogeneous element type (`Pipeline[T]`) vs. type-erasure (`any`) at stage boundaries are both acceptable. Justify the design choice made.
* **Concurrency & Scaling:** The library must support dynamic batching and fan-out across worker goroutines, with the most important hyperparameters configurable (e.g., batch size, worker count, channel buffer depth).
* *[OPTIONAL]* Make one or more hyperparameters self-tuning from runtime signals (e.g., adjust worker count from observed queue depth).
* **Extensibility:** Provide a protocol/interface for future developers to add new stage types without modifying the core. Demonstrate it with at least one example stage.

### Metadata & Instrumentation
* Have the library emit metadata per stage, not just one timing for the whole pipeline (e.g., per-stage latency, batch size, throughput, and error/drop counts).
* Ingest this metadata into the analytics DB alongside the tracking events. Emitting per-stage errors and dropped events, not only latency, is required.
* *[OPTIONAL]* Surface this pipeline metadata on the same Grafana dashboard as the tracking analytics.

### Tests & Benchmarks
* **Unit Testing:** Unit-test the library and keep it clean under the race detector (`go test -race`). Aim for solid coverage of the pipeline package (excluding the test harness and main glue).
* **Benchmarking:** Benchmark the pipeline across increasing event volumes (`10`, `1k`, `100k`, `1M` events) for two payloads:
  1. A fixed benchmark struct (`TestStruct` with 10 fields of mixed, pinned types defined by you or the starter code).
  2. The sample Mable event committed to the repo as `sample_event.json`.
* **Execution Environment:** Report each benchmark with its method and the machine it ran on (CPU, cores, RAM). Keep a default cap of 1M events so the run stays laptop-safe.
* *[OPTIONAL]* Push to 10M events with the sink streamed to disk (do not retain all events in memory), and/or experiment with Go GC tuning and binary build strategies. Sequence these after the mandatory benchmarks.

---

## 4. Documentation & Leadership Deliverables

### `DESIGN.md` (1-2 pages)
This architectural write-up is heavily weighted. It must cover:
* The architecture of the system.
* The 3-4 decisions that actually mattered, alternatives considered, and why they were rejected.
* Defensive arguments for the trade-offs made (e.g., the auth model against the cross-origin constraint, and the pipeline's typing model).
* Known failure modes and how you would scale the system.
* **Team Management Paragraph:** How would you split this work across a team of three and sequence the first two weeks?

### `REVIEW.md`
A written code review artifact reflecting technical leadership.
* Pick one non-trivial file from your own submission (a Go handler or a React component).
* Review it as if a teammate wrote it: list what you would change and why, severity-ranked, and what you would accept as-is.

---

## Submission Guidelines
* **Repository:** Push to a **public** repository on your GitHub. Any change after the submission deadline must be communicated in advance.
* **Naming Convention:** `<insert_name_here>_mable_technical_lead_task_<date_of_submission>`
* **Folder Structure:**
  * `ecommerce/` - for the e-commerce application
  * `script/` - for the tracking script
  * `api/` - for the Go API
  * `pipeline/` - for the Go pipeline library
  * `links/` - for links to the walkthrough video and Grafana dashboard
* **Submission Recipients:** Share the repository link with:
  * horst@mable.ai
  * sayak@mable.ai
  * malay@mable.ai

---

## Ground Rules
* **AI Tooling:** AI coding tools are allowed and encouraged. Please disclose how you used them in your submission text.
* **Data Privacy & Compliance:** Use synthetic data only—do not collect data from real users. Add a simple consent gate and/or note where consent would be required, and avoid persisting real PII. (In production, tracked parameters like IP, location, checkout details, and sessions require consent and data-retention limits).

---

## Evaluation Criteria & Rubric

| Criteria | Weight | Description |
| :--- | :--- | :--- |
| **Design Judgment & Communication** | **25%** | `DESIGN.md` names the decisions that mattered, alternatives rejected, failure modes, and defends trade-offs (auth, pipeline typing). |
| **Library & Systems Design** | **20%** | Clean stage interface; `Pipeline[T]` / `Reduce[T, R]` typing decision is justified; adding a stage is trivial and demonstrated; concurrency is race-free with bounded memory and back-pressure. Pipeline is fully wired into the API's ingest path with per-stage metadata. |
| **Code Quality & Tests** | **20%** | Clean under `--race`; meaningful tests over coverage percentage; readable Gin handlers; one coherent auth model. |
| **Product / Full-Stack Execution** | **15%** | Core user journey works end-to-end; correct browser-vs-server tracking split; sensible event and data model. |
| **Leadership & Risk Instinct** | **15%** | Sharp, severity-ranked code review artifact; proactive flagging of PII/consent requirements; realistic team-split sequencing. |
| **Creativity / Stretch** | **5%** | Extra credit for completing `[OPTIONAL]` items (extra event types, profiling, self-tuning parameters, or unprompted product/ops insights). |

### Priority Guideline
The must-haves are the e-commerce app, the tracking script, the Go event API with ingest to the analytics DB, and the pipeline library with its tests, its 1M-event benchmarks, and its per-stage metadata ingested to the analytics DB. **Prioritize correctness, clear design decisions, and the library over the breadth of optional items.**
