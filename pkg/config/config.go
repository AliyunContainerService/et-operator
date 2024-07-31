package config

// Config is the global configuration for the training operator.
var Config struct {
	InitContainerImage string
}

const (
	// InitContainerImageDefault is the default image for the training job
	// init container.
	InitContainerImageDefault = "alpine:3.10"
)
