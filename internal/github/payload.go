package github

import "time"

type repository struct {
	FullName string `json:"full_name"`
}

type sender struct {
	Login string `json:"login"`
}

// workflowRunPayload é o subconjunto do payload do evento workflow_run que
// o collector consome. Ver:
// https://docs.github.com/en/webhooks/webhook-events-and-payloads#workflow_run
type workflowRunPayload struct {
	Action      string `json:"action"`
	WorkflowRun struct {
		ID           int64     `json:"id"`
		RunAttempt   int       `json:"run_attempt"`
		Name         string    `json:"name"`
		Event        string    `json:"event"`
		Status       string    `json:"status"`
		Conclusion   string    `json:"conclusion"`
		HeadBranch   string    `json:"head_branch"`
		RunStartedAt time.Time `json:"run_started_at"`
		UpdatedAt    time.Time `json:"updated_at"`
	} `json:"workflow_run"`
	Repository repository `json:"repository"`
	Sender     sender     `json:"sender"`
}

// workflowJobPayload é o subconjunto do payload do evento workflow_job que
// o collector consome. Ver:
// https://docs.github.com/en/webhooks/webhook-events-and-payloads#workflow_job
type workflowJobPayload struct {
	Action      string `json:"action"`
	WorkflowJob struct {
		ID           int64      `json:"id"`
		RunID        int64      `json:"run_id"`
		WorkflowName string     `json:"workflow_name"`
		Name         string     `json:"name"`
		Status       string     `json:"status"`
		Conclusion   string     `json:"conclusion"`
		CreatedAt    time.Time  `json:"created_at"`
		StartedAt    time.Time  `json:"started_at"`
		CompletedAt  *time.Time `json:"completed_at"`
	} `json:"workflow_job"`
	Repository repository `json:"repository"`
	Sender     sender     `json:"sender"`
}
