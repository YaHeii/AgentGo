package app

type SendMessageParams struct {
	SessionID string
	Prompt    string
}

type SendMessageResult struct {
	User      Message
	Assistant Message
}
