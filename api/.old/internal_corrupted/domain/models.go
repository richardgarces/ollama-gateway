package domain

type Request struct {
	Prompt string `json:"prompt"`
}

type Response struct {
	Result string `json:"result"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

type AgentRequest struct {
	Input string `json:"input"`
}

type AgentResponse struct {
	Result string `json:"result"`
}

type RefactorRequest struct {
	Path   string `json:"path"`
	Prompt string `json:"prompt"`
}

type RefactorResponse struct {
	Original  string `json:"original"`
	Refactored string `json:"refactored"`
}

type AnalyzeResponse struct {
	Files []FileAnalysis `json:"files"`
}

type FileAnalysis struct {
	Path     string `json:"path"`
	Purpose  string `json:"purpose"`
	Language string `json:"language"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

	Prompt string `json:"prompt"`
}

type Response struct {
	Result string `json:"result"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`


























}	Token string `json:"token"`type LoginResponse struct {}	Password string `json:"password"`	Username string `json:"username"`type LoginRequest struct {}	Analysis string `json:"analysis"`type AnalysisResponse struct {}	Refactor string `json:"refactor"`type RefactorResponse struct {}	Path string `json:"path"`type RefactorRequest struct {}	Message ChatMessage `json:"message"`type ChatChoice struct {}