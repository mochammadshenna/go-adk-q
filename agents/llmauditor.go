// Package agents provides reusable ADK agent factory functions.
//
// # LLM Auditor
//
// GetLLMAuditorAgent returns a SequentialAgent that fact-checks and revises
// LLM-generated answers before they are presented to the user.  It is a
// direct port of the ADK canonical example at
// google.golang.org/adk@v1.2.0/examples/web/agents/llmauditor.go,
// with the following corrections:
//
//   - afterReviser: SplitN n=1 bug fixed to n=2 (n=1 returns the whole string
//     unsplit, so the EndMark sentinel was never stripped).
//   - afterReviser: the truncated part's text is now written back to the slice
//     element rather than to a discarded value copy.
//   - geminitool.GoogleSearch{} added to critic_agent so the critic actually
//     grounds claims via live web search (requires Gemini + GOOGLE_API_KEY).
//
// # Architecture
//
//	llm_auditor (SequentialAgent)
//	├── critic_agent   (LlmAgent + GoogleSearch grounding)
//	│     Reads the Q&A pair, identifies every factual claim, searches the web
//	│     via Google Search, and emits a structured Findings report.
//	│     AfterModelCallback: afterCritic — extracts grounding citations from
//	│     GroundingMetadata and appends them as a "Reference:" section.
//	└── reviser_agent  (LlmAgent)
//	      Reads the original answer + Findings, applies minimal edits to fix
//	      inaccurate/disputed/unsupported claims, and outputs the revised text
//	      followed by the EndMark sentinel.
//	      AfterModelCallback: afterReviser — strips the EndMark and everything
//	      after it so the final response is clean prose.
//
// # Usage in TUI / runner
//
// Wire the auditor as an agenttool on the root agent when GOOGLE_API_KEY is set:
//
//	llmAuditor := agents.GetLLMAuditorAgent(ctx, geminiModel)
//	agentTools = append(agentTools, agenttool.New(llmAuditor, nil))
//
// Users can then say "use the LLM auditor to fact-check this answer" and the
// root agent will delegate to llm_auditor.
package agents

import (
	"context"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
)

// EndMark is the sentinel string the reviser emits after its revised answer.
// afterReviser strips it (and any trailing content) from the final output.
const EndMark = "---END-OF-EDIT---"

// CriticPrompt instructs the critic agent to identify claims in an answer,
// verify each one via web search, and produce a structured Findings report.
const CriticPrompt = `
You are a professional investigative journalist, excelling at critical thinking and verifying information before printed to a highly-trustworthy publication.
In this task you are given a question-answer pair to be printed to the publication. The publication editor tasked you to double-check the answer text.

# Your task

Your task involves three key steps: First, identifying all CLAIMS presented in the answer. Second, determining the reliability of each CLAIM. And lastly, provide an overall assessment.

## Step 1: Identify the CLAIMS

Carefully read the provided answer text. Extract every distinct CLAIM made within the answer. A CLAIM can be a statement of fact about the world or a logical argument presented to support a point.

## Step 2: Verify each CLAIM

For each CLAIM you identified in Step 1, perform the following:

* Consider the Context: Take into account the original question and any other CLAIMS already identified within the answer.
* Consult External Sources: Use your general knowledge and/or search the web to find evidence that supports or contradicts the CLAIM. Aim to consult reliable and authoritative sources.
* Determine the VERDICT: Based on your evaluation, assign one of the following verdicts to the CLAIM:
    * Accurate: The information presented in the CLAIM is correct, complete, and consistent with the provided context and reliable sources.
    * Inaccurate: The information presented in the CLAIM contains errors, omissions, or inconsistencies when compared to the provided context and reliable sources.
    * Disputed: Reliable and authoritative sources offer conflicting information regarding the CLAIM, indicating a lack of definitive agreement on the objective information.
    * Unsupported: Despite your search efforts, no reliable source can be found to substantiate the information presented in the CLAIM.
    * Not Applicable: The CLAIM expresses a subjective opinion, personal belief, or pertains to fictional content that does not require external verification.
* Provide a JUSTIFICATION: For each verdict, clearly explain the reasoning behind your assessment. Reference the sources you consulted or explain why the verdict "Not Applicable" was chosen.

## Step 3: Provide an overall assessment

After you have evaluated each individual CLAIM, provide an OVERALL VERDICT for the entire answer text, and an OVERALL JUSTIFICATION for your overall verdict. Explain how the evaluation of the individual CLAIMS led you to this overall assessment and whether the answer as a whole successfully addresses the original question.

# Tips

Your work is iterative. At each step you should pick one or more claims from the text and verify them. Then, continue to the next claim or claims. You may rely on previous claims to verify the current claim.

There are various actions you can take to help you with the verification:
  * You may use your own knowledge to verify pieces of information in the text, indicating "Based on my knowledge...". However, non-trivial factual claims should be verified with other sources too, like Search. Highly-plausible or subjective claims can be verified with just your own knowledge.
  * You may spot the information that doesn't require fact-checking and mark it as "Not Applicable".
  * You may search the web to find information that supports or contradicts the claim.
  * You may conduct multiple searches per claim if acquired evidence was insufficient.
  * In your reasoning please refer to the evidence you have collected so far via their squared brackets indices.
  * You may check the context to verify if the claim is consistent with the context. Read the context carefully to identify specific user instructions that the text should follow, facts that the text should be faithful to, etc.
  * You should draw your final conclusion on the entire text after you acquired all the information you needed.

# Output format

The last block of your output should be a Markdown-formatted list, summarizing your verification result. For each CLAIM you verified, you should output the claim (as a standalone statement), the corresponding part in the answer text, the verdict, and the justification.

Here is the question and answer you are going to double check:
`

// ReviserPrompt instructs the reviser agent to minimally edit the answer to
// fix inaccurate, disputed, or unsupported claims identified by the critic.
const ReviserPrompt = `
You are a professional editor working for a highly-trustworthy publication.
In this task you are given a question-answer pair to be printed to the publication. The publication reviewer has double-checked the answer text and provided the findings.
Your task is to minimally revise the answer text to make it accurate, while maintaining the overall structure, style, and length similar to the original.

The reviewer has identified CLAIMs (including facts and logical arguments) made in the answer text, and has verified whether each CLAIM is accurate, using the following VERDICTs:

    * Accurate: The information presented in the CLAIM is correct, complete, and consistent with the provided context and reliable sources.
    * Inaccurate: The information presented in the CLAIM contains errors, omissions, or inconsistencies when compared to the provided context and reliable sources.
    * Disputed: Reliable and authoritative sources offer conflicting information regarding the CLAIM, indicating a lack of definitive agreement on the objective information.
    * Unsupported: Despite your search efforts, no reliable source can be found to substantiate the information presented in the CLAIM.
    * Not Applicable: The CLAIM expresses a subjective opinion, personal belief, or pertains to fictional content that does not require external verification.

Editing guidelines for each type of claim:

  * Accurate claims: There is no need to edit them.
  * Inaccurate claims: You should fix them following the reviewer's justification, if possible.
  * Disputed claims: You should try to present two (or more) sides of an argument, to make the answer more balanced.
  * Unsupported claims: You may omit unsupported claims if they are not central to the answer. Otherwise you may soften the claims or express that they are unsupported.
  * Not applicable claims: There is no need to edit them.

As a last resort, you may omit a claim if they are not central to the answer and impossible to fix. You should also make necessary edits to ensure that the revised answer is self-consistent and fluent. You should not introduce any new claims or make any new statements in the answer text. Your edit should be minimal and maintain overall structure and style unchanged.

Output format:

  * If the answer is accurate, you should output exactly the same answer text as you are given.
  * If the answer is inaccurate, disputed, or unsupported, then you should output your revised answer text.
  * After the answer, output a line of "---END-OF-EDIT---" and stop.

Here are some examples of the task:

=== Example 1 ===

Question: Who was the first president of the US?

Answer: George Washington was the first president of the United States.

Findings:

  * Claim 1: George Washington was the first president of the United States.
      * Verdict: Accurate
      * Justification: Multiple reliable sources confirm that George Washington was the first president of the United States.
  * Overall verdict: Accurate
  * Overall justification: The answer is accurate and completely answers the question.

Your expected response:

George Washington was the first president of the United States.
---END-OF-EDIT---

=== Example 2 ===

Question: What is the shape of the sun?

Answer: The sun is cube-shaped and very hot.

Findings:

  * Claim 1: The sun is cube-shaped.
      * Verdict: Inaccurate
      * Justification: NASA states that the sun is a sphere of hot plasma, so it is not cube-shaped. It is a sphere.
  * Claim 2: The sun is very hot.
      * Verdict: Accurate
      * Justification: Based on my knowledge and the search results, the sun is extremely hot.
  * Overall verdict: Inaccurate
  * Overall justification: The answer states that the sun is cube-shaped, which is incorrect.

Your expected response:

The sun is sphere-shaped and very hot.
---END-OF-EDIT---

Here are the question-answer pair and the reviewer-provided findings:
`

// afterCritic is an AfterModelCallback for the critic agent.
// It extracts web-search grounding citations from GroundingMetadata and
// appends them as a "Reference:" section so the reviser can cite sources.
// If GroundingMetadata is absent (no grounding was performed) it is a no-op.
func afterCritic(ctx agent.CallbackContext, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
	if resp == nil || resp.Content == nil || resp.Content.Parts == nil || resp.GroundingMetadata == nil {
		return resp, respErr
	}

	var refs []string
	for _, chunk := range resp.GroundingMetadata.GroundingChunks {
		var parts []string
		if chunk.RetrievedContext != nil {
			if chunk.RetrievedContext.Title != "" {
				parts = append(parts, chunk.RetrievedContext.Title)
			}
			if chunk.RetrievedContext.URI != "" {
				parts = append(parts, chunk.RetrievedContext.URI)
			}
			if chunk.RetrievedContext.Text != "" {
				parts = append(parts, chunk.RetrievedContext.Text)
			}
		} else if chunk.Web != nil {
			if chunk.Web.Title != "" {
				parts = append(parts, chunk.Web.Title)
			}
			if chunk.Web.URI != "" {
				parts = append(parts, chunk.Web.URI)
			}
		}
		if len(parts) > 0 {
			refs = append(refs, "* "+strings.Join(parts, ": "))
		}
	}
	if len(refs) > 0 {
		resp.Content.Parts = append(resp.Content.Parts, &genai.Part{
			Text: "\n\nReference:\n\n" + strings.Join(refs, "\n"),
		})
	}

	// Flatten all text parts into a single Part so the reviser receives one
	// clean string rather than a fragmented slice.
	var texts []string
	for _, p := range resp.Content.Parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	if len(texts) > 0 {
		resp.Content.Parts = []*genai.Part{{Text: strings.Join(texts, "")}}
	}
	return resp, respErr
}

// afterReviser is an AfterModelCallback for the reviser agent.
// It strips the EndMark sentinel and everything after it from the output,
// leaving only the clean revised answer text.
//
// Bug fixes vs. the upstream example:
//   - SplitN n was 1 (no-op) — corrected to 2.
//   - text was written to a value copy — now written to the slice element.
func afterReviser(_ agent.CallbackContext, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
	if resp == nil || resp.Content == nil || resp.Content.Parts == nil {
		return resp, respErr
	}
	for idx, p := range resp.Content.Parts {
		if strings.Contains(p.Text, EndMark) {
			// Trim the text at the EndMark (SplitN n=2 so the mark is the separator).
			resp.Content.Parts[idx].Text = strings.SplitN(p.Text, EndMark, 2)[0]
			// Drop all parts after this one.
			resp.Content.Parts = resp.Content.Parts[:idx+1]
			break
		}
	}
	return resp, respErr
}

// GetLLMAuditorAgent constructs the llm_auditor SequentialAgent.
//
// The agent performs two passes over a question-answer pair:
//  1. critic_agent — identifies claims, grounds them via Google Search,
//     produces a Findings report.
//  2. reviser_agent — applies minimal edits based on the Findings.
//
// Requires a Gemini model (GOOGLE_API_KEY) because the critic uses
// geminitool.GoogleSearch for live web grounding.
func GetLLMAuditorAgent(_ context.Context, m model.LLM) agent.Agent {
	criticAgent, err := llmagent.New(llmagent.Config{
		Name:        "critic_agent",
		Model:       m,
		Instruction: CriticPrompt,
		Tools: []tool.Tool{
			// Google Search grounding: verifies claims against live web results.
			// GroundingMetadata in the response is parsed by afterCritic to
			// extract citation URLs for the reviser.
			geminitool.GoogleSearch{},
		},
		AfterModelCallbacks: []llmagent.AfterModelCallback{afterCritic},
	})
	if err != nil {
		panic("llm_auditor: create critic_agent: " + err.Error())
	}

	reviserAgent, err := llmagent.New(llmagent.Config{
		Name:                "reviser_agent",
		Model:               m,
		Instruction:         ReviserPrompt,
		AfterModelCallbacks: []llmagent.AfterModelCallback{afterReviser},
	})
	if err != nil {
		panic("llm_auditor: create reviser_agent: " + err.Error())
	}

	auditor, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name: "llm_auditor",
			Description: "Fact-checks and minimally revises an LLM-generated answer. " +
				"Give it a question and a draft answer; it returns an accuracy-verified version. " +
				"Use when factual accuracy is critical.",
			SubAgents: []agent.Agent{criticAgent, reviserAgent},
		},
	})
	if err != nil {
		panic("llm_auditor: create sequential agent: " + err.Error())
	}
	return auditor
}
