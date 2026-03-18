package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
	"mayo-cli/internal/privacy"
)

var (
	// Colors
	ColorPrimary   = lipgloss.Color("#FF7F50") // Coral/Orange for the monster
	ColorSecondary = lipgloss.Color("#00CED1") // DarkTurquoise for accents
	ColorMuted     = lipgloss.Color("#626262") // Gray for separators
	ColorWhite     = lipgloss.Color("#FFFFFF")
	ColorGreen     = lipgloss.Color("#50FA7B")
	ColorHighlight = lipgloss.Color("#F1FA8C")

	// Styles
	StyleTitle       = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	StyleBanner      = lipgloss.NewStyle().Foreground(ColorWhite)
	StyleMonster     = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	StyleSeparator   = lipgloss.NewStyle().Foreground(ColorMuted)
	StylePrompt      = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	StyleStatus      = lipgloss.NewStyle().Foreground(ColorHighlight).Italic(true)
	StyleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
	StyleSuccess     = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	StyleDocTitle    = lipgloss.NewStyle().Foreground(ColorPrimary).Underline(true)
	StyleCodeLine    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD"))
	StyleDiffAdded   = lipgloss.NewStyle().Foreground(ColorWhite).Background(lipgloss.Color("#1B391B"))
	StyleDiffRemoved = lipgloss.NewStyle().Foreground(ColorWhite).Background(lipgloss.Color("#391B1B"))
	StyleLineNumber  = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleHighlight   = lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)
	StyleThought     = lipgloss.NewStyle().Foreground(ColorMuted).Italic(true)
	StyleMuted       = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleResultBox   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSecondary).
				Padding(1, 2).
				MarginTop(1)
	StyleSQLTable = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorMuted).
			MarginLeft(2)

	// Debug
	DebugEnabled  = false
	StyleDebug    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Italic(true)
	StyleDebugLLM = lipgloss.NewStyle().Foreground(lipgloss.Color("#BD93F9")).Italic(true) // Dracula Purple
)

func PrintBanner(version string) {
	// fmt.Print("\033[H\033[2J") // Clear screen

	title := StyleTitle.Render(fmt.Sprintf("Welcome to Mayo %s", version))
	sep := StyleSeparator.Render(strings.Repeat("┈", 60))

	poodle := `
       ((   ))
      (  o o  )
       )  v  (
   ___/     \___
  (     MAYO    )
   \___________/
     ||     ||
     m       m`

	moon := `
           __
          /  \
         |    |
          \__/`

	clouds := `
     ☁️  ☁️         ☁️
            ☁️`

	stars := `
    *        .       *
       .        *
    *      .        .`

	fmt.Println(title)
	fmt.Println(sep)

	banner := lipgloss.JoinVertical(lipgloss.Left,
		StyleBanner.Render(stars),
		lipgloss.JoinHorizontal(lipgloss.Center,
			StyleBanner.Render(clouds),
			lipgloss.NewStyle().Foreground(ColorHighlight).Render(moon),
		),
		StyleMonster.Render(poodle),
	)

	fmt.Println(banner)
	fmt.Println(StyleStatus.Render("   Mayo the Poodle is ready to help!"))
	fmt.Println(sep)
	fmt.Println()
}

func RenderSeparator() {
	fmt.Println(StyleSeparator.Render(strings.Repeat("┈", 60)))
}

func FormatPrompt(connName string, dfName string, sessionSummary string, isConnected bool, hasSession bool) string {
	status := "🔴"
	if hasSession {
		if isConnected {
			status = "🟢"
		} else {
			status = "⚪"
		}
	}

	// mayo(alias) icon [df_name] [summary] >
	p := "\033[36m" + "mayo" + "\033[0m" // Cyan
	if connName != "" {
		p += "\033[36m" + fmt.Sprintf("(%s)", connName) + "\033[0m"
	}

	p += " " + status

	if dfName != "" {
		p += " \033[33m[" + dfName + "]\033[0m" // Yellow for dataframe
	}

	if sessionSummary != "" {
		// Clean the summary from newlines and extra spaces
		cleanSummary := strings.ReplaceAll(sessionSummary, "\n", " ")
		cleanSummary = strings.ReplaceAll(cleanSummary, "\r", "")
		if len(cleanSummary) > 25 {
			cleanSummary = cleanSummary[:22] + "..."
		}
		p += " \033[90m[" + cleanSummary + "]\033[0m" // Grey
	}

	return p + " > "
}

func PrintError(msg string) {
	fmt.Printf("%s %s\n", StyleError.Render("❌ Error:"), msg)
}

func PrintSuccess(msg string) {
	fmt.Printf("%s %s\n", StyleSuccess.Render("✅ Success:"), msg)
}

func PrintInfo(msg string) {
	fmt.Printf("%s %s\n", StyleTitle.Render("ℹ️ Info:"), msg)
}

func RenderDiff(lines []string) {
	for i, line := range lines {
		lineNum := StyleLineNumber.Render(fmt.Sprintf("%2d ", i+1))
		if strings.HasPrefix(line, "+ ") {
			fmt.Println(lineNum + StyleDiffAdded.Render(line))
		} else if strings.HasPrefix(line, "- ") {
			fmt.Println(lineNum + StyleDiffRemoved.Render(line))
		} else {
			fmt.Println(lineNum + line)
		}
	}
}

func RenderThinking() {
	fmt.Println(StyleStatus.Render("🤖 Thinking..."))
}

func RenderSQLStatus(msg string) {
	fmt.Printf("\n%s %s\n", StyleMonster.Render("💾"), StyleHighlight.Render(msg))
}

func RenderTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		PrintInfo("No rows returned.")
		return
	}

	// Get terminal width
	termWidth := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		termWidth = w
	}

	// 1. Calculate ideal widths for each column (limit to 40 chars for table neatness)
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			cellLen := len(privacy.RestorePrivacy(cell))
			if cellLen > 40 {
				cellLen = 40
			}
			if cellLen > widths[i] {
				widths[i] = cellLen
			}
		}
	}

	// 2. Check if it fits in terminal
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w + 3
	}

	// Switch to vertical ONLY if columns are many (> 8) or extremely wide
	if (totalWidth > termWidth && len(headers) > 4) || len(headers) > 8 {
		renderVerticalView(headers, rows)
		return
	}

	// 3. Render as Horizontal Table
	headerStyle := lipgloss.NewStyle().
		Foreground(ColorHighlight).
		Background(lipgloss.Color("#34495e")).
		Bold(true).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().Padding(0, 1)

	var sb strings.Builder
	// Render Headers
	for i, h := range headers {
		sb.WriteString(headerStyle.Width(widths[i] + 2).Render(h))
	}
	sb.WriteString("\n")

	// Render Rows
	for ri, row := range rows {
		rowStyle := lipgloss.NewStyle()
		if ri%2 != 0 {
			rowStyle = rowStyle.Background(lipgloss.Color("#2c3e50"))
		}
		for i, cell := range row {
			text := privacy.RestorePrivacy(cell)
			if len(text) > widths[i] {
				text = text[:widths[i]-3] + "..."
			}
			sb.WriteString(rowStyle.Inherit(cellStyle).Width(widths[i] + 2).Render(text))
		}
		sb.WriteString("\n")
	}

	fmt.Println(StyleSQLTable.Render(sb.String()))
}

func renderVerticalView(headers []string, rows [][]string) {
	labelStyle := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(ColorWhite)
	recordHeaderStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Underline(true)

	// Calculate max label width for alignment
	maxLabelWidth := 0
	for _, h := range headers {
		if len(h) > maxLabelWidth {
			maxLabelWidth = len(h)
		}
	}

	for i, row := range rows {
		fmt.Printf("\n%s\n", recordHeaderStyle.Render(fmt.Sprintf("--- RECORD %d ---", i+1)))
		for ci, cell := range row {
			label := fmt.Sprintf("%*s", maxLabelWidth, headers[ci])
			text := privacy.RestorePrivacy(cell)
			fmt.Printf("%s : %s\n", labelStyle.Render(label), valStyle.Render(text))
		}
	}
	fmt.Println()
}

func RenderMarkdown(text string) {
	// Get terminal width for responsiveness
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width = w - 10 // Leave space for box borders and padding
	}

	// Constraints for readability
	if width < 40 {
		width = 40
	}
	if width > 120 {
		width = 120
	}

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	out, err := r.Render(text)
	if err != nil {
		fmt.Println(text)
		return
	}
	// Wrap in a nice box that adapts to width
	boxed := StyleResultBox.Width(width + 4).Render(strings.TrimSpace(out))
	fmt.Println(boxed)
}

func RenderThought(text string) {
	fmt.Println(StyleThought.Render("\n💭 Thinking Process:"))
	fmt.Println(StyleThought.Render(text))
	fmt.Println()
}

func RenderStep(icon string, msg string) {
	fmt.Printf("%s %s\n", icon, StyleMuted.Render(msg))
}

func RenderSQLQuery(query string) {
	fmt.Println(StyleMuted.Render(" └── ") + StyleCodeLine.Render(query))
}

func RenderUsage(prompt, completion, total int) {
	usageStr := fmt.Sprintf("Tokens: %d (in) | %d (out) | %d (total)", prompt, completion, total)
	fmt.Println(StyleMuted.Render(" └── " + usageStr))
}

func RenderDebug(title, content string) {
	fmt.Printf("\n%s\n", StyleDebug.Bold(true).Render("══ DEBUG: "+title+" ══"))
	fmt.Println(StyleDebug.Render(content))
	fmt.Printf("%s\n\n", StyleDebug.Bold(true).Render("══ END DEBUG ══"))
}

func RenderDebugLLM(title, content string) {
	fmt.Printf("\n%s\n", StyleDebugLLM.Bold(true).Render("══ DEBUG: "+title+" ══"))
	fmt.Println(StyleDebugLLM.Render(content))
	fmt.Printf("%s\n\n", StyleDebugLLM.Bold(true).Render("══ END DEBUG ══"))
}

func ClearScreen() {
	fmt.Print("\033[H\033[2J")
}
