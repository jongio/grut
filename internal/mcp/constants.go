package mcp

// Security field names used in sensitive audit field detection.
const (
	fieldContent  = "content"
	fieldKey      = "key"
	fieldMessage  = "message"
	fieldPassword = "password"
	fieldToken    = "token"
)

// Git parameter names.
const (
	paramHash = "hash"
)

// Sensitive file patterns.
const (
	patternDotEnv = ".env"
	patternIDRSA  = "id_rsa"
)
