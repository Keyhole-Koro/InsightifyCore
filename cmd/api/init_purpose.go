package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/llm"
	"insightify/internal/utils"
)

var githubURLPattern = regexp.MustCompile(`https?://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:\.git)?`)

type sourceScoutResult struct {
	RecommendedRepoURL string `json:"recommended_repo_url"`
	Explanation        string `json:"explanation"`
}

type initPurposeResult struct {
	AssistantMessage string `json:"assistant_message"`
	Purpose          string `json:"purpose"`
	RepoURL          string `json:"repo_url"`
	NeedMoreInput    bool   `json:"need_more_input"`
	FollowupQuestion string `json:"followup_question"`
}

func (s *apiServer) launchInitPurposeRun(sessionID, userInput string, isBootstrap bool) (string, error) {
	initRunStore.Lock()
	sess, ok := initRunStore.sessions[sessionID]
	if !ok || sess.RunCtx == nil {
		initRunStore.Unlock()
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	if sess.Running {
		initRunStore.Unlock()
		return "", fmt.Errorf("session %s already has an active run", sessionID)
	}
	sess.Running = true
	runID := fmt.Sprintf("init-purpose-%d", time.Now().UnixNano())
	sess.InitPurposeRunID = runID
	initRunStore.sessions[sessionID] = sess
	initRunStore.Unlock()

	eventCh := make(chan *insightifyv1.WatchRunResponse, 128)
	runStore.Lock()
	runStore.runs[runID] = eventCh
	runStore.Unlock()

	go func() {
		defer func() {
			initRunStore.Lock()
			current := initRunStore.sessions[sessionID]
			current.Running = false
			if current.InitPurposeRunID == runID {
				current.InitPurposeRunID = ""
			}
			initRunStore.sessions[sessionID] = current
			initRunStore.Unlock()
			close(eventCh)
			scheduleRunCleanup(runID)
		}()

		s.executeInitPurposeRun(sess, userInput, isBootstrap, eventCh)
	}()

	return runID, nil
}

func (s *apiServer) executeInitPurposeRun(sess initSession, userInput string, isBootstrap bool, eventCh chan<- *insightifyv1.WatchRunResponse) {
	if isBootstrap && strings.TrimSpace(userInput) == "" {
		msg := "コンピュータの仕組みをひも解いてみますか？ それとも、実際のOSSコードに dive して理解を深めますか？ 気になるテーマや、見たいGitHubリポジトリがあれば貼ってください。"
		view := buildInitPurposeClientView(msg, true)
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_LOG,
			Message:    msg,
			ClientView: view,
		}
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
			Message:    "INPUT_REQUIRED",
			ClientView: view,
		}
		return
	}

	input := strings.TrimSpace(userInput)
	if input == "" {
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   "input is required",
		}
		return
	}

	ctx := llm.WithWorker(context.Background(), "init_purpose")
	extractedRepo := strings.TrimSpace(firstGithubURL(input))

	scout := sourceScoutResult{}
	if extractedRepo == "" {
		// Explicit middle-level worker path when repository is not specified.
		scoutRes, err := runSourceScout(ctx, sess.RunCtx, input)
		if err == nil {
			scout = scoutRes
			if strings.TrimSpace(scout.RecommendedRepoURL) != "" {
				extractedRepo = strings.TrimSpace(scout.RecommendedRepoURL)
			}
		}
	}

	result, err := runInitPurposeLow(ctx, sess.RunCtx, input, extractedRepo, scout.Explanation)
	if err != nil {
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   err.Error(),
		}
		return
	}

	if strings.TrimSpace(result.RepoURL) == "" {
		result.RepoURL = extractedRepo
	}
	if strings.TrimSpace(result.AssistantMessage) == "" {
		result.AssistantMessage = "ありがとう。もう少し詳しく、目的かリポジトリURLを教えてください。"
	}
	if strings.TrimSpace(result.FollowupQuestion) == "" {
		result.FollowupQuestion = "リポジトリURLか、知りたい目的をもう少し具体的に教えてください。"
	}

	needMore := result.NeedMoreInput
	if strings.TrimSpace(result.RepoURL) == "" && strings.TrimSpace(result.Purpose) == "" {
		needMore = true
	}
	if needMore {
		result.AssistantMessage = strings.TrimSpace(result.AssistantMessage + "\n\n" + result.FollowupQuestion)
	}

	// Pseudo-stream assistant text as incremental client-view updates.
	var acc strings.Builder
	for _, piece := range chunkByWord(result.AssistantMessage, 12) {
		acc.WriteString(piece)
		view := buildInitPurposeClientView(acc.String(), needMore)
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_LOG,
			Message:    piece,
			ClientView: view,
		}
		time.Sleep(35 * time.Millisecond)
	}

	if strings.TrimSpace(result.RepoURL) != "" {
		sessionID := sess.SessionID
		initRunStore.Lock()
		cur := initRunStore.sessions[sessionID]
		cur.RepoURL = strings.TrimSpace(result.RepoURL)
		if repo := inferRepoName(cur.RepoURL); repo != "" {
			cur.Repo = repo
		}
		initRunStore.sessions[sessionID] = cur
		initRunStore.Unlock()
	}

	finalView := buildInitPurposeClientView(result.AssistantMessage, needMore)
	finalMessage := "INIT_PURPOSE_COMPLETE"
	if needMore {
		finalMessage = "INPUT_REQUIRED"
	}
	eventCh <- &insightifyv1.WatchRunResponse{
		EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
		Message:    finalMessage,
		ClientView: finalView,
	}
}

func runSourceScout(ctx context.Context, runCtx *RunContext, userInput string) (sourceScoutResult, error) {
	var zero sourceScoutResult
	if runCtx == nil || runCtx.Env == nil || runCtx.Env.LLM == nil {
		return zero, fmt.Errorf("source_scout: llm not configured")
	}
	prompt := `You are source_scout. Return strict JSON:
{
  "recommended_repo_url": "string, optional GitHub URL",
  "explanation": "short reason for recommendation"
}
If the user already provided a repository URL, echo that URL.
If user intent is conceptual and no specific repo is obvious, keep recommended_repo_url empty and suggest learning direction in explanation.`
	input := map[string]any{
		"user_input": userInput,
	}
	llmCtx := llm.WithModelSelection(llm.WithWorker(ctx, "source_scout"), llm.ModelRoleWorker, llm.ModelLevelMiddle, "", "")
	raw, err := runCtx.Env.LLM.GenerateJSON(llmCtx, prompt, input)
	if err != nil {
		return zero, err
	}
	var out sourceScoutResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func runInitPurposeLow(ctx context.Context, runCtx *RunContext, userInput, detectedRepoURL, scoutExplanation string) (initPurposeResult, error) {
	var zero initPurposeResult
	if runCtx == nil || runCtx.Env == nil || runCtx.Env.LLM == nil {
		return zero, fmt.Errorf("init_purpose: llm not configured")
	}
	prompt := `You are init_purpose. Return strict JSON only:
{
  "assistant_message": "friendly Japanese message to user",
  "purpose": "summarized learning goal, optional",
  "repo_url": "GitHub URL if identified, optional",
  "need_more_input": true or false,
  "followup_question": "one short question when more input is needed"
}
Keep message concise and practical.`
	input := map[string]any{
		"user_input":        userInput,
		"detected_repo_url": detectedRepoURL,
		"scout_explanation": scoutExplanation,
	}
	llmCtx := llm.WithModelSelection(llm.WithWorker(ctx, "init_purpose"), llm.ModelRoleWorker, llm.ModelLevelLow, "", "")
	raw, err := runCtx.Env.LLM.GenerateJSON(llmCtx, prompt, input)
	if err != nil {
		return zero, err
	}
	var out initPurposeResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func firstGithubURL(s string) string {
	m := githubURLPattern.FindString(strings.TrimSpace(s))
	return strings.TrimSpace(m)
}

func chunkByWord(s string, wordsPerChunk int) []string {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) == 0 {
		return []string{s}
	}
	if wordsPerChunk <= 0 {
		wordsPerChunk = 8
	}
	out := make([]string, 0, (len(parts)/wordsPerChunk)+1)
	for i := 0; i < len(parts); i += wordsPerChunk {
		j := i + wordsPerChunk
		if j > len(parts) {
			j = len(parts)
		}
		ch := strings.Join(parts[i:j], " ")
		if j < len(parts) {
			ch += " "
		}
		out = append(out, ch)
	}
	return out
}

func buildInitPurposeClientView(text string, awaitingInput bool) *pipelinev1.ClientView {
	state := "Waiting for input"
	if !awaitingInput {
		state = "Ready"
	}
	view := &pipelinev1.ClientView{
		Phase: "init_purpose",
		Graph: &pipelinev1.GraphView{
			Nodes: []*pipelinev1.GraphNode{
				{
					Uid:         "init-purpose-assistant",
					Label:       "Init Purpose Assistant",
					Description: strings.TrimSpace(text),
				},
				{
					Uid:         "init-purpose-state",
					Label:       "State",
					Description: state,
				},
			},
			Edges: []*pipelinev1.GraphEdge{
				{From: "init-purpose-assistant", To: "init-purpose-state"},
			},
		},
	}
	utils.AssignGraphNodeUIDs(view)
	return view
}
