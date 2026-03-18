# 🐩 Mayo CLI: Autonomous AI Research & Data Analysis Partner

[![Go Report Card](https://goreportcard.com/badge/github.com/yoshuajoe/mayo-cli)](https://goreportcard.com/report/github.com/yoshuajoe/mayo-cli)
[![Build Status](https://github.com/yoshuajoe/mayo-cli/actions/workflows/test.yml/badge.svg)](https://github.com/yoshuajoe/mayo-cli/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Mayo CLI** is an autonomous AI companion for your terminal, designed to transform how you interact with data. Whether you're a data scientist, researcher, or developer, Mayo helps you query, analyze, and enrich your datasets using natural language—all while keeping your data private and secure.

```text
       ((   ))
      (  o o  )
       )  v  (
   ___/     \___
  (     MAYO    )
    \___________/
      ||     ||
      m       m
```
> "Mayo the Poodle is ready to help!"

---

## ✨ Key Features

- 🧠 **Autonomous Querying**: Ask questions in plain English, and Mayo generates and executes the SQL for you.
- 🔌 **Universal Connectivity**: Seamlessly connect to **PostgreSQL**, **MySQL**, **SQLite**, **CSV files**, and even entire **folders**.
- 🔐 **Privacy First**: Built-in PII (Personally Identifiable Information) masking and secure credential storage using your OS Keychain.
- 🚀 **Data Enrichment**: Use the `/enhance` command to batch-process datasets using AI for classification, sentiment analysis, or data cleaning.
- 📂 **Research Sessions**: Persistent work sessions that save your history, queries, and summaries. Never lose your progress.
- 🕵️ **Smart Scraper**: Integrated background scraping (via Teleskop.id) to feed fresh data into your analysis.
- 📊 **Interactive UX**: Guided onboarding (`/wizard`), autocompletion, and stunning markdown reports generated directly in your terminal.

---

## 🚀 Getting Started

### Prerequisites

- **Go** (1.21 or later)
- **SQLite3**
- An AI Provider API Key (OpenAI, Anthropic, or Google Gemini)

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/yoshuajoe/mayo-cli.git
   cd mayo-cli
   ```

2. **Setup dependencies:**
   ```bash
   make setup
   ```

3. **Install globally (Optional):**
   ```bash
   sudo make install
   ```

4. **Initialize Mayo:**
   ```bash
   mayo /wizard
   ```
   *The wizard will guide you through setting up your AI profile and connecting your first data source.*

---

## 🛠 Usage & Commands

Mayo operates in an interactive shell. Once started, you can type natural language queries or use "Slash Commands" for administrative tasks.

### Common Slash Commands

| Command | Description |
| :--- | :--- |
| `/wizard` | Guided onboarding for new users. |
| `/connect` | Connect to a database or file (e.g., `/connect postgres [dsn]`). |
| `/df` | Manage dataframes (load, save, list, status). |
| `/enhance` | Start high-performance AI data enhancement tasks. |
| `/sessions` | Manage research sessions (list, create, switch). |
| `/share` | Generate a beautiful markdown research report. |
| `/privacy` | Toggle Privacy Mode (PII masking). |
| `/scraper` | Control background data scraping workers. |
| `/help` | List all available commands. |

### Example Workflow

1. **Connect to data**: `/connect file data/users.csv`
2. **Explore**: "Show me the top 10 users by activity last month."
3. **Analyze**: "What is the average growth rate of our subscriber base?"
4. **Enrich**: `/enhance start --prompt "Classify the sentiment of these feedback comments"`
5. **Export**: `/share summary_report.md`

---

## 📐 Architecture

Mayo is built for speed and extensibility:
- **Core**: Written in Go for high performance and easy distribution.
- **Engine**: Uses a custom **Orchestrator** to handle LLM communication and SQL execution safely.
- **UI**: Powered by `lipgloss` and `charmbracelet` for a premium terminal experience.
- **Storage**: Local metadata and session state are managed via SQLite and JSON.

---

## 🗺 Roadmap

See the [ROADMAP.md](ROADMAP.md) for planned features, including:
- [ ] Safe SQL Parser integration.
- [ ] Multi-format exports (Excel, JSON).
- [ ] Cross-source JOINS (e.g., Postgres + CSV).
- [ ] Advanced ASCII visualizations.

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.

---

*Made with ❤️ by [Yoshua Putro](https://github.com/yoshuajoe)*
