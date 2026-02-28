package ai

import (
	"bytes"
	"text/template"
)

// ExtractionPromptData holds data for the extraction prompt template.
type ExtractionPromptData struct {
	UserName    string
	UserID      string
	FromDate    string
	ToDate      string
	ThreadsJSON string
}

// IntegrationPromptData holds data for the integration prompt template.
type IntegrationPromptData struct {
	UserName     string
	FromDate     string
	ToDate       string
	TotalThreads int
	TopicsJSON   string
}

var extractionTmpl = template.Must(template.New("extraction").Parse(`You are a Slack message analysis assistant.
Below are Slack threads posted while {{.UserName}} (Slack ID: {{.UserID}}) was on leave ({{.FromDate}} to {{.ToDate}}).

Analyze each thread and return results in the following JSON format.
Respond with JSON only. Do not wrap in markdown code blocks.

{
  "topics": [
    {
      "title": "A concise topic title (one line)",
      "summary": "Summary in 2-3 sentences. What was discussed and what was the conclusion.",
      "channel": "Channel name",
      "permalink": "Thread permalink",
      "participants": ["Relevant people"],
      "action_required": true/false,
      "action_description": "Action {{.UserName}} needs to take (if action_required is true)",
      "priority": "high/medium/low",
      "match_reasons": ["Reason this thread matched the search"]
    }
  ]
}

Priority criteria:
- high: Direct requests/questions to {{.UserName}}, blockers, incident response
- medium: Important progress or decisions on projects {{.UserName}} is involved in
- low: FYI, informational content

Thread data:

{{.ThreadsJSON}}`))

var integrationTmpl = template.Must(template.New("integration").Parse(`You are an assistant that organizes Slack message analysis results.
Below is a list of topics extracted from Slack messages while {{.UserName}} was on leave ({{.FromDate}} to {{.ToDate}}).

Organize them in the following Markdown format.
Group topics that share the same theme.

---

# Slack Catch-up Report
**User**: {{.UserName}}
**Period**: {{.FromDate}} to {{.ToDate}}
**Threads collected**: {{.TotalThreads}}

## Executive Summary
Write a 3-5 sentence overview. Start with the most important items.

## Action Required (High Priority)
### 1. [Topic title]
- **Channel**: #channel-name
- **Summary**: summary
- **Action**: specific action needed
- **People**: @name1, @name2
- **Thread**: permalink

(List all high priority topics. If none, write "None.")

## Worth Reviewing (Medium Priority)
### N. [Topic title]
- **Channel**: #channel-name
- **Summary**: summary
- **Action**: action if any
- **People**: @name1, @name2
- **Thread**: permalink

(List all medium priority topics. If none, write "None.")

## FYI (Low Priority)
### N. [Topic title]
- **Channel**: #channel-name
- **Summary**: summary
- **Thread**: permalink

(List all low priority topics. If none, write "None.")

## Next Actions
List action items in priority order:
- [ ] [HIGH] [Action] — #channel-name (people: @name)
- [ ] [MED] [Action] — #channel-name (people: @name)

---

Topic data:

{{.TopicsJSON}}`))

// RenderExtractionPrompt renders the extraction prompt with the given data.
func RenderExtractionPrompt(data ExtractionPromptData) (string, error) {
	return renderPrompt(extractionTmpl, data)
}

// RenderIntegrationPrompt renders the integration prompt with the given data.
func RenderIntegrationPrompt(data IntegrationPromptData) (string, error) {
	return renderPrompt(integrationTmpl, data)
}

func renderPrompt(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
