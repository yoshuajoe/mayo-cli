# 🐶 Mayo Command Manual (v1.4.0)

Mayo combines the reasoning of an AI with the capability of a data scientist. This document provides a comprehensive review of all commands available in the Mayo CLI, categorized by their functionality.

---

## 🧠 AI & Configuration
Manage AI behavior, privacy settings, and persistent context.

| Command | Subcommand | Description |
| :--- | :--- | :--- |
| `/mayo` | - | Displays version, license, and current system health status. |
| `/setup` | `/s` | Launch the interactive configuration wizard for AI Providers (Gemini, OpenAI, Groq, Anthropic) and general preferences. |
| `/model` | - | Change the active LLM model for the current session. |
| `/profile` | `ai`, `ds` | Manually switch between saved AI or Data Source profiles. |
| `/context` | `[text/file/clear]` | Set persistent business logic or domain knowledge that Mayo should always remember. |
| `/privacy` | - | Toggle PII Masking (ON/OFF). Masks emails, phones, and credentials before cloud transit. |
| `/debug` | - | Toggle Debug Mode to see raw AI prompts and metadata. |
| `/analyst` | - | Toggle "Analyst Insight" which automatically analyzes query results after they are fetched. |
| `/wizard` | - | Starts the guided onboarding process for new users. |

---

## 🔌 Connectivity & Data Sources
Connect Mayo to your databases and files.

| Command | Subcommand | Description |
| :--- | :--- | :--- |
| `/connect` | `/c` | Connect to a new data source (Postgres, MySQL, SQLite, File). |
| `/sources` | - | List all active connections and their detailed connection strings. |
| `/disconnect` | `[alias]` | Close a connection to a specific data source. |
| `/scan` | - | Re-read schema and metadata from all connected sources. |
| `/describe` | `[alias]` | Generate a Pandas-style statistical summary of a table or dataframe. |
| `/annotate` | `[target] [desc]` | Add AI-readable descriptions to improve context (e.g., `/annotate users.id "Internal UUID"`). |

---

## 📊 Data Processing (Dataframes)
Mayo uses "Dataframe Mode" to stage results in memory for complex analysis.

| Command | Subcommand | Description |
| :--- | :--- | :--- |
| `/reconcile`| `/compare` | AI-powered reconciliation between two connected sources. Mayo identifies mapping patterns and discrepancies. |
| `/df` | `list` | List all saved dataframes in local storage. |
| `/df` | `load [name]` | Load a saved dataframe or knowledge document into current memory. |
| `/df` | `commit [name]`| Persist current result set/memory into local SQLite storage. |
| `/df` | `status` | Check the current state of the working copy. |
| `/df` | `reset` | Clear memory and return to "Pure Database Mode". |
| `/df` | `export [name]`| Export a result set directly to a file (Markdown, CSV, JSON). |
| `/plot` | `[chart type]` | Generate ASCII charts or descriptions of trend data. |
| `/enhance` | `start` | Launch the AI Data Enhancer worker for the active dataframe. |

---

## 📚 Knowledge Base (RAG)
Build a semantic memory from external documents.

| Command | Arguments | Description |
| :--- | :--- | :--- |
| `/knowledge` | `[file]` | Parse and index a PDF, Markdown, or Text file into the session's vector store. |
| `/knowledge` | `against [file]`| Compare a document against existing indexed knowledge (Compliance/Gap analysis). |

---

## 💾 Sessions & Research Management
Maintain a clear history of your research threads.

| Command | Subcommand | Description |
| :--- | :--- | :--- |
| `/sessions` | `create` | Start a completely fresh research session. |
| `/sessions` | `list` | Switch between historical research sessions. |
| `/sessions` | `rename` | Give a meaningful title to the current session. |
| `/sessions` | `delete` | Remove a specific session and its associated data/vault. |
| `/export` | `[file.md]` | Export the raw session log (transcript) to a file. |
| `/share` | `[file.md]` | Synthesize current session results into an AI-generated Executive Report. |
| `/audit` | - | View the last 100 lines of the prompt audit log for transparency. |
| `/history` | `[session_id]` | View the full transcript of a specific session. |
| `/changelog` | - | View the latest feature updates and version history. |

---

## 📡 API & System
Expose Mayo as a service or manage system utilities.

| Command | Subcommand | Description |
| :--- | :--- | :--- |
| `/serve` | `spawn`, `stop`, `logs`, `status` | Manage Mayo's background API server. |
| `/this` | - | "God Mode" status: IDs, directories, and active structures. |
| `/clear` | - | Clear the terminal screen. |
| `/help` | `[query]` | Open the interactive manual or search for specific commands. |
| `/exit` | `/q`, `/quit`| Safely terminate the CLI session and close connections. |
| `/history` | `[session_id]` | View chat logs for the current or specified session. |

---

## 📡 Terminal Commands (External)
These commands are run directly from your terminal shell, not inside Mayo's interactive mode.

| Command | Usage | Description |
| :--- | :--- | :--- |
| `mayo serve` | `mayo serve --port 8080` | Start a foreground REST API server with multi-session support. |
| `mayo release-notes` | `mayo release-notes` | Generate an AI changelog from git commits. |
| `mayo scraper` | `mayo scraper spawn [q]` | Start a background Teleskop.id data scraper. |
| `mayo wizard` | `mayo wizard` | Run the onboarding wizard (alias for `/setup`). |

---
*Generated for review on 2026-03-19*
*Status: READY*
