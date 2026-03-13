package ui

import (
	"fmt"
	"github.com/guptarohit/asciigraph"
)

func RenderChart(data []float64, caption string) string {
	if len(data) == 0 {
		return "No data to render chart."
	}
	graph := asciigraph.Plot(data, asciigraph.Height(10), asciigraph.Width(60))
	return fmt.Sprintf("\n📊 Analysis: %s\n%s\n", caption, graph)
}
