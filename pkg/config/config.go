package config

// Config struct contains target group and namespace filters
type Config struct {
	targetGroups string
	namespace    string
	onlyNewPods  bool
}

// GetTargetGroups - return value
func (config Config) GetTargetGroups() string {
	return config.targetGroups
}

// GetNamespace - return value
func (config Config) GetNamespace() string {
	return config.namespace
}

// GetOnlyNewPods - return value
func (config Config) GetOnlyNewPods() bool {
	return config.onlyNewPods
}

// CreateConfig - return a config struct with it's members set
func CreateConfig(onlyNewPods bool) *Config {
	return &Config{
		onlyNewPods: onlyNewPods,
	}
}
