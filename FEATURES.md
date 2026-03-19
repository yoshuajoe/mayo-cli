# Mayo CLI: Feature Roadmap & Status

List of features and the development roadmap for Mayo CLI to achieve production stability and optimal user experience.

## 1. Robustness & Production Readiness

### ✅ Safe SQL Execution (Security)
*   **Status**: 🚧 In Progress / Planned
*   **Plan**: Implement a formal SQL parser (e.g., `github.com/pganalyze/pg_query_go` for Postgres or a native SQLite parser) to ensure only `SELECT` commands can be executed, replacing the current vulnerable regex validation.

### ✅ Secrets Management
*   **Status**: 🚧 Planned
*   **Plan**: Integrate with OS Keychain (`github.com/zalando/go-keyring`) to store API Key credentials encrypted at the OS level, replacing plain-text storage in `.mayo-cli/config.json`.

### ✅ Large Dataset Handling
*   **Status**: 🚧 In Progress
*   **Plan**: Implement "Streaming Data Retrieval" and automatic `LIMIT` enforcement in LLM context. Add chunking mechanisms within the `Orchestrator` to prevent Out-of-Memory (OOM) errors during large query executions.

---

## 2. User Experience (UX) Enhancements

### ✅ AI Data Enhancer
*   **Status**: 💎 Done (v1.3.0)
*   **Feature**: Background AI-powered SQLite data enrichment, incremental batch processing, and an interactive `/enhance start` wizard.

### ✅ Deep Autocomplete & Auto-Suggestions
*   **Status**: 💎 Done (Basic) / 🚧 Improving
*   **Feature**: Fuzzy matching for command suggestions in case of typos. Future plan: Deep autocomplete for table and column names once a database is connected.

### ✅ Interactive Query Review
*   **Status**: 🚧 Planned
*   **Feature**: Introduce a pause before executing AI-generated SQL so users can perform manual tweaks. Add a `--manual` flag or an interactive survey interruption before execution.

### ✅ Multi-Format Export
*   **Status**: 🚧 Planned
*   **Feature**: New commands like `/export csv`, `/export excel`, and `/export json` for seamless integration with external BI tools like Tableau or Excel.

### ✅ Advanced Visualizations
*   **Status**: 🚧 Planned
*   **Feature**: `/plot` command to generate ASCII charts in the terminal or interactive HTML charts that open automatically in the browser.

---

## 3. Advanced Intelligence & Context (AI Features)

### ✅ Analyst Insight
*   **Status**: 💎 Done
*   **Feature**: `/analyst` command to automatically provide insights and analysis on query results.

### ✅ AI Reconcile & Data Reconciliation
*   **Status**: 💎 Done
*   **Feature**: `/reconcile` command for data reconciliation (e.g., matching bank data vs. internal CSVs) powered by AI.

### ✅ Cross-Source Join (The "Bridge" Feature)
*   **Status**: 💎 Done
*   **Feature**: JOIN across different database types (Postgres + CSV) via in-memory SQLite. Also supports loading indexed knowledge documents as queryable datasets via `/df load knowledge:[session_id]` for cross-querying with SQL data.

### ✅ Schema Annotation
*   **Status**: 🚧 Planned
*   **Feature**: Allow users to provide manual annotations for cryptic database columns, saved in `metadata.md` for the AI to learn and reference.

### ✅ Vector Store Integration (RAG) with PII Masking
*   **Status**: 💎 Done
*   **Feature**: Semantic search and vector-based RAG on research documents (PDF/Markdown). Powered by model embeddings and cosine similarity. PII in documents is automatically masked before being sent to external AI APIs.

### ✅ Secured REST API Server (`mayo serve`)
*   **Status**: 💎 Done
*   **Feature**: Capability to run Mayo as a secured HTTP server. Includes Bearer token authentication, `/v1/query` and `/v1/status` endpoints, and CORS support for the integration with other applications.

---

## 4. Maintenance & Quality

### ✅ Automated Testing & CI/CD
*   **Status**: 💎 Done
*   **Feature**: Unit tests for core components and GitHub Actions setup for automated builds and testing on every commit.

### ✅ Build Artifact Strategy
*   **Status**: 💎 Done
*   **Feature**: Automated binary generation via GitHub Actions to simplify distribution, similar to the Homebrew installation experience.

### ✅ Onboarding Wizard
*   **Status**: 💎 Done
*   **Feature**: `/init` or `/wizard` command to guide new users through initial setup (AI Profiles, Database connections, and Privacy settings).

### ✅ Teleskop Scraper Integration
*   **Status**: 💎 Done 
*   **Feature**: Background scraper integration (via Teleskop.id) that feeds directly into Mayo's SQLite for real-time analysis.

### ✅ Caddy & HTTPS Proxy (`/telegram`)
*   **Status**: 💎 Done
*   **Feature**: Automatic Caddyfile generation and setup for secure HTTPS endpoints, allowing Mayo to be safely exposed for Telegram bot integrations.

---

> *This document is updated periodically as a development guide for Mayo CLI.*
