package ai

import (
	"context"
)

func (o *Orchestrator) RunAudit(ctx context.Context) (string, error) {
	auditPrompt := "Perform an autonomous audit of the connected data sources. " +
		"Identify potential data quality issues, outliers, anomalies, and summarize the key characteristics of the data. " +
		"Present your findings in a structured Markdown report."

	return o.ProcessQuery(ctx, auditPrompt)
}
