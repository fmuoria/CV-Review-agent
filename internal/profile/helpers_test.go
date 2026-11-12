package profile

import "github.com/fmuoria/CV-Review-agent/internal/llm"

func llmReq(cv string) llm.Request { return llm.Request{Hint: cv} }
